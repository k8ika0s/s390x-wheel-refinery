package objectstore

import (
	"bytes"
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOStore uploads objects to an S3-compatible backend.
type MinIOStore struct {
	Client   *minio.Client
	Bucket   string
	BasePath string
}

// NewMinIOStore initializes a MinIO client and ensures the bucket exists.
func NewMinIOStore(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*MinIOStore, error) {
	if endpoint == "" || bucket == "" {
		return nil, fmt.Errorf("endpoint and bucket required")
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}
	return &MinIOStore{Client: client, Bucket: bucket}, nil
}

// Put uploads data to bucket/key.
func (m *MinIOStore) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := m.Client.PutObject(ctx, m.Bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}
