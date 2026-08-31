package pdfprinter2

import (
	"context"
	"log"
	"os"
)

var Logger *log.Logger

func init() {
	Logger = log.New(os.Stdout, "pdfprinter: ", log.Lshortfile)
}

func Print(ctx context.Context, html string) ([]byte, error) {
	return printOpt(ctx, PrintOptInput{
		HTML: html,
	})
}

func PrintOpt(ctx context.Context, opt PrintOptInput) ([]byte, error) {
	return printOpt(ctx, opt)
}
