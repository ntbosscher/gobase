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
	go printer.Init()
}

func Print(ctx context.Context, html string) ([]byte, error) {
	return printer.Print(ctx, PrintOptInput{
		HTML: html,
	})
}

func PrintOpt(ctx context.Context, opt PrintOptInput) ([]byte, error) {
	return printer.Print(ctx, opt)
}
