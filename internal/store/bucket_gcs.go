package store

import (
	"context"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
)

type GCSBucket struct {
	client     *storage.Client
	bucketName string
}

func NewGCSBucket(ctx context.Context, bucketName string) (*GCSBucket, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}
	return &GCSBucket{client: client, bucketName: bucketName}, nil
}

func (b *GCSBucket) Close() error {
	return b.client.Close()
}

func (b *GCSBucket) Write(ctx context.Context, key string, data []byte, contentType string) error {
	w := b.client.Bucket(b.bucketName).Object(key).NewWriter(ctx)
	w.ContentType = contentType
	if _, err := w.Write(data); err != nil {
		w.Close()
		return fmt.Errorf("GCS write failed: %w", err)
	}
	return w.Close()
}

func (b *GCSBucket) Read(ctx context.Context, key string) ([]byte, error) {
	r, err := b.client.Bucket(b.bucketName).Object(key).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("GCS read failed: %w", err)
	}
	defer r.Close()
	return io.ReadAll(r)
}

func (b *GCSBucket) Delete(ctx context.Context, key string) error {
	return b.client.Bucket(b.bucketName).Object(key).Delete(ctx)
}
