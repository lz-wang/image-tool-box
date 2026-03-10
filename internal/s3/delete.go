package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// DeleteOptions 删除选项
type DeleteOptions struct {
  Key string
}

// Delete 从存储桶删除对象
func Delete(ctx context.Context, client *Client, key string, _ *DeleteOptions) error {
  if key == "" {
    return ErrMissingKey
  }
  // 执行删除
  _, err := client.client.DeleteObject(ctx, &s3.DeleteObjectInput{
    Bucket: aws.String(client.bucket),
    Key:    aws.String(key),
  })
  if err != nil {
    return WrapError(err)
  }
  fmt.Printf("Delete completed: s3://%s/%s\n", client.bucket, key)
  return nil
}
