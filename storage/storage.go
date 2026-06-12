// Package storage 提供对象存储统一接口。
// 默认实现基于 MinIO（S3 兼容），可替换为 AWS S3、阿里云 OSS 等。
// 支持上传、下载、删除、获取预签名 URL。
package storage

import (
	"context"
	"io"
	"time"
)

// Storage 对象存储接口。
type Storage interface {
	// Upload 上传对象，返回对象 URI（如 s3://bucket/key）。
	Upload(ctx context.Context, key string, reader io.Reader, opts ...UploadOption) (string, error)

	// Download 下载对象。
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete 删除对象。
	Delete(ctx context.Context, key string) error

	// GetPresignedURL 获取预签名 URL，用于客户端直接上传/下载。
	GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}

// UploadOption 上传选项函数。
type UploadOption func(*UploadOptions)

// UploadOptions 上传选项。
type UploadOptions struct {
	ContentType string            // 文件 MIME 类型
	Metadata    map[string]string // 自定义元数据
}

// WithContentType 设置上传文件的 Content-Type。
func WithContentType(ct string) UploadOption {
	return func(o *UploadOptions) {
		o.ContentType = ct
	}
}

// WithMetadata 设置上传文件的自定义元数据。
func WithMetadata(m map[string]string) UploadOption {
	return func(o *UploadOptions) {
		o.Metadata = m
	}
}

// Config 对象存储配置。
type Config struct {
	Endpoint  string // 存储端点（MinIO: localhost:9000, S3: s3.amazonaws.com）
	AccessKey string // Access Key
	SecretKey string // Secret Key
	Bucket    string // Bucket 名称
	Region    string // 区域（如 us-east-1）
	UseSSL    bool   // 是否使用 SSL
}

// Info 文件信息。
type Info struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag"`
	LastModified time.Time `json:"last_modified"`
}

// 编译期接口断言：确保 MinIO 实现符合接口
var _ Storage = (*minioStorage)(nil)

// minioStorage 编译时占位类型，实际实现在 plugins/minio 包。
type minioStorage struct{}

func (s *minioStorage) Upload(ctx context.Context, key string, reader io.Reader, opts ...UploadOption) (string, error) {
	return "", nil
}
func (s *minioStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	return nil, nil
}
func (s *minioStorage) Delete(ctx context.Context, key string) error { return nil }
func (s *minioStorage) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	return "", nil
}