package store

import (
	"context"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
)

type Bucket struct {
	client     *storage.Client
	bucketName string
}

func NewBucket(ctx context.Context, bucketName string) (*Bucket, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}
	return &Bucket{client: client, bucketName: bucketName}, nil
}

func (b *Bucket) Close() error {
	return b.client.Close()
}

func (b *Bucket) WriteBody(ctx context.Context, key string, body []byte) error {
	w := b.client.Bucket(b.bucketName).Object(key).NewWriter(ctx)
	w.ContentType = "application/octet-stream"
	if _, err := w.Write(body); err != nil {
		w.Close()
		return fmt.Errorf("failed to write to GCS: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close GCS writer: %w", err)
	}
	return nil
}

func (b *Bucket) ReadBody(ctx context.Context, key string) ([]byte, error) {
	r, err := b.client.Bucket(b.bucketName).Object(key).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read from GCS: %w", err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read GCS object body: %w", err)
	}
	return data, nil
}

func (b *Bucket) DeleteBody(ctx context.Context, key string) error {
	err := b.client.Bucket(b.bucketName).Object(key).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete GCS object: %w", err)
	}
	return nil
}
