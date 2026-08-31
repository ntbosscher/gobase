// Package browserpool owns a single shared Chromium process for every caller in
// the app that needs a browser (HTML->PDF rendering, WebGL/pixi readback, etc.).
// It manages the Playwright driver, the one browser, concurrency gating,
// idle-shutdown and lifetime-recycle. Each caller owns its own context/page —
// that is the only place callers diverge, so the pool never builds contexts,
// applies network guards, or strips WebRTC. That work lives with the caller.
//
// The pool imports no security surface (no ssrfproxy, no printer). Callers that
// render untrusted content are responsible for their own egress controls; the
// app wires those into LaunchOptions before the first Acquire (see LaunchOptions).
package browserpool

import (
	"context"
	"errors"
	"log"
	"os"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"
)

// ErrOverloaded is returned by Acquire when the pool is at capacity and the
// caller's bounded wait (AcquireOptions.MaxWait) elapses before a slot frees.
var ErrOverloaded = errors.New("browserpool: overloaded, no slot within MaxWait")

// Logger receives lifecycle lines (launch, auto-shutdown, launch errors).
var Logger = log.New(os.Stdout, "browserpool: ", log.Lshortfile)

// LaunchOptions is the union of what every caller needs, set ONCE before the
// first Acquire and read on each browser (re)launch. The app assembles it —
// software-GL (swiftshader) args for WebGL, and the egress-proxy Server URL plus
// its hardening args for callers that render untrusted HTML. The pool treats it
// as opaque config; it makes no security promise of its own.
//
// Set Headless here too — the pool does not force it.
var LaunchOptions playwright.BrowserTypeLaunchOptions

// Lifecycle/concurrency tunables. InactiveShutdownDelay and MaxBrowserLifetime
// are read live by the manager. MaxConcurrent is read once, when the pool is
// first initialized (Warmup or the first Acquire), because it sizes the
// concurrency semaphore; set it before then.
var (
	// MaxConcurrent caps simultaneous renders sharing the one browser. Excess
	// Acquire calls block (cancellable via ctx) until a slot frees. Values < 1
	// are treated as 1. Read once at pool init.
	MaxConcurrent = 8

	// InactiveShutdownDelay closes the idle browser after this much inactivity.
	// The driver stays up; only the browser is closed, so the next Acquire
	// relaunches without reinstalling/restarting the driver.
	InactiveShutdownDelay = 5 * time.Minute

	// MaxBrowserLifetime recycles the browser once it is older than this. The
	// recycle drains in-flight renders to zero first, so it never interrupts a
	// render. The driver survives the recycle.
	MaxBrowserLifetime = 30 * time.Minute
)

type poolObj struct {
	initOnce sync.Once

	// sem is a counting semaphore sized to MaxConcurrent. A held unit (a send)
	// spans a caller's whole render, so the manager can only close the browser
	// when it can seize every unit — i.e. no caller is using it.
	sem chan struct{}

	mu         sync.Mutex // guards the fields below
	installed  bool
	playwright *playwright.Playwright
	chrome     playwright.Browser
	launchedAt time.Time
}

var pool = &poolObj{}

// Warmup starts the manager and installs Chromium in the background so the first
// Acquire pays less. Call it at startup, after setting LaunchOptions and
// MaxConcurrent. Optional — Acquire warms up on its own.
func Warmup() { pool.init() }

func (p *poolObj) init() {
	p.initOnce.Do(func() {
		n := MaxConcurrent
		if n < 1 {
			n = 1
		}
		p.sem = make(chan struct{}, n)
		go p.autoShutdown()
	})
}

// AcquireOptions tunes a single Acquire.
type AcquireOptions struct {
	// MaxWait bounds how long Acquire waits for a free slot when the pool is at
	// capacity:
	//   == 0  block until a slot frees or ctx is done (the Acquire default).
	//    > 0  wait up to this long, then return ErrOverloaded.
	//    < 0  do not wait — return ErrOverloaded immediately if no slot is free.
	// The bound covers only the wait for a slot, never the render itself.
	MaxWait time.Duration
}

// Acquire blocks for a concurrency slot, ensures the shared browser is up
// (installs + runs the driver once, launches with LaunchOptions on first use or
// after a recycle), and returns it. The caller builds its own context/page.
//
// Call release exactly once when the render is done — after closing your
// context/page. It frees the slot and lets idle/lifetime recycle proceed. The
// pool never closes the browser while any slot is held.
func Acquire(ctx context.Context) (browser playwright.Browser, release func(), err error) {
	return AcquireOpt(ctx, AcquireOptions{})
}

// AcquireOpt is Acquire with a bounded wait for a slot (see AcquireOptions). Use
// it to fail fast with ErrOverloaded under load instead of blocking.
func AcquireOpt(ctx context.Context, opts AcquireOptions) (browser playwright.Browser, release func(), err error) {
	pool.init()
	return pool.acquire(ctx, opts.MaxWait)
}

func (p *poolObj) acquire(ctx context.Context, maxWait time.Duration) (playwright.Browser, func(), error) {
	if err := p.acquireSlot(ctx, maxWait); err != nil {
		return nil, nil, err
	}

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { <-p.sem })
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureBrowserUnsafe(); err != nil {
		release()
		return nil, nil, err
	}

	return p.chrome, release, nil
}

// acquireSlot takes one semaphore unit, honoring the bounded-wait regimes in
// AcquireOptions.MaxWait.
func (p *poolObj) acquireSlot(ctx context.Context, maxWait time.Duration) error {
	if maxWait < 0 {
		// No wait: take a slot only if one is free right now.
		select {
		case p.sem <- struct{}{}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
			return ErrOverloaded
		}
	}

	if maxWait == 0 {
		// Block until a slot frees or ctx is done.
		select {
		case p.sem <- struct{}{}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Bounded wait: a slot, or give up with ErrOverloaded.
	t := time.NewTimer(maxWait)
	defer t.Stop()

	select {
	case p.sem <- struct{}{}:
		return nil
	case <-t.C:
		return ErrOverloaded
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *poolObj) autoShutdown() {
	// Warm the install off the first-Acquire path.
	_ = p.setupInstance()

	// Track when the pool first looked idle, so we only close after the full
	// InactiveShutdownDelay of quiet (matched at ticker granularity).
	var idleSince *time.Time

	tc := time.NewTicker(time.Minute)
	defer tc.Stop()

	for range tc.C {
		p.mu.Lock()
		hasBrowser := p.chrome != nil
		age := time.Since(p.launchedAt)
		p.mu.Unlock()

		if !hasBrowser {
			idleSince = nil
			continue
		}

		// Lifetime-recycle: drain to zero (waits for in-flight, never kills a
		// render), then close. The driver stays; the next Acquire relaunches.
		if age > MaxBrowserLifetime {
			Logger.Println("auto-shutdown: max-lifetime, recycling browser")
			p.drainAndCloseBrowser()
			idleSince = nil
			continue
		}

		// Idle-shutdown: only when nothing is held. len(sem)==0 gates the
		// attempt; tryCloseIfIdle re-checks atomically so we never close under a
		// live render.
		if len(p.sem) == 0 {
			now := time.Now()
			if idleSince == nil {
				idleSince = &now
			} else if now.Sub(*idleSince) > InactiveShutdownDelay {
				if p.tryCloseIfIdle() {
					Logger.Println("auto-shutdown: inactive-delay, browser closed")
				}
				idleSince = nil
			}
		} else {
			idleSince = nil
		}
	}
}

// drainAndCloseBrowser seizes every semaphore unit — completing only once all
// in-flight renders have released — then closes the browser and frees the units.
// New Acquire calls block on the semaphore until this returns.
func (p *poolObj) drainAndCloseBrowser() {
	n := cap(p.sem)
	for i := 0; i < n; i++ {
		p.sem <- struct{}{}
	}
	p.closeBrowser()
	for i := 0; i < n; i++ {
		<-p.sem
	}
}

// tryCloseIfIdle closes the browser only if it can seize every unit without
// blocking (i.e. no caller holds one). Returns whether it closed.
func (p *poolObj) tryCloseIfIdle() bool {
	n := cap(p.sem)
	seized := 0
	idle := true

	for seized < n {
		select {
		case p.sem <- struct{}{}:
			seized++
		default:
			idle = false
		}
		if !idle {
			break
		}
	}

	if idle {
		p.closeBrowser()
	}

	for i := 0; i < seized; i++ {
		<-p.sem
	}

	return idle
}

func (p *poolObj) closeBrowser() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeBrowserUnsafe()
}

// closeBrowserUnsafe closes only the browser, keeping the driver alive so the
// next launch skips the install/run. installed is left set on purpose.
func (p *poolObj) closeBrowserUnsafe() {
	if p.chrome != nil {
		p.chrome.Close()
		p.chrome = nil
		p.launchedAt = time.Time{}
	}
}

func (p *poolObj) setupInstance() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.setupInstanceUnsafe()
}

func (p *poolObj) setupInstanceUnsafe() error {
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

func (p *poolObj) ensureBrowserUnsafe() error {
	if err := p.setupInstanceUnsafe(); err != nil {
		return err
	}

	if p.playwright == nil {
		pw, err := playwright.Run(&playwright.RunOptions{
			Browsers: []string{"chromium"},
		})
		if err != nil {
			Logger.Println(err)
			return err
		}
		p.playwright = pw
	}

	if p.chrome == nil {
		ch, err := p.playwright.Chromium.Launch(LaunchOptions)
		if err != nil {
			Logger.Println(err)
			return err
		}
		p.chrome = ch
		p.launchedAt = time.Now()
	}

	return nil
}
