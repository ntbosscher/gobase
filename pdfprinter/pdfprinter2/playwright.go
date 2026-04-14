package pdfprinter2

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
)

type printerObj struct {
	mu sync.Mutex

	installed  bool
	playwright *playwright.Playwright
	chrome     playwright.Browser

	activeIncrementC chan bool
	activeDecrementC chan bool
}

func (p *printerObj) Init() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.activeIncrementC = make(chan bool)
	p.activeDecrementC = make(chan bool)
	p.setupInstanceUnsafe()

	go p.autoShutdown()
}

func (p *printerObj) autoShutdown() {
	active := 0
	inactiveDelay := 5 * time.Minute
	maxActiveTime := 30 * time.Minute

	var inactiveShutdown *time.Time = nil
	var maxActiveShutdown *time.Time = nil

	tc := time.NewTicker(time.Minute)
	defer tc.Stop()

	for {
		select {
		case _, ok := <-p.activeIncrementC:
			if !ok {
				return
			}

			if active == 0 {
				tm := time.Now().Add(maxActiveTime)
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
				tm := time.Now().Add(inactiveDelay)
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

	if p.playwright != nil {
		p.playwright.Stop()
		p.playwright = nil
	}
}

func (p *printerObj) setupBrowser() (playwright.BrowserContext, context.CancelFunc, error) {
	p.activeIncrementC <- true
	success := false

	p.mu.Lock()
	defer p.mu.Unlock()

	defer func() {
		if !success {
			p.activeDecrementC <- true
		}
	}()

	err := p.setupInstanceUnsafe()
	if err != nil {
		return nil, nil, err
	}

	p.playwright, err = playwright.Run(&playwright.RunOptions{
		Browsers: []string{"chromium"},
	})

	if err != nil {
		Logger.Println(err)
		p.shutdownInstanceUnsafe()
		return nil, nil, err
	}

	p.chrome, err = p.playwright.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})

	if err != nil {
		Logger.Println(err)
		p.shutdownInstanceUnsafe()
		return nil, nil, err
	}

	bctx, err := p.chrome.NewContext()
	if err != nil {
		Logger.Println(err)
		p.shutdownInstanceUnsafe()
		return nil, nil, err
	}

	success = true

	return bctx, func() {
		p.activeDecrementC <- true
	}, nil
}

func (p *printerObj) Print(ctx context.Context, html string) ([]byte, error) {

	bctx, done, err := p.setupBrowser()
	if err != nil {
		return nil, err
	}

	defer done()
	defer bctx.Close()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	pg, err := bctx.NewPage()
	if err != nil {
		return nil, errors.New("pdfprinter: could not create page")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	err = pg.SetContent(html)
	if err != nil {
		return nil, errors.New("pdfprinter: could not set content")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	err = pg.EmulateMedia(playwright.PageEmulateMediaOptions{
		Media: playwright.MediaPrint,
	})

	// give it a second to render
	<-time.After(3 * time.Second)

	return pg.PDF(playwright.PagePdfOptions{
		Scale: playwright.Float(1),
	})
}
