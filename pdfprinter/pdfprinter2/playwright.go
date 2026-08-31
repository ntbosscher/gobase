package pdfprinter2

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"
	"github.com/ntbosscher/gobase/browserpool"
	"github.com/ntbosscher/gobase/ssrfproxy"
)

// Tunable limits, read per render. Concurrency, idle-shutdown and
// lifetime-recycle now live on the shared browserpool (browserpool.MaxConcurrent
// etc.), which this package renders through.
var (
	// RenderTimeout bounds each page operation (SetContent/PDF) so a hostile
	// page that stalls on a slow subresource can't hang the worker. 0 disables
	// the timeout (not recommended for untrusted HTML).
	RenderTimeout = 30 * time.Second

	// RenderSettleDelay lets async content finish painting before the snapshot.
	RenderSettleDelay = 3 * time.Second

	// MaxHTMLBytes caps caller-supplied HTML size. 0 means no limit.
	MaxHTMLBytes = 20 * 1024 * 1024

	// EgressProxyEnabled controls whether this package configures the shared
	// browser's http/https egress through the built-in ssrfproxy
	// (resolve-once-and-pin). That proxy is the authoritative SSRF /
	// DNS-rebinding control for untrusted HTML; disabling it leaves only the
	// Route guard, whose IP check resolves DNS separately and is therefore
	// rebindable. Leave this on for untrusted HTML.
	//
	// Read once, on the first Print, and only if the app has not already set
	// browserpool.LaunchOptions.Proxy (e.g. a shared browser wired with its own
	// allow-listed proxy). To render untrusted HTML through a shared/WebGL
	// browser safely, the app must set that proxy itself; this package then
	// leaves it alone.
	EgressProxyEnabled = true

	// EgressProxyOptions configures the built-in proxy when this package sets it
	// up (EgressProxyEnabled and no app-provided proxy): a custom IsBlockedIP
	// policy, Resolver, or dial timeout. Its Logger defaults to this package's
	// Logger when unset. Read at first-Print setup.
	EgressProxyOptions ssrfproxy.Options
)

var egressSetup sync.Once
var egressSetupErr error

// ensureEgressConfigured wires the SSRF egress proxy into the shared browser the
// first time we render, unless the app already configured a proxy. It fails
// closed: if it cannot start the proxy, Print returns the error rather than
// rendering untrusted HTML with no authoritative egress control.
func ensureEgressConfigured() error {
	egressSetup.Do(func() {
		// App already pinned an egress proxy on the shared browser (e.g. the
		// WebGL-shared setup with an allow-listed render host). Trust it.
		if browserpool.LaunchOptions.Proxy != nil {
			return
		}

		if browserpool.LaunchOptions.Headless == nil {
			browserpool.LaunchOptions.Headless = playwright.Bool(true)
		}

		if !EgressProxyEnabled {
			// Explicitly disabled: launch proxy-less, leaving only the per-context
			// Route guard (rebindable). The caller opted into this.
			return
		}

		opts := EgressProxyOptions
		if opts.Logger == nil {
			opts.Logger = Logger
		}

		// Start the SSRF-safe egress proxy and route the shared browser's
		// http/https traffic through it. The proxy resolves-once-and-pins, which
		// is what actually defeats DNS rebinding — Chromium delegates name
		// resolution to the proxy, so there is no second lookup to rebind. It
		// lives for the process lifetime, surviving browser recycles.
		proxy, err := ssrfproxy.Start(opts)
		if err != nil {
			Logger.Println(err)
			egressSetupErr = err
			return
		}

		browserpool.LaunchOptions.Proxy = &playwright.Proxy{Server: proxy.URL()}
		browserpool.LaunchOptions.Args = append(browserpool.LaunchOptions.Args,
			// Force loopback through the proxy too. Chromium bypasses the proxy
			// for localhost/127.0.0.1 by default; "<-loopback>" removes that
			// implicit bypass so 127.0.0.1 SSRF can't skip it.
			"--proxy-bypass-list=<-loopback>",
			// Defense-in-depth for WebRTC: the APIs are already removed per
			// context (webRTCBlockScript), but if any path slips through, this
			// stops it opening direct (non-proxied) UDP sockets that would
			// sidestep the proxy and reach internal IPs.
			"--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
			// Prefer TCP+CONNECT for TLS; avoids any UDP/QUIC path around the
			// proxy.
			"--disable-quic",
		)
	})

	return egressSetupErr
}

type PrintOptInput struct {
	// optional: HTML content to print
	HTML string

	// optional: hooks for print process. Note the network egress guard is
	// installed before these run, so any requests they trigger (e.g.
	// pg.Goto(url)) are also subject to AllowRequestURL / the default SSRF
	// policy.
	OnSetupContent func(pg playwright.Page)

	// optional: custom render function (see OnSetupContent note re: egress guard)
	OnRender func(pg playwright.Page) ([]byte, error)

	// AllowRequestURL, if set, is consulted for every network request the page
	// attempts; returning false aborts that request. Use it to allow-list
	// specific hosts, or to lock the renderer down completely
	// (func(string) bool { return false }).
	//
	// When nil, the default guard applies: data:/blob:/about: are allowed, and
	// http(s) requests are blocked when the host resolves to a
	// loopback/private/link-local address (e.g. cloud-metadata 169.254.169.254
	// or RFC1918 internal services). Public http(s) is permitted so remote
	// images/fonts/styles still load.
	//
	// This hook is a per-request allow-list / scheme filter layered on top of
	// the always-on ssrfproxy egress proxy (which is the authoritative,
	// rebinding-proof control). Blocking private ranges does not depend on this
	// hook — the proxy pins the resolved IP regardless.
	AllowRequestURL func(rawURL string) bool

	// MaxSlotWait bounds how long Print waits for a free render slot on the
	// shared browser (browserpool.MaxConcurrent). It maps to
	// browserpool.AcquireOptions.MaxWait:
	//   == 0  block until a slot frees or ctx is done (default).
	//    > 0  wait up to this long, then fail with browserpool.ErrOverloaded.
	//    < 0  fail immediately with browserpool.ErrOverloaded if the pool is full.
	// It bounds only the wait for a slot, never the render (see RenderTimeout).
	MaxSlotWait time.Duration
}

func printOpt(ctx context.Context, input PrintOptInput) ([]byte, error) {

	if MaxHTMLBytes > 0 && len(input.HTML) > MaxHTMLBytes {
		return nil, errors.New("pdfprinter: html exceeds max size")
	}

	if err := ensureEgressConfigured(); err != nil {
		return nil, err
	}

	b, release, err := browserpool.AcquireOpt(ctx, browserpool.AcquireOptions{
		MaxWait: input.MaxSlotWait,
	})
	if err != nil {
		return nil, err
	}

	defer release()

	bctx, err := b.NewContext()
	if err != nil {
		Logger.Println(err)
		return nil, errors.New("pdfprinter: could not create browser context")
	}

	defer bctx.Close()

	// Remove the WebRTC APIs from every page before any page script runs.
	// WebRTC is never needed to render a PDF and is an SSRF/deanonymization
	// vector (host ICE candidates, STUN/TURN), so we delete the entry points
	// outright rather than only constraining them via launch flags — this holds
	// even if a future Chromium drops the flag.
	if err := bctx.AddInitScript(playwright.Script{Content: playwright.String(webRTCBlockScript)}); err != nil {
		Logger.Println(err)
		return nil, errors.New("pdfprinter: could not install webrtc guard")
	}

	// Install the network egress guard before any page is created so no
	// subresource/script request can escape it (SSRF protection).
	if err := applyNetworkGuard(bctx, input.AllowRequestURL); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	pg, err := bctx.NewPage()
	if err != nil {
		return nil, errors.New("pdfprinter: could not create page")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Bound every page operation (SetContent/PDF); a stalling page can't hang.
	pg.SetDefaultTimeout(float64(RenderTimeout.Milliseconds()))

	if input.OnSetupContent != nil {
		input.OnSetupContent(pg)
	}

	if input.HTML != "" {
		if err = pg.SetContent(input.HTML); err != nil {
			return nil, errors.New("pdfprinter: could not set content")
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if input.OnRender != nil {
		return input.OnRender(pg)
	}

	err = pg.EmulateMedia(playwright.PageEmulateMediaOptions{
		Media: playwright.MediaPrint,
	})

	if err != nil {
		Logger.Println("emulate media:", err)
	}

	// let async content settle, but stay cancellable
	select {
	case <-time.After(RenderSettleDelay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return pg.PDF(playwright.PagePdfOptions{
		Scale: playwright.Float(1),
	})
}

// webRTCBlockScript removes the WebRTC constructors from the page. It defines
// them as non-writable, non-configurable `undefined` so attacker JS can neither
// construct a peer connection (`new RTCPeerConnection` -> TypeError) nor restore
// the globals, while feature-detection (`if (window.RTCPeerConnection)`) simply
// sees them as absent and skips WebRTC gracefully. getUserMedia is also stubbed
// so it can't feed a connection.
const webRTCBlockScript = `(() => {
  const strip = (obj, name) => {
    try { Object.defineProperty(obj, name, { value: undefined, writable: false, configurable: false }); }
    catch (e) { try { obj[name] = undefined; } catch (e2) {} }
  };
  for (const n of ['RTCPeerConnection', 'webkitRTCPeerConnection', 'mozRTCPeerConnection',
                   'RTCDataChannel', 'RTCSessionDescription', 'RTCIceCandidate']) {
    strip(window, n);
  }
  try {
    if (navigator.mediaDevices) {
      navigator.mediaDevices.getUserMedia = () => Promise.reject(new Error('disabled'));
    }
  } catch (e) {}
  strip(navigator, 'getUserMedia');
  strip(navigator, 'webkitGetUserMedia');
  strip(navigator, 'mozGetUserMedia');
})();`

// applyNetworkGuard intercepts every request the page makes and aborts the ones
// the policy rejects. It enforces scheme policy and the caller's
// AllowRequestURL allow-list, and does a best-effort private-IP check.
//
// Note: the authoritative SSRF boundary is the ssrfproxy egress proxy the
// browser is launched with (resolve-once-and-pin), which is what actually
// defeats DNS rebinding. This Route guard is defense-in-depth and host
// allow-listing: its own IP check resolves DNS separately and is therefore
// rebindable, so it must not be relied on as the sole control. It uses the
// strict default IP policy (ssrfproxy.IsBlockedIP) even when the shared proxy
// allow-lists an internal host, so untrusted PDF HTML cannot reach that host.
func applyNetworkGuard(bctx playwright.BrowserContext, allow func(rawURL string) bool) error {
	if allow == nil {
		allow = defaultAllowRequestURL
	}

	return bctx.Route("**/*", func(route playwright.Route) {
		rawURL := route.Request().URL()

		if allow(rawURL) {
			_ = route.Continue()
			return
		}

		Logger.Println("blocked request (ssrf guard):", redactURL(rawURL))
		_ = route.Abort()
	})
}

// defaultAllowRequestURL permits inlined/in-memory schemes and public http(s),
// and blocks http(s) to loopback/private/link-local hosts. It fails closed on
// anything it can't parse or resolve.
func defaultAllowRequestURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	switch strings.ToLower(u.Scheme) {
	case "data", "blob", "about":
		// Inlined or in-memory content — no network egress.
		return true
	case "http", "https":
		// checked below
	default:
		// file, ftp, ws(s), chrome, and any unknown scheme: block.
		return false
	}

	host := u.Hostname()
	if host == "" {
		return false
	}

	return !hostResolvesToBlockedIP(host)
}

func hostResolvesToBlockedIP(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ssrfproxy.IsBlockedIP(ip)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return true // fail closed
	}

	for _, ip := range ips {
		if ssrfproxy.IsBlockedIP(ip) {
			return true
		}
	}

	return false
}

// redactURL strips the query/fragment so a blocked-request log line can't leak
// tokens carried in the URL.
func redactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "(unparseable url)"
	}

	return u.Scheme + "://" + u.Host + u.Path
}
