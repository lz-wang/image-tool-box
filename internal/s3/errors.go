package s3

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var (
	// ErrMissingEndpoint 端点未配置
	ErrMissingEndpoint = errors.New("endpoint is required")

	// ErrMissingCredentials 凭证未配置
	ErrMissingCredentials = errors.New("access key and secret key are required (set via flags or S3_ACCESS_KEY_ID/S3_SECRET_ACCESS_KEY env vars)")

	// ErrMissingBucket 存储桶未指定
	ErrMissingBucket = errors.New("bucket name is required")

	// ErrMissingKey 对象键未指定
	ErrMissingKey = errors.New("object key is required")

	// ErrMissingInput 输入文件未指定
	ErrMissingInput = errors.New("input file path is required")

	// ErrFileNotFound 文件未找到
	ErrFileNotFound = errors.New("file not found")

	// ErrObjectNotFound 对象未找到
	ErrObjectNotFound = errors.New("object not found in bucket")

	// ErrBucketNotFound 存储桶未找到
	ErrBucketNotFound = errors.New("bucket not found")
)

// WrapError 包装 S3 API 错误，提供更友好的错误信息
func WrapError(err error) error {
	if err == nil {
		return nil
	}

	// 处理 NoSuchKey 错误
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return fmt.Errorf("%w: %s", ErrObjectNotFound, err)
	}

	// 处理 NoSuchBucket 错误
	var noSuchBucket *types.NoSuchBucket
	if errors.As(err, &noSuchBucket) {
		return fmt.Errorf("%w: %s", ErrBucketNotFound, err)
	}

	// 处理 AccessDenied 错误
	var accessDenied *types.AccessDenied
	if errors.As(err, &accessDenied) {
		return fmt.Errorf("access denied: check your credentials and permissions")
	}

	return err
}
