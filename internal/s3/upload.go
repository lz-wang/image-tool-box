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

// contentTypes 内容类型映射
var contentTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".json": "application/json",
	".txt":  "text/plain",
	".html": "text/html",
	".css":  "text/css",
	".js":   "application/javascript",
	".pdf":  "application/pdf",
	".zip":  "application/zip",
}

// UploadOptions 上传选项
type UploadOptions struct {
	ContentType string
}

// Upload 上传文件到存储桶
func Upload(ctx context.Context, client *Client, inputPath string, key string, opts *UploadOptions) error {
  if inputPath == "" {
    return ErrMissingInput
  }
  if key == "" {
    return ErrMissingKey
  }
  // 打开本地文件
  file, err := os.Open(inputPath)
  if err != nil {
    return fmt.Errorf("failed to open input file: %w", err)
  }
  defer file.Close()

  // 获取文件信息
  fileInfo, err := file.Stat()
  if err != nil {
    return fmt.Errorf("failed to get file info: %w", err)
  }

  // 自动检测 Content type
  contentType := "application/octet-stream"
  if opts != nil && opts.ContentType != "" {
    contentType = opts.ContentType
  } else {
    ext := filepath.Ext(inputPath)
    if ct, ok := contentTypes[ext]; ok {
      contentType = ct
    }
  }
  // 获取文件大小
  fileSize := fileInfo.Size()
  // 创建上传输入
  var body io.Reader = file
  // 如果文件大于 5MB， 显示进度提示
  if fileSize > 5*1024*1024 {
    fmt.Printf("Uploading %s (%.2f MB)...\n", inputPath, float64(fileSize)/(1024*1024))
  }
  // 执行上传
  _, err = client.client.PutObject(ctx, &s3.PutObjectInput{
    Bucket:      aws.String(client.bucket),
    Key:         aws.String(key),
    Body:        body,
    ContentType: aws.String(contentType),
  })
  if err != nil {
    return WrapError(err)
  }
  fmt.Printf("Upload completed: %s -> s3://%s/%s (%d bytes)\n", inputPath, client.bucket, key, fileSize)
  return nil
}
