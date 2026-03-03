package services

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ObjectStore wraps an S3-compatible client for Cloudflare R2.
type ObjectStore struct {
	client *s3.Client
	bucket string
}

// NewObjectStore creates a new ObjectStore targeting the given R2 endpoint.
func NewObjectStore(endpoint, accessKeyID, secretKey, bucket string) *ObjectStore {
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       "auto",
		Credentials:  credentials.NewStaticCredentialsProvider(accessKeyID, secretKey, ""),
	})
	return &ObjectStore{client: client, bucket: bucket}
}

// IsConfigured returns true if the object store has a valid client.
func (o *ObjectStore) IsConfigured() bool {
	return o != nil && o.client != nil
}

// Upload stores data at the given key.
func (o *ObjectStore) Upload(ctx context.Context, key string, data []byte) error {
	_, err := o.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(o.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("objectstore upload %s: %w", key, err)
	}
	return nil
}

// Download retrieves the object at the given key and returns its contents.
func (o *ObjectStore) Download(ctx context.Context, key string) ([]byte, error) {
	out, err := o.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(o.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("objectstore download %s: %w", key, err)
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("objectstore read body %s: %w", key, err)
	}
	return data, nil
}

// Delete removes the object at the given key.
func (o *ObjectStore) Delete(ctx context.Context, key string) error {
	_, err := o.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(o.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("objectstore delete %s: %w", key, err)
	}
	return nil
}
