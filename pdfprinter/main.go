package pdfprinter

import (
	"context"
	"log"

	"github.com/ntbosscher/gobase/pdfprinter/pdfprinter2"
)

// Logger is retained for backwards compatibility. Rendering now happens in
// pdfprinter2, which has its own logger (pdfprinter2.Logger).
//
// Deprecated: use pdfprinter2 instead
var Logger = log.Default()

// Print takes the html given and renders it to PDF.
//
// Deprecated: this now delegates to pdfprinter2, which renders via Playwright with better security. Prefer calling
// pdfprinter2.Print / pdfprinter2.PrintOpt directly in new code.
func Print(ctx context.Context, html string) ([]byte, error) {
	return pdfprinter2.Print(ctx, html)
}
