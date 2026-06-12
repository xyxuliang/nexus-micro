// Package minio 提供 MinIO 对象存储实现。
// 实现 Nexus Micro 框架的 storage.Storage 接口，支持 S3 兼容的 API。
// 适用于文件上传、图片存储、文档管理等场景。
package minio

import (
	"context"
	"fmt"
	"io"
	"time"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/xyxuliang/nexus-micro/storage"
)

// Storage 是基于 MinIO 的对象存储实现。
// 实现 storage.Storage 接口，支持上传、下载、删除和预签名 URL。
type Storage struct {
	client *miniogo.Client
	config *storage.Config
}

// NewStorage 创建一个新的 MinIO Storage 实例。
func NewStorage(cfg *storage.Config) (*Storage, error) {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "localhost:9000"
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "nexus"
	}

	client, err := miniogo.New(cfg.Endpoint, &miniogo.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("minio: create client failed: %w", err)
	}

	// 确保 Bucket 存在
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("minio: check bucket failed: %w", err)
	}
	if !exists {
		err = client.MakeBucket(ctx, cfg.Bucket, miniogo.MakeBucketOptions{Region: cfg.Region})
		if err != nil {
			return nil, fmt.Errorf("minio: create bucket failed: %w", err)
		}
	}

	return &Storage{
		client: client,
		config: cfg,
	}, nil
}

// Upload 上传文件到 MinIO。
func (s *Storage) Upload(ctx context.Context, key string, reader io.Reader, opts ...storage.UploadOption) (string, error) {
	uploadOpts := &storage.UploadOptions{}
	for _, o := range opts {
		o(uploadOpts)
	}

	putOpts := miniogo.PutObjectOptions{
		ContentType:  uploadOpts.ContentType,
		UserMetadata: uploadOpts.Metadata,
	}

	info, err := s.client.PutObject(ctx, s.config.Bucket, key, reader, -1, putOpts)
	if err != nil {
		return "", fmt.Errorf("minio: upload failed: %w", err)
	}

	return fmt.Sprintf("s3://%s/%s", s.config.Bucket, info.Key), nil
}

// Download 从 MinIO 下载文件。
func (s *Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.config.Bucket, key, miniogo.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("minio: download failed: %w", err)
	}
	return obj, nil
}

// Delete 从 MinIO 删除文件。
func (s *Storage) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.config.Bucket, key, miniogo.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("minio: delete failed: %w", err)
	}
	return nil
}

// GetPresignedURL 获取预签名 URL（用于临时访问）。
func (s *Storage) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	url, err := s.client.PresignedGetObject(ctx, s.config.Bucket, key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("minio: presigned url failed: %w", err)
	}
	return url.String(), nil
}

// =============================================================================
// 使用示例
// =============================================================================

// Example 展示如何使用 MinIO Storage。
func Example() {
	st, err := NewStorage(&storage.Config{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Bucket:    "nexus",
		UseSSL:    false,
	})
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	// 上传文件
	_ = st // use st

	// 获取预签名 URL
	url, err := st.GetPresignedURL(ctx, "avatars/user-123.png", 1*time.Hour)
	if err != nil {
		panic(err)
	}
	fmt.Printf("presigned URL: %s\n", url)
}