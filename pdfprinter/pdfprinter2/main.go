package pdfprinter2

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ntbosscher/gobase/lg"
)

var Logger *log.Logger

var printer = &printerObj{}

func init() {
	Logger = log.New(os.Stdout, "pdfprinter: ", log.Lshortfile)
	go printer.Init()
}

func Print(ctx context.Context, html string) ([]byte, error) {
	return printer.Print(ctx, html)
}

func runPrint(ctx context.Context, html string) ([]byte, error) {

	tmp, err := ioutil.TempDir(os.TempDir(), "chrome-html-to-pdf-*")
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			lg.Println(ctx, err)
		}
	}()

	inputFile := filepath.Join(tmp, "input.html")
	outputFile := filepath.Join(tmp, "package.pdf")
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

	if err = os.WriteFile(inputFile, []byte(html), os.ModePerm); err != nil {
		return nil, err
	}

	var cmdName string
	var args []string
	var env []string

	stdChromeArgs := []string{
		"--headless", "--disable-gpu",
		"--no-pdf-header-footer",
		"--user-data-dir=" + chromeUserData,
		"--crash-dumps-dir=" + chromeCrashDump,
		"--virtual-time-budget=10000",
		"--force-device-scale-factor=1",
		"--no-sandbox", "--print-to-pdf=" + outputFile, inputFile}

	switch runtime.GOOS {
	case "darwin":
		cmdName = `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`
		args = stdChromeArgs

	case "linux":
		// setup dbus session
		cmdName = "google-chrome-stable"
		args = stdChromeArgs

		userId := os.Getuid()
		env = []string{
			fmt.Sprintf("DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/%d/bus", userId),
			"PATH=" + os.Getenv("PATH"),
		}
	}

	if cmdName == "" {
		return nil, errors.New("unsupported OS")
	}

	// manually cancel the context when the pdf is done writing
	// chrome doesn't exit when it's done writing the pdf
	ctx, cancel := context.WithCancel(ctx)
	timer := time.AfterFunc(30*time.Second, cancel)

	normalExitC := make(chan bool)
	go func() {
		select {
		case <-normalExitC:
		case <-ctx.Done():
			lg.Println(ctx, "pdfprinter: context closed")
		}
	}()

	wrScanner := newMultiWriteScanner(func(line string) {
		if strings.Contains(line, "bytes written") {
			lg.Println(ctx, "pdfprinter: cancelled process b/c saw \"bytes written\"")
			cancel()
		} else {
			timer.Reset(3 * time.Second) // sometimes chrome just prints anything and that's the end for some reason.
		}
	}, os.Stdout)

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Stderr = wrScanner
	cmd.Stdout = wrScanner
	cmd.Env = env

	err = cmd.Run()

	if _, errExists := os.Stat(outputFile); errExists != nil {
		if err != nil {
			Logger.Println(cmdName, strings.Join(args, " "))
			Logger.Println("loading file error:", errExists.Error())
			Logger.Println("cmd error:", err.Error())
			return nil, err
		}
	}

	// failed, but pdf was created
	if err != nil && !strings.Contains(err.Error(), "killed") {
		Logger.Println(cmdName, strings.Join(args, " "))
		Logger.Println("error:", err.Error())
	}

	pdf, err := os.ReadFile(outputFile)
	if err != nil {
		Logger.Println(err)
		return nil, err
	}

	close(normalExitC)
	return pdf, nil
}

type multiWriteScanner struct {
	mu      sync.Mutex
	scanner func(line string)
	buf     *bytes.Buffer
	sc      *bufio.Scanner
	others  []io.Writer
}

func newMultiWriteScanner(scanner func(line string), others ...io.Writer) *multiWriteScanner {
	buf := &bytes.Buffer{}
	sc := bufio.NewScanner(buf)
	sc.Split(bufio.ScanLines)

	return &multiWriteScanner{
		scanner: scanner,
		buf:     buf,
		sc:      sc,
		others:  others,
	}
}

func (mws *multiWriteScanner) Write(p []byte) (n int, err error) {
	mws.mu.Lock()
	defer mws.mu.Unlock()

	n, err = mws.buf.Write(p)
	if err != nil {
		return n, err
	}

	for _, wr := range mws.others {
		wr.Write(p)
	}

	for mws.sc.Scan() {
		mws.scanner(mws.sc.Text())
	}

	return n, nil
}
