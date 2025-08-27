package remotezip

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"golang.org/x/sys/unix"
)

type Zip struct {
	wr      *zip.Writer
	tempDir string
	tempZip *os.File
	mu      *sync.Mutex

	remoteQueue []downloadItem

	err error
}

func NewZip() *Zip {
	return &Zip{
		mu: &sync.Mutex{},
	}
}

const GB = 1024 * 1024 * 1024
const MB = 1024 * 1024

func (z *Zip) checkDiskSpace() error {
	if runtime.GOOS == "linux" {
		var stat unix.Statfs_t
		wd, err := os.Getwd()
		if err != nil {
			z.err = err
			return err
		}

		z.err = unix.Statfs(wd, &stat)
		if z.err != nil {
			return z.err
		}

		if stat.Bavail*uint64(stat.Bsize) < 1*GB {
			return errors.New("Not enough free space on server (need at least 1GB)")
		}
	}

	return nil
}

func (z *Zip) initInternalsUnsafe() error {
	if z.err != nil {
		return z.err
	}

	if z.tempDir != "" {
		// already initialized
		return nil
	}

	z.tempDir, z.err = os.MkdirTemp(os.TempDir(), "zipit-*")
	if z.err != nil {
		return z.err
	}

	z.tempZip, z.err = os.Create(filepath.Join(z.tempDir, "output.zip"))
	if z.err != nil {
		return z.err
	}

	z.wr = zip.NewWriter(z.tempZip)
	return z.err
}

func (z *Zip) Close() error {
	var err error

	if z.wr != nil {
		z.wr.Close()
		z.wr = nil
	}

	if z.tempZip != nil {
		z.tempZip.Close()
		z.tempZip = nil
	}

	if z.tempDir != "" {
		err = os.RemoveAll(z.tempDir)
		z.tempDir = ""
	}

	return err
}

func (z *Zip) setupForRun() ([]downloadItem, error) {
	z.mu.Lock()
	defer z.mu.Unlock()

	if z.err != nil {
		return nil, z.err
	}

	if err := z.initInternalsUnsafe(); err != nil {
		return nil, err
	}

	if z.err = z.checkDiskSpace(); z.err != nil {
		return nil, z.err
	}

	return z.remoteQueue, nil
}

// Run creates the zip file and returns a file handle to the zip file
// the file handle must be closed by the caller
func (z *Zip) Run(ctx context.Context) (*os.File, error) {

	queueItems, err := z.setupForRun()
	if err != nil {
		return nil, err
	}

	queue := make(chan downloadItem, 10)
	errC := make(chan error, 1)
	wg := &sync.WaitGroup{}

	for i := 0; i < 5; i++ {
		wg.Add(1)

		go func() {
			err2 := z.worker(ctx, wg, queue)

			select {
			case errC <- err2:
			default:
			}
		}()
	}

	var loopErr error

	for i, item := range queueItems {
		select {
		case queue <- item:
		case err = <-errC:
			loopErr = err
		}

		if loopErr != nil {
			break
		}

		// check disk space to prevent running out for big files
		if i != 0 && i%10 == 0 {
			loopErr = z.checkDiskSpace()
			if loopErr != nil {
				break
			}
		}
	}

	close(queue)
	wg.Wait()

	// entering restricted zone
	z.mu.Lock()
	defer z.mu.Unlock()

	// pull errors out and expose via z.err
	if z.err == nil {
		if loopErr != nil {
			z.err = loopErr
		} else {
			select {
			case err = <-errC:
				z.err = err
			default:
			}
		}
	}

	if z.err != nil {
		return nil, z.err
	}

	wr := z.wr
	z.wr = nil

	if err = wr.Close(); err != nil {
		z.err = err
		return nil, z.err
	}

	tempZip := z.tempZip
	z.tempZip = nil

	if err = tempZip.Close(); err != nil {
		z.err = err
		return nil, z.err
	}

	var outputFile *os.File
	var outputHandle *os.File

	outputFile, z.err = os.CreateTemp(os.TempDir(), "zipit-final-*.zip")
	if z.err != nil {
		return nil, z.err
	}

	z.err = outputFile.Close()
	if z.err != nil {
		return nil, z.err
	}

	z.err = os.Rename(tempZip.Name(), outputFile.Name())
	if z.err != nil {
		return nil, z.err
	}

	outputHandle, z.err = os.Open(outputFile.Name())
	if z.err != nil {
		os.Remove(outputFile.Name())
		return nil, z.err
	}

	err = z.Close()
	if err != nil {
		os.Remove(outputFile.Name())
		return nil, err
	}

	return outputHandle, nil
}

func (z *Zip) worker(ctx context.Context, wg *sync.WaitGroup, queue chan downloadItem) error {
	defer wg.Done()

	tempFile, err := os.CreateTemp(z.tempDir, "download-*")
	if err != nil {
		return err
	}

	defer func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}()

	resetTempFile := func() error {
		tempFile.Close()
		os.Remove(tempFile.Name())

		tempFile, err = os.CreateTemp(z.tempDir, "download-*")
		if err != nil {
			return err
		}

		return nil
	}

	var item downloadItem
	var ok bool

	for {

		select {
		case item, ok = <-queue:
			if !ok {
				return nil
			}
		case <-ctx.Done():
			return nil
		}

		maxAttemptCt := 2

		for retryDownloadIndex := 0; retryDownloadIndex < maxAttemptCt; retryDownloadIndex++ {
			// reset tempFile the easy way
			ferr := tempFile.Truncate(0)
			if ferr == nil {
				_, ferr = tempFile.Seek(0, io.SeekStart)
			}

			if ferr != nil {
				// reset tempFile the expensive way
				if err = resetTempFile(); err != nil {
					return err
				}
			}

			// download the file into tempFile
			if err = item.Download(ctx, tempFile); err != nil {
				if retryDownloadIndex < maxAttemptCt {
					continue
				}

				return err
			}

			// reset reader position for fileWriter
			if _, ferr = tempFile.Seek(0, io.SeekStart); ferr != nil {
				if retryDownloadIndex >= maxAttemptCt {
					return ferr
				}

				if err = resetTempFile(); err != nil {
					return err
				}

				continue
			}

			err = z.addFileFromWriterSync(item.Name, tempFile)
			if err != nil {
				if retryDownloadIndex >= maxAttemptCt {
					return err
				}

				continue
			}

			break
		}
	}
}

type downloadItem struct {
	Name     string
	Download DownloadFunc
}

type DownloadFunc func(ctx context.Context, writer io.WriterAt) error

// AddRemoteFile queues a remote file to be downloaded and added to the zip file
// download must be able to be called from any goroutine
func (z *Zip) AddRemoteFile(name string, download DownloadFunc) error {
	if z.err != nil {
		return z.err
	}

	z.remoteQueue = append(z.remoteQueue, downloadItem{
		Name:     name,
		Download: download,
	})

	return nil
}

func (z *Zip) addFileFromWriterSync(name string, data io.Reader) error {
	z.mu.Lock()
	defer z.mu.Unlock()

	// if we've already errored, don't continue
	if z.err != nil {
		return z.err
	}

	if z.err = z.initInternalsUnsafe(); z.err != nil {
		return z.err
	}

	w, err := z.wr.Create(name)
	if err != nil {
		z.err = err
		return z.err
	}

	_, z.err = io.Copy(w, data)
	return z.err
}

func (z *Zip) AddMemoryFile(name string, data io.Reader) error {
	// if we've already errored, don't continue
	if z.err != nil {
		return z.err
	}

	if z.err = z.initInternalsUnsafe(); z.err != nil {
		return z.err
	}

	w, err := z.wr.Create(name)
	if err != nil {
		z.err = err
		return z.err
	}

	_, z.err = io.Copy(w, data)
	return z.err
}
