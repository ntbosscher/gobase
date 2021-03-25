package s3fs

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/ntbosscher/gobase/env"
	errors2 "github.com/pkg/errors"
	"io"
	"mime"
	"mime/multipart"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var bucket string
var region string
var endpoint string

func init() {
	bucket = env.Require("AWS_BUCKET")
	endpoint = env.Require("AWS_ENDPOINT") // "nyc3.digitaloceanspaces.com"
	region = env.Require("AWS_REGION")     // "nyc3"
	env.Require("AWS_ACCESS_KEY_ID")
	env.Require("AWS_SECRET_ACCESS_KEY")
}

func sess() *session.Session {
	return session.Must(session.NewSession(&aws.Config{
		Endpoint: &endpoint,
		Region:   &region,
	}))
}

type UploadInput struct {
	FileName   string
	Key        string
	Body       io.Reader
	FileHeader *multipart.FileHeader
}

type uploadIterator struct {
	input              []*UploadInput
	openMultipartFiles []multipart.File
	position           int
	mu                 sync.RWMutex
}

// initialize sets up .Body for all UploadInput that used the FileHeader input method
// caller is responsible for calling .cleanup() at the appropriate time
func (u *uploadIterator) initialize() error {

	for _, item := range u.input {
		if item.Body == nil && item.FileHeader == nil {
			return errors.New("can't pass an upload with neither Body or FileHeader present")
		}

		if item.Body != nil && item.FileHeader != nil {
			return errors.New("can't pass an upload with both Body and FileHeader present")
		}

		if item.FileHeader != nil {
			// this will get cleaned up in u.cleanup()
			fi, err := item.FileHeader.Open()
			if err != nil {
				return errors2.Wrap(err, "failed to open FileHeader for upload")
			}

			item.Body = fi
			u.openMultipartFiles = append(u.openMultipartFiles, fi)
		}
	}

	return nil
}

func (u *uploadIterator) cleanup() error {
	for _, file := range u.openMultipartFiles {
		file.Close()
	}
}

func (u *uploadIterator) Next() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.position < len(u.input)
}

func (u *uploadIterator) Err() error {
	return nil
}

func (u *uploadIterator) UploadObject() s3manager.BatchUploadObject {

	u.mu.Lock()

	item := u.input[u.position]
	u.position++

	u.mu.Unlock()

	return s3manager.BatchUploadObject{
		Object: &s3manager.UploadInput{
			Bucket:             aws.String(bucket),
			Key:                aws.String(item.Key),
			Body:               item.Body,
			ContentDisposition: aws.String("attachment; filename=" + item.FileName),
		},
		After: func() error {
			return nil
		},
	}
}

func Upload(ctx context.Context, input []*UploadInput) error {
	uploader := s3manager.NewUploader(sess())

	iter := &uploadIterator{
		input:    input,
		position: 0,
	}

	if err := iter.initialize(); err != nil {
		return err
	}

	defer iter.cleanup()

	return uploader.UploadWithIterator(ctx, iter)
}

func Download(ctx context.Context, key string) ([]byte, error) {
	downloader := s3manager.NewDownloader(sess())

	buf := aws.NewWriteAtBuffer([]byte{})

	_, err := downloader.DownloadWithContext(ctx, buf, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	return buf.Bytes(), err
}

type DownloadType string

const (
	Inline     DownloadType = "inline"
	Attachment DownloadType = "attachment"
)

func DownloadLink(key string, downloadType DownloadType, fileName string) (string, error) {
	s3svc := s3.New(sess())

	contentType := aws.String(mime.TypeByExtension(filepath.Ext(fileName)))
	if aws.StringValue(contentType) == "" {
		contentType = nil
	}

	req, _ := s3svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket:                     aws.String(bucket),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String(string(downloadType) + "; filename=" + sanitizeDownloadFileName(fileName)),
		ResponseContentType:        contentType,
	})

	return req.Presign(5 * time.Minute)
}

// there's a chrome bug that doesn't handle commas in Content-Disposition filenames
// https://answers.nuxeo.com/general/q/d8348e07fe5e441183bae07dfda00e40/Comma-in-file-name-cause-problem-in-Chrome-Browser
func sanitizeDownloadFileName(fileName string) string {
	return strings.Replace(fileName, ",", "", -1)
}
