package s3fs

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/jv"
	errors2 "github.com/pkg/errors"
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

func sessOpt(ctx context.Context) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx,
		config.WithBaseEndpoint("https://"+endpoint),
		config.WithRegion(region))
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

func (u *uploadIterator) cleanup() {
	for _, file := range u.openMultipartFiles {
		file.Close()
	}
}

func (u *uploadIterator) Run(ctx context.Context) error {

	cfg, err := sessOpt(ctx)
	if err != nil {
		return err
	}

	uploader := manager.NewUploader(s3.NewFromConfig(cfg))

	workerC := make(chan *UploadInput)
	errC := make(chan error)
	doneC := make(chan bool, 10)
	workerCount := 0

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i := 0; i < len(u.input) && i < 4; i++ {
		workerCount++

		go func() {
			defer func() {
				doneC <- true
			}()

			for item := range workerC {
				_, err2 := uploader.Upload(ctx, &s3.PutObjectInput{
					Bucket:             aws.String(bucket),
					Key:                aws.String(item.Key),
					Body:               item.Body,
					ContentDisposition: aws.String("attachment; filename=" + item.FileName),
				})

				if err2 != nil {
					select {
					case errC <- err2:
						return
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	for _, item := range u.input {
		select {
		case workerC <- item:
		case <-ctx.Done():
			return ctx.Err()
		case err2 := <-errC:
			close(workerC)
			cancel()

			return err2
		}
	}

	close(workerC)

	for ct := 0; ct < workerCount; ct++ {
		select {
		case <-doneC:

		case <-ctx.Done():
			return ctx.Err()

		case err2 := <-errC:
			cancel()
			return err2
		}
	}

	return nil
}

func Upload(ctx context.Context, input []*UploadInput) error {

	iter := &uploadIterator{
		input: input,
	}

	if err := iter.initialize(); err != nil {
		return err
	}

	defer iter.cleanup()

	return iter.Run(ctx)
}

func Download(ctx context.Context, key string) ([]byte, error) {
	cfg, err := sessOpt(ctx)
	if err != nil {
		return nil, err
	}

	downloader := manager.NewDownloader(s3.NewFromConfig(cfg))
	buf := manager.NewWriteAtBuffer([]byte{})

	_, err = downloader.Download(ctx, buf, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	return buf.Bytes(), err
}

func DownloadToWriter(ctx context.Context, key string, wr io.WriterAt) (int64, error) {

	cfg, err := sessOpt(ctx)
	if err != nil {
		return 0, err
	}

	downloader := manager.NewDownloader(s3.NewFromConfig(cfg))

	return downloader.Download(ctx, wr, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
}

type DownloadType string

const (
	Inline     DownloadType = "inline"
	Attachment DownloadType = "attachment"
)

// DownloadLink is a convenience function for DownloadLink2 with a background context
// Deprecated: use DownloadLink2 instead
func DownloadLink(key string, downloadType DownloadType, fileName string) (string, error) {
	return DownloadLink2(context.Background(), key, downloadType, fileName)
}

func DownloadLink2(ctx context.Context, key string, downloadType DownloadType, fileName string) (string, error) {
	cfg, err := sessOpt(ctx)
	if err != nil {
		return "", err
	}

	s3svc := s3.NewPresignClient(s3.NewFromConfig(cfg))

	contentType := aws.String(mime.TypeByExtension(filepath.Ext(fileName)))
	if *contentType == "" {
		contentType = nil
	}

	req, err := s3svc.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(bucket),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String(string(downloadType) + "; filename=" + sanitizeDownloadFileName(fileName)),
		ResponseContentType:        contentType,
	}, func(o *s3.PresignOptions) {
		o.Expires = 5 * time.Minute
	})

	if err != nil {
		return "", err
	}

	return req.URL, nil
}

// GetPreSignedUploadURL is a convenience function for GetPreSignedUploadURL2 with a background context
// Deprecated: use GetPreSignedUploadURL2 instead
func GetPreSignedUploadURL(key string) (string, error) {
	return GetPreSignedUploadURL2(context.Background(), key)
}

func GetPreSignedUploadURL2(ctx context.Context, key string) (string, error) {
	return GetPreSignedUploadURL3(ctx, key, 5*time.Minute)
}

func GetPreSignedUploadURL3(ctx context.Context, key string, expiry time.Duration) (string, error) {
	cfg, err := sessOpt(ctx)
	if err != nil {
		return "", err
	}

	s3svc := s3.NewPresignClient(s3.NewFromConfig(cfg))

	rq, err := s3svc.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, func(o *s3.PresignOptions) {
		o.Expires = expiry
	})

	if err != nil {
		return "", err
	}

	return rq.URL, nil
}

// Remove is a convenience function for Remove2 with a background context
// Deprecated: use Remove2 instead
func Remove(key string) error {
	return Remove2(context.Background(), key)
}

// Remove2 removes an object from the bucket
func Remove2(ctx context.Context, key string) error {

	cfg, err := sessOpt(ctx)
	if err != nil {
		return err
	}

	s3svc := s3.NewFromConfig(cfg)

	_, err = s3svc.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	return err
}

// there's a chrome bug that doesn't handle commas in Content-Disposition filenames
// https://answers.nuxeo.com/general/q/d8348e07fe5e441183bae07dfda00e40/Comma-in-file-name-cause-problem-in-Chrome-Browser
func sanitizeDownloadFileName(fileName string) string {
	return strings.Replace(fileName, ",", "", -1)
}

// SetPermission updates the object's permission to the canned acl (see https://docs.aws.amazon.com/AmazonS3/latest/userguide/acl-overview.html#canned-acl)
func SetPermission(ctx context.Context, key string, cannedACL string) error {
	cfg, err := sessOpt(ctx)
	if err != nil {
		return err
	}

	s3svc := s3.NewFromConfig(cfg)

	_, err = s3svc.PutObjectAcl(ctx, &s3.PutObjectAclInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		ACL:    types.ObjectCannedACL(cannedACL),
	})

	return err
}

func GetInfo(ctx context.Context, key string) (*s3.HeadObjectOutput, error) {
	cfg, err := sessOpt(ctx)
	if err != nil {
		return nil, err
	}

	s3svc := s3.NewFromConfig(cfg)

	return s3svc.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
}

func Copy(ctx context.Context, sourceKey string, targetKey string) error {
	cfg, err := sessOpt(ctx)
	if err != nil {
		return err
	}

	s3svc := s3.NewFromConfig(cfg)

	_, err = s3svc.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: aws.String(bucket + sourceKey),
		Key:        aws.String(targetKey),
	})

	return err
}

type LifeCycleRule struct {
	Prefix            string
	ExpiresAfterNDays int
}

func SetLifeCycleRules(ctx context.Context, list []*LifeCycleRule) error {
	cfg, err := sessOpt(ctx)
	if err != nil {
		return err
	}

	s3svc := s3.NewFromConfig(cfg)

	_, err = s3svc.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket: aws.String(bucket),
		LifecycleConfiguration: &types.BucketLifecycleConfiguration{
			Rules: jv.Mapper(list, func(item *LifeCycleRule) types.LifecycleRule {
				return types.LifecycleRule{
					Filter: &types.LifecycleRuleFilter{
						Prefix: aws.String(item.Prefix),
					},
					Expiration: &types.LifecycleExpiration{
						Days: aws.Int32(int32(item.ExpiresAfterNDays)),
					},
					Status: types.ExpirationStatusEnabled,
				}
			}),
		},
	})

	return err
}
