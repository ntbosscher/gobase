package remotezip

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

// MaxFileSize caps the uncompressed size of any single entry written into the zip.
// Set to 0 to disable the per-file cap.
var MaxFileSize int64 = 2 * GB

// MaxTotalSize caps the total uncompressed size across all entries in the zip.
// Set to 0 to disable the total cap.
var MaxTotalSize int64 = 10 * GB

const workerCount = 5

var errEntryTooLarge = errors.New("remotezip: entry exceeds configured size cap (MaxFileSize/MaxTotalSize)")

type Zip struct {
	wr      *zip.Writer
	tempDir string
	tempZip *os.File
	mu      *sync.Mutex

	remoteQueue []downloadItem

	totalWritten int64

	err error
}

// NewZip creates an empty zip. Call Close when done, even when Run is never called.
func NewZip() *Zip {
	return &Zip{
		mu: &sync.Mutex{},
	}
}

const GB = 1024 * 1024 * 1024
const MB = 1024 * 1024

// File is the finished zip. Close also removes it from disk.
type File struct {
	*os.File
}

func (f *File) Close() error {
	err := f.File.Close()
	rmErr := os.Remove(f.Name())

	if err != nil {
		return err
	}

	return rmErr
}

// checkDiskSpace makes sure every worker can hold a max-size download in the temp dir at once.
func checkDiskSpace() error {
	var stat unix.Statfs_t
	if err := unix.Statfs(os.TempDir(), &stat); err != nil {
		return err
	}

	need := uint64(1 * GB)
	if MaxFileSize > 0 {
		need = uint64(MaxFileSize * workerCount)
	}

	if uint64(stat.Bavail)*uint64(stat.Bsize) < need {
		return fmt.Errorf("Not enough free space on server (need at least %dMB)", need/MB)
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

// Close removes the temp files. It does not touch the File returned by Run.
func (z *Zip) Close() error {
	z.mu.Lock()
	defer z.mu.Unlock()

	return z.cleanupUnsafe()
}

// cleanupUnsafe removes the temp files. Callers must hold z.mu.
func (z *Zip) cleanupUnsafe() error {
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

	return z.remoteQueue, nil
}

// Run creates the zip file and returns it. The caller must Close the returned
// File, which also removes it from disk. On error, Run removes all temp files.
func (z *Zip) Run(ctx context.Context) (out *File, err error) {
	defer func() {
		if err != nil {
			z.Close()
		}
	}()

	queueItems, err := z.setupForRun()
	if err != nil {
		return nil, err
	}

	queue := make(chan downloadItem)
	// one slot per worker so a failing worker never blocks
	errC := make(chan error, workerCount)
	wg := &sync.WaitGroup{}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := z.worker(ctx, queue); err != nil {
				errC <- err
			}
		}()
	}

	var loopErr error

send:
	for _, item := range queueItems {
		// check disk space to prevent running out for big files
		if loopErr = checkDiskSpace(); loopErr != nil {
			break
		}

		select {
		case queue <- item:
		case loopErr = <-errC:
			break send
		case <-ctx.Done():
			loopErr = ctx.Err()
			break send
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
			case z.err = <-errC:
			default:
			}
		}
	}

	if z.err != nil {
		return nil, z.err
	}

	wr := z.wr
	z.wr = nil

	if z.err = wr.Close(); z.err != nil {
		return nil, z.err
	}

	tempZip := z.tempZip
	z.tempZip = nil

	if z.err = tempZip.Close(); z.err != nil {
		return nil, z.err
	}

	// move the zip out of tempDir so cleanupUnsafe can remove tempDir
	final, err := os.CreateTemp(os.TempDir(), "zipit-final-*.zip")
	if err != nil {
		z.err = err
		return nil, z.err
	}

	finalName := final.Name()
	z.err = final.Close()

	if z.err == nil {
		z.err = os.Rename(tempZip.Name(), finalName)
	}

	var handle *os.File
	if z.err == nil {
		handle, z.err = os.Open(finalName)
	}

	if z.err != nil {
		os.Remove(finalName)
		return nil, z.err
	}

	if err = z.cleanupUnsafe(); err != nil {
		handle.Close()
		os.Remove(finalName)
		return nil, err
	}

	return &File{File: handle}, nil
}

func (z *Zip) worker(ctx context.Context, queue chan downloadItem) (err error) {
	// a panic in a DownloadFunc would otherwise crash the process
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("remotezip: download panicked: %v", r)
		}
	}()

	tempFile, err := os.CreateTemp(z.tempDir, "download-*")
	if err != nil {
		return err
	}

	defer func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}()

	const maxAttempts = 2

	for {
		var item downloadItem
		var ok bool

		select {
		case item, ok = <-queue:
			if !ok {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}

		for attempt := 1; ; attempt++ {
			if err = tempFile.Truncate(0); err != nil {
				return err
			}

			err = item.Download(ctx, &limitedWriterAt{w: tempFile, max: MaxFileSize})
			if err == nil {
				break
			}

			if attempt >= maxAttempts || ctx.Err() != nil || errors.Is(err, errEntryTooLarge) {
				return err
			}
		}

		// reset reader position for fileWriter
		if _, err = tempFile.Seek(0, io.SeekStart); err != nil {
			return err
		}

		if err = z.addFileFromWriterSync(item.Name, tempFile); err != nil {
			return err
		}
	}
}

// limitedWriterAt stops a download at max bytes so an oversized remote file
// can't fill the disk before the zip size caps are checked. max <= 0 means no limit.
type limitedWriterAt struct {
	w   io.WriterAt
	max int64
}

func (l *limitedWriterAt) WriteAt(p []byte, off int64) (int, error) {
	if l.max > 0 && off+int64(len(p)) > l.max {
		return 0, errEntryTooLarge
	}

	return l.w.WriteAt(p, off)
}

type downloadItem struct {
	Name     string
	Download DownloadFunc
}

type DownloadFunc func(ctx context.Context, writer io.WriterAt) error

// AddRemoteFile queues a remote file to be downloaded and added to the zip file
// download must be able to be called from any goroutine
func (z *Zip) AddRemoteFile(name string, download DownloadFunc) error {
	z.mu.Lock()
	defer z.mu.Unlock()

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

	return z.addFileUnsafe(name, data)
}

func (z *Zip) AddMemoryFile(name string, data io.Reader) error {
	// hold the lock: this mutates z.wr/z.err/z.totalWritten, which the download
	// workers also touch — reading/writing them without the lock is a data race.
	z.mu.Lock()
	defer z.mu.Unlock()

	return z.addFileUnsafe(name, data)
}

// addFileUnsafe writes one entry. Callers must hold z.mu.
func (z *Zip) addFileUnsafe(name string, data io.Reader) error {
	// if we've already errored, don't continue
	if z.err != nil {
		return z.err
	}

	cleanName, err := sanitizeZipName(name)
	if err != nil {
		z.err = err
		return z.err
	}

	if z.err = z.initInternalsUnsafe(); z.err != nil {
		return z.err
	}

	w, err := z.wr.Create(cleanName)
	if err != nil {
		z.err = err
		return z.err
	}

	z.err = z.copyWithLimitUnsafe(w, data)
	return z.err
}

// copyWithLimitUnsafe copies data into w, enforcing MaxFileSize and MaxTotalSize.
// Callers must hold z.mu.
func (z *Zip) copyWithLimitUnsafe(w io.Writer, data io.Reader) error {
	remaining := int64(-1) // -1 == unlimited

	if MaxFileSize > 0 {
		remaining = MaxFileSize
	}

	if MaxTotalSize > 0 {
		totalRemaining := MaxTotalSize - z.totalWritten
		if totalRemaining < 0 {
			totalRemaining = 0
		}

		if remaining < 0 || totalRemaining < remaining {
			remaining = totalRemaining
		}
	}

	if remaining < 0 {
		written, err := io.Copy(w, data)
		z.totalWritten += written
		return err
	}

	// copy remaining+1 so we can detect an over-cap file
	written, err := io.Copy(w, io.LimitReader(data, remaining+1))
	z.totalWritten += written
	if err != nil {
		return err
	}

	if written > remaining {
		return errEntryTooLarge
	}

	return nil
}

// sanitizeZipName normalizes an entry name to a safe, archive-relative path so the
// writer can't emit zip-slip payloads (e.g. "../../etc/passwd") that a naive extractor
// would write outside the destination directory.
func sanitizeZipName(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")

	// prefix "/" then Clean so any leading "../" that would escape the root collapses away
	name = path.Clean("/" + name)
	name = strings.TrimPrefix(name, "/")

	if name == "" || name == "." {
		return "", errors.New("remotezip: invalid (empty) entry name")
	}

	return name, nil
}
