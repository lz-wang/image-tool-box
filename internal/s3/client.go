package s3

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Client S3 客户端封装
type Client struct {
  client         *s3.Client
  bucket         string
  forcePathStyle bool
}

// NewClient 创建 S3 客户端
func NewClient(ctx context.Context, cfg *Config) (*Client, error) {
  if err := cfg.Validate(); err != nil {
    return nil, err
  }

  // 创建自定义 HTTP 客户端（设置超时）
  httpClient := &http.Client{
    Timeout: 30 * time.Second,
    Transport: &http.Transport{
      MaxIdleConns:        10,
      IdleConnTimeout:     30 * time.Second,
      DisableCompression:  false,
      MaxIdleConnsPerHost: 10,
    },
  }

  // 创建凭证提供者
  creds := credentials.NewStaticCredentialsProvider(
    cfg.AccessKeyID,
    cfg.SecretAccessKey,
    "", // session token，通常为空
  )

  // 加载 AWS 配置
  awsCfg, err := config.LoadDefaultConfig(ctx,
    config.WithCredentialsProvider(creds),
    config.WithRegion(cfg.Region),
    config.WithHTTPClient(httpClient),
  )
  if err != nil {
    return nil, fmt.Errorf("failed to load AWS config: %w", err)
  }

  // 创建 S3 客户端
  client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
    o.BaseEndpoint = aws.String(cfg.Endpoint)
    if cfg.ForcePathStyle {
      o.UsePathStyle = true
    }
  })
  return &Client{
    client:         client,
    bucket:         cfg.Bucket,
    forcePathStyle: cfg.ForcePathStyle,
  }, nil
}
