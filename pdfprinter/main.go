package pdfprinter

import (
	"context"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var Logger = log.Default()

// Print takes the html given and renders it to PDF using google-chrome.
func Print(ctx context.Context, html string) ([]byte, error) {

	tmp, err := ioutil.TempDir(os.TempDir(), "chrome-html-to-pdf-*")
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			log.Println(err)
		}
	}()

	inputFile := filepath.Join(tmp, "input.html")
	outputFile := filepath.Join(tmp, "railing-package.pdf")
	chromeBaseDir := filepath.Join(tmp, "chrome-internal-stuff")
	chromeUserData := filepath.Join(chromeBaseDir, "user")
	chromeDiskCache := filepath.Join(chromeBaseDir, "disk-cache")
	chromeCrashDump := filepath.Join(chromeBaseDir, "crash-dump")

	if err = os.MkdirAll(chromeUserData, os.ModePerm); err != nil {
		Logger.Println(err)
		return nil, err
	}

	if err = os.MkdirAll(chromeDiskCache, os.ModePerm); err != nil {
		Logger.Println(err)
		return nil, err
	}

	if err = os.MkdirAll(chromeCrashDump, os.ModePerm); err != nil {
		Logger.Println(err)
		return nil, err
	}

	if err = ioutil.WriteFile(inputFile, []byte(html), os.ModePerm); err != nil {
		return nil, err
	}

	var chrome string
	switch runtime.GOOS {
	case "darwin":
		chrome = `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`
	case "linux":
		chrome = "google-chrome-stable"
	}

	if chrome == "" {
		return nil, errors.New("unsupported OS")
	}

	args := []string{"--headless", "--disable-gpu", "--print-to-pdf-no-header",
		"--user-data-dir=" + chromeUserData,
		"--disk-cache-dir=" + chromeDiskCache,
		"--crash-dumps-dir=" + chromeCrashDump,
		"--virtual-time-budget=10000",
		"--no-sandbox", "--print-to-pdf=" + outputFile, inputFile}

	result, err := exec.CommandContext(ctx, chrome, args...).CombinedOutput()

	if _, errExists := os.Stat(outputFile); errExists != nil {
		if err != nil {
			Logger.Println(chrome, strings.Join(args, " "))
			Logger.Println("error:", err.Error())
			Logger.Println(string(result))
			return nil, err
		}
	}

	// failed, but pdf was created
	if err != nil {
		Logger.Println(chrome, strings.Join(args, " "))
		Logger.Println("error:", err.Error())
		Logger.Println(string(result))
	}

	pdf, err := ioutil.ReadFile(outputFile)
	if err != nil {
		Logger.Println(err)
		return nil, err
	}

	return pdf, nil
}
