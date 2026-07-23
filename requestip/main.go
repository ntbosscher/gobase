package requestip

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// Middleware makes the client IP available via IP(ctx).
//
// At least one Option is required so the trust model is always an explicit
// choice, never an accidental default:
//   - NoProxies      — use only r.RemoteAddr (the unforgeable TCP peer) and
//     ignore the spoofable X-Forwarded-For / X-Real-IP headers.
//   - TrustProxies   — honor forwarding headers only from configured trusted proxies.
//   - TrustProxyHops — honor forwarding headers for a known number of proxy hops.
func Middleware(opt Option, opts ...Option) func(withIp http.Handler) http.Handler {
	c := &config{}
	for _, o := range append([]Option{opt}, opts...) {
		o(c)
	}

	return func(withIP http.Handler) http.Handler {
		return &server{
			next:   withIP,
			config: c,
		}
	}
}

// Option configures the trusted-proxy behaviour of Middleware.
type Option func(c *config)

// NoProxies makes the middleware fail-closed: it uses only r.RemoteAddr (the
// direct TCP peer, which a client cannot forge) and ignores the spoofable
// X-Forwarded-For / X-Real-IP headers entirely. Use this when the app is exposed
// directly (no reverse proxy / load balancer in front of it).
func NoProxies() Option {
	return func(c *config) {
		// no configuration: the zero config already resolves to RemoteAddr only.
		// This exists so "trust nothing" is a deliberate, self-documenting choice.
	}
}

type config struct {
	trustedNets []*net.IPNet
	trustHops   int
	useHops     bool
}

// TrustProxies honors X-Forwarded-For / X-Real-IP only when the direct peer
// (r.RemoteAddr) falls within one of the given trusted CIDR ranges or exact IPs.
// When trusted, the real client is resolved by walking the forwarding chain from
// the closest hop outward and returning the first address that is not itself a
// trusted proxy. A client that connected directly, or spoofed headers arriving
// from an untrusted peer, fall back to RemoteAddr.
//
// Entries may be CIDRs ("10.0.0.0/8", "fd00::/8") or single IPs ("192.0.2.1").
// Panics on an invalid entry so a misconfiguration surfaces at startup.
func TrustProxies(cidrs ...string) Option {
	return func(c *config) {
		for _, entry := range cidrs {
			if n := parseCIDROrIP(entry); n != nil {
				c.trustedNets = append(c.trustedNets, n)
			}
		}
	}
}

// TrustProxyHops honors X-Forwarded-For by trusting exactly n proxy hops in front
// of the app (a single load balancer -> n=1). The client IP is taken n entries
// from the right of the (X-Forwarded-For..., RemoteAddr) chain. Use this when the
// proxies' addresses aren't stable but their count is. n <= 0 disables header trust.
//
// Takes precedence over TrustProxies if both are set.
func TrustProxyHops(n int) Option {
	return func(c *config) {
		c.useHops = true
		c.trustHops = n
	}
}

type server struct {
	next   http.Handler
	config *config
}

type ipKey string

var _ipKey ipKey = "ip"

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	source := s.config.clientIP(r)

	if source != "" {
		ctx := context.WithValue(r.Context(), _ipKey, source)
		r = r.WithContext(ctx)
	}

	s.next.ServeHTTP(w, r)
}

func (c *config) clientIP(r *http.Request) string {
	remote := hostOnly(r.RemoteAddr)

	if c.useHops {
		if c.trustHops <= 0 {
			return remote
		}

		chain := append(parseForwardedFor(r), remote)
		idx := len(chain) - 1 - c.trustHops
		if idx < 0 {
			idx = 0
		}

		return chain[idx]
	}

	// No trusted proxies configured, or the direct peer isn't one of them:
	// ignore the forwarding headers, they can't be trusted.
	if len(c.trustedNets) == 0 || !c.isTrusted(remote) {
		return remote
	}

	// Walk the chain from the closest hop outward, discarding trusted proxies.
	// The first non-trusted address is the real client.
	chain := append(parseForwardedFor(r), remote)
	for i := len(chain) - 1; i >= 0; i-- {
		if !c.isTrusted(chain[i]) {
			return chain[i]
		}
	}

	// Entire chain is our own proxies -> original client is the leftmost entry.
	return chain[0]
}

func (c *config) isTrusted(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	for _, n := range c.trustedNets {
		if n.Contains(ip) {
			return true
		}
	}

	return false
}

// parseForwardedFor returns the validated IPs from X-Forwarded-For (or X-Real-IP
// as a fallback), left-to-right. Unparseable entries are dropped so garbage can
// never be handed back as the client IP.
func parseForwardedFor(r *http.Request) []string {
	raw := r.Header.Get("X-Forwarded-For")
	if raw == "" {
		raw = r.Header.Get("X-Real-IP")
	}

	var out []string
	for _, part := range strings.Split(raw, ",") {
		host := hostOnly(part)
		if host == "" || net.ParseIP(host) == nil {
			continue
		}

		out = append(out, host)
	}

	return out
}

// hostOnly trims whitespace and strips a trailing port if present.
func hostOnly(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}

	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}

	return addr
}

func parseCIDROrIP(entry string) *net.IPNet {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return nil
	}

	if strings.Contains(entry, "/") {
		_, network, err := net.ParseCIDR(entry)
		if err != nil {
			panic("requestip: invalid trusted proxy CIDR " + strconv.Quote(entry) + ": " + err.Error())
		}

		return network
	}

	ip := net.ParseIP(entry)
	if ip == nil {
		panic("requestip: invalid trusted proxy IP " + strconv.Quote(entry))
	}

	if v4 := ip.To4(); v4 != nil {
		return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}
	}

	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
}

func IP(ctx context.Context) string {
	value := ctx.Value(_ipKey)
	if value == nil {
		return ""
	}

	return value.(string)
}

// KeyFromRequest returns a stable per-client key suitable for rate limiting. It
// prefers the trusted-proxy-resolved client IP (available when Middleware is
// installed) and otherwise falls back to the raw TCP peer host parsed from
// remoteAddr. If remoteAddr has no port it is returned unchanged. Callers should
// pass r.RemoteAddr as remoteAddr.
func KeyFromRequest(ctx context.Context, remoteAddr string) string {
	if ip := IP(ctx); ip != "" {
		return ip
	}

	if host := hostOnly(remoteAddr); host != "" {
		return host
	}

	return remoteAddr
}
