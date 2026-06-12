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
	// Upload 上传对象。
	Upload(ctx context.Context, key string, reader io.Reader) error

	// Download 下载对象。
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete 删除对象。
	Delete(ctx context.Context, key string) error

	// GetPresignedURL 获取预签名 URL，用于客户端直接上传/下载。
	GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}

// Config MinIO 配置。
type Config struct {
	Endpoint string // MinIO 端点
	AccessKey string // Access Key
	SecretKey string // Secret Key
	Bucket    string // Bucket 名称
	UseSSL    bool   // 是否使用 SSL
}

// Info 文件信息。
type Info struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag"`
	LastModified time.Time `json:"last_modified"`
}