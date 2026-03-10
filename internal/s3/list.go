package s3

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ListOptions 列表选项
type ListOptions struct {
	Prefix    string
	Delimiter string
	MaxKeys   int32
}

// ObjectInfo 对象信息
type ObjectInfo struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	ETag         string    `json:"etag"`
	StorageClass string    `json:"storage_class"`
}

// List 列出存储桶中的对象
func List(ctx context.Context, client *Client, opts *ListOptions) ([]ObjectInfo, error) {
	maxKeys := int32(1000)
	if opts != nil && opts.MaxKeys > 0 {
		maxKeys = opts.MaxKeys
	}

	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(client.bucket),
		MaxKeys: aws.Int32(maxKeys),
	}
	if opts != nil {
		if opts.Prefix != "" {
			input.Prefix = aws.String(opts.Prefix)
		}
		if opts.Delimiter != "" {
			input.Delimiter = aws.String(opts.Delimiter)
		}
	}

	result, err := client.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, WrapError(err)
	}

	var objects []ObjectInfo
	for _, item := range result.Contents {
		size := int64(0)
		if item.Size != nil {
			size = *item.Size
		}
		lastModified := time.Time{}
		if item.LastModified != nil {
			lastModified = *item.LastModified
		}
		etag := ""
		if item.ETag != nil {
			etag = *item.ETag
		}
		storageClass := ""
		if item.StorageClass != "" {
			storageClass = string(item.StorageClass)
		}
		obj := ObjectInfo{
			Key:          aws.ToString(item.Key),
			Size:         size,
			LastModified: lastModified,
			ETag:         etag,
			StorageClass: storageClass,
		}
		objects = append(objects, obj)
	}

	// 按 Key 排序
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].Key < objects[j].Key
	})

	return objects, nil
}

// FormatOutput 格式化输出
func FormatOutput(objects []ObjectInfo, format string) string {
	switch format {
	case "json":
		return formatJSON(objects)
	case "plain":
		return formatPlain(objects)
	case "table":
		fallthrough
	default:
		return formatTable(objects)
	}
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatTable(objects []ObjectInfo) string {
	if len(objects) == 0 {
		return "No objects found"
	}
	var sb strings.Builder
	sb.WriteString("KEY\t\tSIZE\t\tLAST MODIFIED\t\tETAG\t\tSTORAGE CLASS\n")
	for _, obj := range objects {
		lastMod := obj.LastModified.Format("2006-01-02 15:04:05")
		size := formatBytes(obj.Size)
		sb.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\n",
			obj.Key, size, lastMod, obj.ETag, obj.StorageClass))
	}
	return sb.String()
}

func formatPlain(objects []ObjectInfo) string {
	if len(objects) == 0 {
		return "No objects found"
	}
	var sb strings.Builder
	for _, obj := range objects {
		sb.WriteString(obj.Key)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func formatJSON(objects []ObjectInfo) string {
	if len(objects) == 0 {
		return "No objects found"
	}
	data, err := json.MarshalIndent(objects, "", "  ")
	if err != nil {
		return fmt.Sprintf("failed to marshal objects: %v", err)
	}
	return string(data)
}
