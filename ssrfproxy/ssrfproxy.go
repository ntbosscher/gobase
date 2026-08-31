// Package ssrfproxy implements a localhost HTTP/HTTPS forward proxy that makes
// server-side request forgery (SSRF) via DNS rebinding impossible.
//
// The core guarantee: for every destination the proxy resolves the hostname
// exactly once, rejects the request if any resolved address is
// loopback/private/link-local/etc., and then connects to that validated IP
// directly (pinning). Because an HTTP-proxy client (e.g. Chromium launched with
// --proxy-server) delegates name resolution to the proxy, there is no second,
// independent resolution for an attacker to rebind between the check and the
// connect. IP-literal-based checks in the renderer can be raced; this cannot.
//
// The proxy only ever sees http/https traffic — data:, blob:, and about: are
// handled internally by the browser and never egress — so its only job is IP
// vetting. Point Chromium at it with a browser-level proxy and make sure
// loopback is not bypassed (see the package consumer for the required flags).
package ssrfproxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ntbosscher/gobase/env"
)

// Options configures a Proxy. The zero value is usable; all fields are optional.
type Options struct {
	// Resolver used for destination hostname lookups. nil => net.DefaultResolver.
	Resolver *net.Resolver

	// DialTimeout bounds each upstream connect attempt. 0 => 10s.
	DialTimeout time.Duration

	// Logger receives one line per blocked/failed request. nil => no logging.
	Logger *log.Logger

	// IsBlockedIP decides whether a resolved address must be refused. nil =>
	// IsBlockedIP (the package default covering loopback/private/link-local/
	// CGNAT/unspecified/multicast).
	IsBlockedIP func(net.IP) bool
}

// Proxy is a running forward proxy bound to a loopback port. Create one with
// Start and stop it with Close.
type Proxy struct {
	ln        net.Listener
	srv       *http.Server
	transport *http.Transport
	resolver  *net.Resolver
	dialer    *net.Dialer
	isBlocked func(net.IP) bool
	logger    *log.Logger
}

// Start binds a listener on 127.0.0.1:<free-port> and begins serving. The proxy
// runs until Close is called.
func Start(opts Options) (*Proxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("ssrfproxy: listen: %w", err)
	}

	dialTimeout := opts.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}

	p := &Proxy{
		ln:        ln,
		resolver:  opts.Resolver,
		dialer:    &net.Dialer{Timeout: dialTimeout},
		isBlocked: opts.IsBlockedIP,
		logger:    opts.Logger,
	}
	if p.resolver == nil {
		p.resolver = net.DefaultResolver
	}
	if p.isBlocked == nil {
		p.isBlocked = IsBlockedIP
	}

	// The forwarding transport dials through the SSRF-vetting dialer and does not
	// follow redirects (RoundTrip never does) — each redirect target is re-sent
	// by the client through the proxy and re-vetted.
	p.transport = &http.Transport{
		DialContext:           p.safeDialContext,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}

	p.srv = &http.Server{
		Handler:           http.HandlerFunc(p.handle),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() { _ = p.srv.Serve(ln) }()

	return p, nil
}

// Addr returns the proxy's host:port (e.g. "127.0.0.1:54321").
func (p *Proxy) Addr() string { return p.ln.Addr().String() }

// URL returns the proxy address as an http:// URL suitable for --proxy-server /
// playwright.Proxy.Server.
func (p *Proxy) URL() string { return "http://" + p.Addr() }

// Close stops the proxy and closes the listener.
func (p *Proxy) Close() error {
	if p == nil || p.srv == nil {
		return nil
	}
	return p.srv.Close()
}

func (p *Proxy) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handleHTTP(w, r)
}

// handleConnect tunnels an HTTPS (or any TCP) connection. The proxy vets and
// pins the destination IP, then blindly pipes bytes — TLS stays end-to-end
// between the client and the origin.
func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	upstream, err := p.safeDialContext(r.Context(), "tcp", r.Host)
	if err != nil {
		p.logBlocked("CONNECT", r.Host, err)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "proxy error", http.StatusInternalServerError)
		return
	}

	client, _, err := hj.Hijack()
	if err != nil {
		upstream.Close()
		return
	}

	if _, err := client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		client.Close()
		upstream.Close()
		return
	}

	// Pipe both directions; when either side closes, tear down both so the other
	// io.Copy unblocks.
	go func() {
		_, _ = io.Copy(upstream, client)
		upstream.Close()
		client.Close()
	}()
	_, _ = io.Copy(client, upstream)
	client.Close()
	upstream.Close()
}

// handleHTTP forwards a plain-HTTP proxy request.
func (p *Proxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if !r.URL.IsAbs() || r.URL.Host == "" {
		http.Error(w, "proxy requires absolute-form request", http.StatusBadRequest)
		return
	}

	outreq := r.Clone(r.Context())
	outreq.RequestURI = "" // required to reuse a server request as a client request
	removeHopByHopHeaders(outreq.Header)

	resp, err := p.transport.RoundTrip(outreq)
	if err != nil {
		p.logBlocked("HTTP", r.URL.Host, err)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	defer resp.Body.Close()

	removeHopByHopHeaders(resp.Header)
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// safeDialContext is the SSRF boundary. It resolves the destination once,
// refuses the request if any resolved address is blocked, then connects to a
// validated address explicitly (pinned) so no rebinding can occur between the
// check and the connect.
func (p *Proxy) safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("ssrfproxy: bad address %q: %w", addr, err)
	}

	// IP literal: no resolution needed, vet and dial directly.
	if ip := net.ParseIP(host); ip != nil {
		if p.isBlocked(ip) {
			return nil, &BlockedError{Host: host, IP: ip}
		}
		return p.dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}

	ips, err := p.resolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("ssrfproxy: resolve %q: %w", host, err) // fail closed
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("ssrfproxy: no addresses for %q", host)
	}

	// Conservative: if ANY resolved address is blocked, refuse the whole request
	// rather than cherry-picking an allowed one (defeats dual A/AAAA tricks).
	for _, ip := range ips {
		if p.isBlocked(ip) {
			return nil, &BlockedError{Host: host, IP: ip}
		}
	}

	// Pin: dial the validated addresses explicitly. This is the only resolution
	// in play, so there is nothing for an attacker to rebind.
	var lastErr error
	for _, ip := range ips {
		conn, err := p.dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("ssrfproxy: dial %q: %w", host, lastErr)
}

func (p *Proxy) logBlocked(method, target string, err error) {
	if p.logger != nil {
		p.logger.Printf("ssrfproxy: refused %s %s: %v", method, target, err)
	}
}

// BlockedError is returned when a destination resolves to a disallowed address.
type BlockedError struct {
	Host string
	IP   net.IP
}

func (e *BlockedError) Error() string {
	return fmt.Sprintf("blocked host %q (resolves to %s)", e.Host, e.IP)
}

// IsBlockedIP reports whether ip is in a range that must not be reachable from
// untrusted content: loopback, RFC1918/ULA private, link-local (incl. the
// 169.254.169.254 cloud-metadata address), carrier-grade NAT, unspecified, and
// all multicast.
//
// As an exception, loopback (127.0.0.0/8, ::1) is allowed when env.IsTesting is
// set (TEST=true), so tests can render against a local httptest server. Only
// loopback is relaxed — private, link-local, metadata, and CGNAT ranges stay
// blocked even under testing. Never set TEST=true in an environment that renders
// untrusted HTML.
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	// Loopback is blocked in normal operation, but allowed under test so a local
	// httptest server is reachable.
	if ip.IsLoopback() && !env.IsTesting {
		return true
	}

	// IsPrivate covers RFC1918 + fc00::/7 (ULA); IsLinkLocalUnicast covers
	// 169.254.0.0/16 (incl. cloud metadata) + fe80::/10.
	if ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() ||
		ip.IsUnspecified() {
		return true
	}

	// Carrier-grade NAT 100.64.0.0/10 is not covered by IsPrivate.
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true
	}

	return false
}

// hopByHopHeaders are per-connection headers that a proxy must not forward.
var hopByHopHeaders = []string{
	"Connection",
	"Proxy-Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func removeHopByHopHeaders(h http.Header) {
	// Drop anything named in the Connection header first, then the standard set.
	for _, f := range h["Connection"] {
		for _, name := range strings.Split(f, ",") {
			if name = strings.TrimSpace(name); name != "" {
				h.Del(name)
			}
		}
	}
	for _, name := range hopByHopHeaders {
		h.Del(name)
	}
}

func copyHeader(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
