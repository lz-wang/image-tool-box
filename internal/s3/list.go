package s3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ListSchemaVersion 是 list --format json 的机器可读契约版本。
// v2：JSON 从裸对象数组升级为结构化 ListResult，携带 complete / 分页
// 元数据；脚本消费方应以 schema_version 判断契约版本。
const ListSchemaVersion = "itb.s3.list.v2"

// maxPageSize 是单次 ListObjectsV2 请求 MaxKeys 的协议上限。
const maxPageSize = int32(1000)

// ErrIncompleteList 分页遍历无法可靠继续：服务端报告 IsTruncated=true
// 但未返回可用的 NextContinuationToken（缺失、重复或不前进）。
// 此时已收集的对象不作为成功结果返回，避免调用方把半份数据当完整清单。
var ErrIncompleteList = errors.New("incomplete object listing")

// ListOptions 列表选项
type ListOptions struct {
	Prefix    string
	Delimiter string

	// PageSize 是单次 ListObjectsV2 请求的 MaxKeys（1..1000）。
	// 0 表示使用协议默认（Normalize 归一化为 maxPageSize）；
	// 负值或超过 1000 由 Validate 拒绝，绝不静默收敛。
	PageSize int32

	// Limit 是输出对象总数上限；0 表示不限制。启用 Limit 后达到上限
	// 即停止翻页：complete=false 并返回 NextContinuationToken 供恢复。
	// 截断发生在 S3 请求边界：达到上限前最后一次请求的 MaxKeys 会
	// 收缩为剩余配额，保证 token 恰好指向最后输出对象的下一个键，
	// 恢复遍历不跳过任何对象。
	Limit int

	// ContinuationToken 从上一次 list 返回的 token 恢复遍历。
	ContinuationToken string

	// All 为 true 时持续翻页直到遍历结束；false（默认）只请求一页。
	All bool
}

// Normalize 归一化选项：未设置（0）的 PageSize 补协议默认。
func (o *ListOptions) Normalize() {
	if o.PageSize == 0 {
		o.PageSize = maxPageSize
	}
}

// Validate 校验选项。domain 是参数规则的唯一事实来源：超出协议范围
// 的 PageSize 与负数 Limit 是参数错误，绝不静默收敛——CLI validator
// 只是快速 UX 前置，不能是唯一防线。
func (o *ListOptions) Validate() error {
	if o.PageSize < 0 || o.PageSize > maxPageSize {
		return fmt.Errorf("%w: page-size must be in 1..%d, got %d", ErrInvalidOptions, maxPageSize, o.PageSize)
	}
	if o.Limit < 0 {
		return fmt.Errorf("%w: limit must not be negative, got %d", ErrInvalidOptions, o.Limit)
	}
	return nil
}

// ListResult 一次 list 操作的完整结果。
type ListResult struct {
	// SchemaVersion 机器可读契约版本（itb.s3.list.v2）
	SchemaVersion string `json:"schema_version"`

	// Bucket 存储桶名称
	Bucket string `json:"bucket"`

	// Prefix 本次遍历使用的键前缀
	Prefix string `json:"prefix"`

	// Complete 表示从本次起始 token 开始是否已正常遍历结束。
	// 被 --limit 截断、未启用 --all 且服务端还有后续页时为 false。
	Complete bool `json:"complete"`

	// Count 是 Objects 的对象数量
	Count int `json:"count"`

	// Pages 是实际发出的 ListObjectsV2 请求次数
	Pages int `json:"pages"`

	// NextContinuationToken 遍历未结束时的恢复 token；
	// complete=true 或服务端无后续页时省略。
	NextContinuationToken string `json:"next_continuation_token,omitempty"`

	Objects []ObjectInfo `json:"objects"`
}

// ObjectInfo 对象信息
type ObjectInfo struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	ETag         string    `json:"etag"`
	StorageClass string    `json:"storage_class"`
}

// List 列出存储桶中的对象。
//
// 默认只请求一页（与 v0.9.3 单页行为一致）；All=true 时持续翻页。
// complete 只表示"从本次起始 token 开始已经正常遍历结束"，不表示
// bucket 全局没有更多对象（调用方可能从中间 token 起步）。
//
// 中间任何一页请求失败时整个 List 失败，不返回半份成功结果。
func List(ctx context.Context, client *Client, opts *ListOptions) (*ListResult, error) {
	var options ListOptions
	if opts != nil {
		options = *opts
	}
	options.Normalize()
	if err := options.Validate(); err != nil {
		return nil, err
	}
	pageSize := options.PageSize

	result := &ListResult{
		SchemaVersion: ListSchemaVersion,
		Bucket:        client.bucket,
		Prefix:        options.Prefix,
		Objects:       []ObjectInfo{},
	}

	// token 环路保护：S3 的 token 应当每次前进；重复出现说明 provider
	// 实现有缺陷，继续循环只会无限请求
	seenTokens := map[string]bool{}
	if options.ContinuationToken != "" {
		seenTokens[options.ContinuationToken] = true
	}
	token := options.ContinuationToken

	for {
		// MaxKeys 收缩到剩余配额：--limit 截断必须落在 S3 请求边界上，
		// 否则 NextContinuationToken 会跳过本页已收到但未输出的对象
		batch := pageSize
		if options.Limit > 0 {
			if remaining := options.Limit - result.Count; remaining < int(batch) {
				batch = int32(remaining)
			}
		}

		input := &s3.ListObjectsV2Input{
			Bucket:  aws.String(client.bucket),
			MaxKeys: aws.Int32(batch),
		}
		if options.Prefix != "" {
			input.Prefix = aws.String(options.Prefix)
		}
		if options.Delimiter != "" {
			input.Delimiter = aws.String(options.Delimiter)
		}
		if token != "" {
			input.ContinuationToken = aws.String(token)
		}

		page, err := client.client.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, WrapError(err)
		}
		result.Pages++

		for _, item := range page.Contents {
			obj := ObjectInfo{
				Key:          aws.ToString(item.Key),
				Size:         aws.ToInt64(item.Size),
				LastModified: aws.ToTime(item.LastModified),
				ETag:         aws.ToString(item.ETag),
				StorageClass: string(item.StorageClass),
			}
			result.Objects = append(result.Objects, obj)
		}
		result.Count = len(result.Objects)
		// 按 Key 排序，保持面向人的输出顺序稳定
		sortObjects(result.Objects)

		if !aws.ToBool(page.IsTruncated) {
			result.Complete = true
			return result, nil
		}

		next := aws.ToString(page.NextContinuationToken)
		if next == "" || seenTokens[next] {
			return nil, fmt.Errorf("%w: provider reported more objects but no usable continuation token", ErrIncompleteList)
		}
		seenTokens[next] = true

		// 达到 --limit：正常退出，complete=false 并提供恢复 token
		if options.Limit > 0 && result.Count >= options.Limit {
			result.NextContinuationToken = next
			return result, nil
		}

		if !options.All {
			// 单页模式：服务端还有后续页，如实报告未完整遍历
			result.NextContinuationToken = next
			return result, nil
		}
		token = next
	}
}

// FormatOutput 格式化输出
func FormatOutput(result *ListResult, format string) string {
	switch format {
	case "json":
		return formatJSON(result)
	case "plain":
		return formatPlain(result.Objects)
	case "table":
		fallthrough
	default:
		return formatTable(result.Objects)
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

func formatJSON(result *ListResult) string {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf("failed to marshal list result: %v", err)
	}
	return string(data)
}

// sortObjects 按 Key 排序；ListObjectsV2 本身按 UTF-8 字节序返回，
// 排序是对 provider 实现差异的兜底。
func sortObjects(objects []ObjectInfo) {
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].Key < objects[j].Key
	})
}
