package store

import (
	"bytes"
	"context"
	"io"

	"github.com/mustafa-karli/basis/port"
	"github.com/mustafa-karli/basis/service/storage"
)

// BasisBucket adapts basis port.ObjectStorage to bdsmail's ObjectStore interface.
type BasisBucket struct {
	storage    port.ObjectStorage
	bucketName string
}

// NewBasisS3Bucket creates an S3-backed ObjectStore using basis storage.
func NewBasisS3Bucket(bucketName string) (*BasisBucket, error) {
	s, err := storage.NewStorageS3()
	if err != nil {
		return nil, err
	}
	return &BasisBucket{storage: s, bucketName: bucketName}, nil
}

// NewBasisGCSBucket creates a GCS-backed ObjectStore using basis storage.
func NewBasisGCSBucket(ctx context.Context, bucketName string) (*BasisBucket, error) {
	s, err := storage.NewStorageGCS(ctx)
	if err != nil {
		return nil, err
	}
	return &BasisBucket{storage: s, bucketName: bucketName}, nil
}

func (b *BasisBucket) Close() error {
	return nil
}

func (b *BasisBucket) Write(ctx context.Context, key string, data []byte, contentType string) error {
	return b.storage.Upload(ctx, b.bucketName, key, bytes.NewReader(data), contentType)
}

func (b *BasisBucket) Read(ctx context.Context, key string) ([]byte, error) {
	rc, err := b.storage.Download(ctx, b.bucketName, key)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (b *BasisBucket) Delete(ctx context.Context, key string) error {
	return b.storage.Delete(ctx, b.bucketName, key)
}
