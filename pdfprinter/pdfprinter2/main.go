package pdfprinter2

import (
	"context"
	"log"
	"os"
)

var Logger *log.Logger

var printer = &printerObj{}

func init() {
	Logger = log.New(os.Stdout, "pdfprinter: ", log.Lshortfile)
	// Start the manager goroutine, which runs playwright.Install and launch in the background so the first Print is
	// faster. This returns immediately; the warmup runs off the init path. All tunable limits —
	// including MaxConcurrentRenders — are read dynamically by the manager, so
	// consumers can still override them after import.
	printer.Init()
}

func Print(ctx context.Context, html string) ([]byte, error) {
	return printer.Print(ctx, PrintOptInput{
		HTML: html,
	})
}

func PrintOpt(ctx context.Context, opt PrintOptInput) ([]byte, error) {
	return printer.Print(ctx, opt)
}
