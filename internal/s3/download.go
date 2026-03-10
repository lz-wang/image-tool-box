package s3

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// DownloadOptions 下载选项
type DownloadOptions struct {
  OutputFile string
}

// Download 从存储桶下载文件
func Download(ctx context.Context, client *Client, key string, outputPath string, opts *DownloadOptions) error {
  if key == "" {
    return ErrMissingKey
  }
  // 创建输出目录
  outputDir := filepath.Dir(outputPath)
  if outputDir != "" && outputDir != "." {
    if err := os.MkdirAll(outputDir, 0755); err != nil {
      return fmt.Errorf("failed to create output directory: %w", err)
    }
  }
  // 创建输出文件
  file, err := os.Create(outputPath)
  if err != nil {
    return fmt.Errorf("failed to create output file: %w", err)
  }
  defer file.Close()
  // 执行下载
  result, err := client.client.GetObject(ctx, &s3.GetObjectInput{
    Bucket: aws.String(client.bucket),
    Key:    aws.String(key),
  })
  if err != nil {
    return WrapError(err)
  }
  defer result.Body.Close()
  // 获取文件大小
  var size int64
  if result.ContentLength != nil {
    size = *result.ContentLength
  }
  if size > 5*1024*1024 {
    fmt.Printf("Downloading (%.2f MB)...\n", float64(size)/(1024*1024))
  }
  // 写入文件
  _, err = io.Copy(file, result.Body)
  if err != nil {
    return fmt.Errorf("failed to write file: %w", err)
  }
  fmt.Printf("Download completed: %s -> %s (%d bytes)\n", key, outputPath, size)
  return nil
}
