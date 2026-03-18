package store

import "context"

// ObjectStore is the interface for object/blob storage backends (GCS, S3).
type ObjectStore interface {
	Close() error
	Write(ctx context.Context, key string, data []byte, contentType string) error
	Read(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}
