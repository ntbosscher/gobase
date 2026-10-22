package remotezip

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
)

type generate struct {
	length int
}

func (g generate) Fill(writer io.WriterAt) error {
	data := []byte("test234567890-oiufdajsdf;aslkdf;alsdfasdf;aklsdjfnnwef23lk2")
	var err error

	i := int64(0)

	for i < int64(g.length) {
		_, err = writer.WriteAt(data, i)
		if err != nil {
			return err
		}

		i += int64(len(data))
	}

	return nil
}

func TestNewZipIt(t *testing.T) {

	zipper := NewZip()
	err := zipper.AddMemoryFile("test.txt", bytes.NewReader([]byte("test")))
	if err != nil {
		t.Fatalf("Error adding file: %v", err)
	}

	err = zipper.AddMemoryFile("test2.txt", bytes.NewReader([]byte("test2")))
	if err != nil {
		t.Fatalf("Error adding file: %v", err)
	}

	for i := 0; i < 5; i++ {
		err = zipper.AddRemoteFile(fmt.Sprint(i, "test.txt"), func(ctx context.Context, writer io.WriterAt) error {
			gen := &generate{length: 50 * MB}
			return gen.Fill(writer)
		})
	}

	out, err := zipper.Run(context.Background())
	if err != nil {
		t.Fatalf("Error running zipper: %v", err)
	}

	info, err := out.Stat()
	if err != nil {
		t.Fatalf("Error getting file info: %v", err)
	}

	rd, err := zip.NewReader(out, info.Size())
	if err != nil {
		t.Fatalf("Error reading zip file: %v", err)
	}

	for _, item := range rd.File {
		fmt.Println(item.Name, item.UncompressedSize64)
	}

	err = out.Close()
	if err != nil {
		t.Fatalf("Error closing file: %v", err)
	}

	if _, err = os.Stat(out.Name()); !os.IsNotExist(err) {
		t.Fatalf("Expected zip to be removed on close, got: %v", err)
	}
}

func TestRunCleansUpOnError(t *testing.T) {
	zipper := NewZip()

	err := zipper.AddMemoryFile("test.txt", bytes.NewReader([]byte("test")))
	if err != nil {
		t.Fatalf("Error adding file: %v", err)
	}

	err = zipper.AddRemoteFile("fail.txt", func(ctx context.Context, writer io.WriterAt) error {
		panic("boom")
	})
	if err != nil {
		t.Fatalf("Error adding file: %v", err)
	}

	tempDir := zipper.tempDir

	_, err = zipper.Run(context.Background())
	if err == nil {
		t.Fatalf("Expected error from panicking download")
	}

	if _, err = os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("Expected tempDir to be removed, got: %v", err)
	}
}

func TestRunCancelledContext(t *testing.T) {
	zipper := NewZip()

	for i := 0; i < 50; i++ {
		err := zipper.AddRemoteFile(fmt.Sprint(i, ".txt"), func(ctx context.Context, writer io.WriterAt) error {
			<-ctx.Done()
			return ctx.Err()
		})
		if err != nil {
			t.Fatalf("Error adding file: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := zipper.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Expected context.Canceled, got: %v", err)
	}
}
