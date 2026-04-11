package store

import (
	"bytes"
	"context"
	"fmt"
	"io"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Bucket provides S3-compatible object storage (works with AWS S3 and Cloudflare R2).
type S3Bucket struct {
	client     *s3.Client
	bucketName string
}

// NewS3Bucket creates an S3-compatible ObjectStore.
// For R2: endpoint = "https://<account-id>.r2.cloudflarestorage.com", region = "auto"
// For AWS S3: endpoint = "" (uses default), region = "us-west-2"
func NewS3Bucket(bucketName, region, endpoint string) (*S3Bucket, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	var client *s3.Client
	if endpoint != "" {
		// Custom endpoint (Cloudflare R2 or MinIO)
		client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = &endpoint
		})
	} else {
		client = s3.NewFromConfig(cfg)
	}

	return &S3Bucket{client: client, bucketName: bucketName}, nil
}

func (b *S3Bucket) Close() error {
	return nil
}

func (b *S3Bucket) Write(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &b.bucketName,
		Key:         &key,
		Body:        bytes.NewReader(data),
		ContentType: &contentType,
	})
	return err
}

func (b *S3Bucket) Read(ctx context.Context, key string) ([]byte, error) {
	result, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &b.bucketName,
		Key:    &key,
	})
	if err != nil {
		return nil, fmt.Errorf("S3 read failed: %w", err)
	}
	defer result.Body.Close()
	return io.ReadAll(result.Body)
}

func (b *S3Bucket) Delete(ctx context.Context, key string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &b.bucketName,
		Key:    &key,
	})
	return err
}
