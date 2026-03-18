package store

import (
	"bytes"
	"context"
	"fmt"
	"io"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Bucket struct {
	client     *s3.Client
	bucketName string
}

func NewS3Bucket(ctx context.Context, region, bucketName string) (*S3Bucket, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return &S3Bucket{client: s3.NewFromConfig(cfg), bucketName: bucketName}, nil
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
