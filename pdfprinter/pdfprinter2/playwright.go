package pdfprinter2

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ntbosscher/gobase/pdfprinter/pdfprinter2/ssrfproxy"
	"github.com/playwright-community/playwright-go"
)

// Tunable limits. Override these to change the renderer's resource bounds. All
// of them are read dynamically — per render, per manager grant decision, or per
// auto-shutdown tick — so they can be changed at any time, including after the
// package has been imported/initialized.
var (
	// MaxConcurrentRenders caps simultaneous renders sharing the single browser
	// process; excess Print calls block (cancellable via ctx) until a slot
	// frees, preventing unbounded page/context growth under load. The manager
	// reads it on every grant decision, so changes take effect immediately.
	// Values < 1 are treated as 1.
	MaxConcurrentRenders = 8

	// RenderTimeout bounds each page operation (SetContent/PDF) so a hostile
	// page that stalls on a slow subresource can't hang the worker. 0 disables
	// the timeout (not recommended for untrusted HTML).
	RenderTimeout = 30 * time.Second

	// RenderSettleDelay lets async content finish painting before the snapshot.
	RenderSettleDelay = 3 * time.Second

	// MaxHTMLBytes caps caller-supplied HTML size. 0 means no limit.
	MaxHTMLBytes = 20 * 1024 * 1024

	// InactiveShutdownDelay closes the idle browser after this much inactivity.
	InactiveShutdownDelay = 5 * time.Minute

	// MaxBrowserLifetime recycles the browser once it is older than this — only
	// while idle, never mid-render.
	MaxBrowserLifetime = 30 * time.Minute

	// EgressProxyEnabled controls whether the browser's http/https egress is
	// forced through the built-in ssrfproxy (resolve-once-and-pin). That proxy
	// is the authoritative SSRF / DNS-rebinding control for untrusted HTML;
	// disabling it leaves only the Route guard, whose IP check resolves DNS
	// separately and is therefore rebindable. Leave this on for untrusted HTML.
	//
	// Read when a browser is launched, so a change takes effect on the next
	// (re)launch, not mid-lifetime. Recycle the browser (e.g. lower
	// MaxBrowserLifetime, or let it go idle) to apply a change promptly.
	EgressProxyEnabled = true

	// EgressProxyOptions configures the built-in proxy when EgressProxyEnabled
	// is true: a custom IsBlockedIP policy (e.g. to also block extra ranges, or
	// to allow-list an internal host), a custom Resolver, or a dial timeout. Its
	// Logger defaults to this package's Logger when unset. Read at browser
	// launch.
	EgressProxyOptions ssrfproxy.Options
)

type printerObj struct {
	initOnce sync.Once
	mu       sync.Mutex

	installed  bool
	playwright *playwright.Playwright
	chrome     playwright.Browser
	proxy      *ssrfproxy.Proxy

	activeIncrementC chan *incrementReq
	activeDecrementC chan struct{}
}

type incrementReq struct {
	OkChan chan bool
}

func (p *printerObj) Init() {
	p.initOnce.Do(func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		p.activeIncrementC = make(chan *incrementReq)
		p.activeDecrementC = make(chan struct{})

		go p.autoShutdown()
	})
}

func (p *printerObj) autoShutdown() {
	active := 0

	var inactiveShutdown *time.Time = nil
	var maxActiveShutdown *time.Time = nil

	_ = p.setupInstance()

	tc := time.NewTicker(time.Minute)
	defer tc.Stop()

	for {
		select {
		case incrReq, ok := <-p.activeIncrementC:
			if !ok {
				return
			}

			if active+1 > MaxConcurrentRenders {
				incrReq.OkChan <- false
				continue
			}

			incrReq.OkChan <- true

			if active == 0 {
				tm := time.Now().Add(MaxBrowserLifetime)
				maxActiveShutdown = &tm
			}

			active++
			inactiveShutdown = nil
			Logger.Println("active-increment: active=", active)

		case _, ok := <-p.activeDecrementC:
			if !ok {
				return
			}

			active--
			Logger.Println("active-decrement: active=", active)

			if active <= 0 {
				active = 0
				tm := time.Now().Add(InactiveShutdownDelay)
				inactiveShutdown = &tm
			}

		case <-tc.C:

			if inactiveShutdown != nil && time.Now().After(*inactiveShutdown) {
				Logger.Println("auto-shutdown: inactive-delay: active=", active)

				if active <= 0 {
					p.shutdown()
				}

				inactiveShutdown = nil
				break
			}

			if maxActiveShutdown != nil && time.Now().After(*maxActiveShutdown) {
				Logger.Println("auto-shutdown: max-active-time: active=", active)

				// Drain in-flight renders to zero before recycling the browser.
				// While draining we're not in the main select, so new increment
				// requests block until this completes — a brief, by-design latency
				// spike (not a render kill: in-flight work finishes first).
				for active > 0 {
					select {
					case <-p.activeDecrementC:
						active--
					}
				}

				p.shutdown()
				maxActiveShutdown = nil
				break
			}
		}
	}
}

func (p *printerObj) shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.shutdownInstanceUnsafe()
}

func (p *printerObj) setupInstance() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.setupInstanceUnsafe()
}

func (p *printerObj) setupInstanceUnsafe() error {
	if p.installed {
		return nil
	}

	err := playwright.Install(&playwright.RunOptions{
		Browsers: []string{"chromium"},
	})

	if err != nil {
		Logger.Println(err)
		return err
	}

	p.installed = true
	return nil
}

func (p *printerObj) shutdownInstanceUnsafe() {
	if p.chrome != nil {
		p.chrome.Close()
		p.chrome = nil
	}

	if p.proxy != nil {
		p.proxy.Close()
		p.proxy = nil
	}

	if p.playwright != nil {
		p.playwright.Stop()
		p.playwright = nil
	}
}

func (p *printerObj) setupBrowser(ctx context.Context) (playwright.BrowserContext, context.CancelFunc, error) {
	okChan := make(chan bool, 1)

	select {
	case p.activeIncrementC <- &incrementReq{OkChan: okChan}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	if !<-okChan {
		return nil, nil, errors.New("pdfprinter: could not acquire browser slot, overloaded per pdfprinter2.MaxConcurrentRenders. callers to .Print() should use a queue and manage concurrency elsewhere")
	}

	success := false

	p.mu.Lock()
	defer p.mu.Unlock()

	defer func() {
		if !success {
			p.activeDecrementC <- struct{}{}
		}
	}()

	if err := p.setupInstanceUnsafe(); err != nil {
		return nil, nil, err
	}

	if p.playwright == nil {
		pw, err := playwright.Run(&playwright.RunOptions{
			Browsers: []string{"chromium"},
		})

		if err != nil {
			Logger.Println(err)
			p.shutdownInstanceUnsafe()
			return nil, nil, err
		}

		p.playwright = pw
	}

	if p.chrome == nil {
		launchOpts := playwright.BrowserTypeLaunchOptions{
			Headless: playwright.Bool(true),
		}

		if EgressProxyEnabled {
			// Start the SSRF-safe egress proxy and route all browser http/https
			// traffic through it. The proxy resolves-once-and-pins, which is
			// what actually defeats DNS rebinding — Chromium delegates name
			// resolution to the proxy when a proxy is set, so there is no second
			// lookup to rebind.
			opts := EgressProxyOptions
			if opts.Logger == nil {
				opts.Logger = Logger
			}

			proxy, err := ssrfproxy.Start(opts)
			if err != nil {
				Logger.Println(err)
				p.shutdownInstanceUnsafe()
				return nil, nil, err
			}
			p.proxy = proxy

			launchOpts.Proxy = &playwright.Proxy{Server: proxy.URL()}
			launchOpts.Args = append(launchOpts.Args,
				// Force loopback through the proxy too. Chromium bypasses the
				// proxy for localhost/127.0.0.1 by default; "<-loopback>"
				// removes that implicit bypass so 127.0.0.1 SSRF can't skip it.
				"--proxy-bypass-list=<-loopback>",
				// Keep WebRTC from opening direct (non-proxied) UDP sockets that
				// would sidestep the proxy and could reach internal IPs.
				"--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
				// Prefer TCP+CONNECT for TLS; avoids any UDP/QUIC path around
				// the proxy.
				"--disable-quic",
			)
		}

		ch, err := p.playwright.Chromium.Launch(launchOpts)
		if err != nil {
			Logger.Println(err)
			p.shutdownInstanceUnsafe()
			return nil, nil, err
		}

		p.chrome = ch
	}

	bctx, err := p.chrome.NewContext()
	if err != nil {
		Logger.Println(err)
		p.shutdownInstanceUnsafe()
		return nil, nil, err
	}

	success = true

	return bctx, func() {
		p.activeDecrementC <- struct{}{}
	}, nil
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
}

func (p *printerObj) Print(ctx context.Context, input PrintOptInput) ([]byte, error) {

	if MaxHTMLBytes > 0 && len(input.HTML) > MaxHTMLBytes {
		return nil, errors.New("pdfprinter: html exceeds max size")
	}

	bctx, done, err := p.setupBrowser(ctx)
	if err != nil {
		return nil, err
	}

	defer done()
	defer bctx.Close()

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

// applyNetworkGuard intercepts every request the page makes and aborts the ones
// the policy rejects. It enforces scheme policy and the caller's
// AllowRequestURL allow-list, and does a best-effort private-IP check.
//
// Note: the authoritative SSRF boundary is the ssrfproxy egress proxy the
// browser is launched with (resolve-once-and-pin), which is what actually
// defeats DNS rebinding. This Route guard is defense-in-depth and host
// allow-listing: its own IP check resolves DNS separately and is therefore
// rebindable, so it must not be relied on as the sole control.
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
