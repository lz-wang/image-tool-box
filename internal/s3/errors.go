package s3

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

var (
	// ErrMissingEndpoint 端点未配置
	ErrMissingEndpoint = errors.New("endpoint is required")

	// ErrMissingCredentials 凭证未配置
	ErrMissingCredentials = errors.New("access key and secret key are required (set via flags or ITB_S3_ACCESS_KEY_ID/ITB_S3_SECRET_ACCESS_KEY env vars)")

	// ErrInvalidCredentials 凭证已提供但被 provider 判定无效
	//（InvalidAccessKeyId / SignatureDoesNotMatch / ExpiredToken 等）。
	// 与"本地未配置凭证"（ErrMissingCredentials）是不同的失败类别：
	// 前者需要更换凭证，后者需要补齐配置。
	ErrInvalidCredentials = errors.New("credentials were rejected by the provider")

	// ErrInvalidConfig 连接配置非法（网络行为参数为负等）。
	ErrInvalidConfig = errors.New("invalid S3 config")

	// ErrInvalidOptions 操作选项非法（page-size 超出协议范围、负数
	// limit/expect-size 等）。参数校验由 domain Normalize/Validate
	// 统一承担，CLI validator 只是快速 UX 前置。
	ErrInvalidOptions = errors.New("invalid options")

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

	// ErrAccessDenied 访问被拒绝（凭证或权限问题）。
	// 对 HeadObject 而言，403 也可能意味着无法确认对象是否存在，
	// 因此绝不把权限错误映射为"对象不存在"。
	ErrAccessDenied = errors.New("access denied")

	// ErrInvalidMetadata 用户 metadata 参数非法（缺 key=value、空 key、
	// 控制字符、重复 key 等）
	ErrInvalidMetadata = errors.New("invalid object metadata")

	// ErrReservedMetadataKey 试图占用系统保留的 metadata 键
	// （itb-sha256 由 itb 内部写入，用户不可覆盖）
	ErrReservedMetadataKey = errors.New("reserved metadata key")

	// ErrVerifyFailed 上传后 HEAD 校验未通过：远端 header/metadata
	// 与本次 PUT 的预期不一致。HEAD 只能证明 metadata/header 一致，
	// 不能证明 body 字节完整；body 校验由 download 校验承担。
	ErrVerifyFailed = errors.New("upload verification failed")

	// ErrChecksumMismatch 下载内容校验未通过：实际 SHA-256 与对象
	// metadata（--verify）或期望值（--verify-sha256）不一致。
	// 失败时本次下载的 partial 文件已被删除。
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrInvalidSHA256 --verify-sha256 参数不是合法的 SHA-256 digest
	//（64 个十六进制字符 / 32 字节）。参数错误必须在任何网络请求
	// 之前失败，而不是等下载完成后才变成 checksum mismatch。
	ErrInvalidSHA256 = errors.New("invalid SHA-256 digest")

	// ErrUnsupportedCapability provider 明确不支持所需能力
	//（如条件写 If-None-Match 返回 501 NotImplemented）。
	// 遇到该错误绝不自动降级为 HEAD + PUT——那会重新引入条件写
	// 要消除的 TOCTOU 竞态。
	ErrUnsupportedCapability = errors.New("provider does not support the requested capability")
)

// credentialProviderCodes 是 provider 判定"凭证无效"的错误码并集：
// 键不存在、签名不匹配、token 过期/失效。
var credentialProviderCodes = map[string]bool{
	"InvalidAccessKeyId":         true,
	"SignatureDoesNotMatch":      true,
	"ExpiredToken":               true,
	"ExpiredTokenException":      true,
	"InvalidToken":               true,
	"UnrecognizedClientException": true,
}

// WrapError 包装 S3 API 错误，提供更友好的错误信息。
//
// 除 typed error 外还解析 Smithy 的 HTTP 响应错误：
// HeadObject 在对象不存在时不一定携带 NoSuchKey typed error，
// 只返回 404 状态码（无 s3:ListBucket 权限时甚至返回 403），
// 因此 404 统一映射为 ErrObjectNotFound，403 保留为权限错误。
//
// 所有分支都用双 %w 把原始 provider 错误保留在 unwrap 链中：
// 错误分类层需要从链中提取 HTTP 状态码与 provider code，只保留
// 文本（%s）会让 provider_code 丢失。
func WrapError(err error) error {
	if err == nil {
		return nil
	}

	// 处理 NoSuchKey 错误
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return fmt.Errorf("%w: %w", ErrObjectNotFound, err)
	}

	// 处理 NoSuchBucket 错误
	var noSuchBucket *types.NoSuchBucket
	if errors.As(err, &noSuchBucket) {
		return fmt.Errorf("%w: %w", ErrBucketNotFound, err)
	}

	// 处理 provider 判定凭证无效：与 AccessDenied（权限不足）是
	// 不同的失败类别，消费方需要分别处理
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && credentialProviderCodes[apiErr.ErrorCode()] {
		return fmt.Errorf("%w: %w", ErrInvalidCredentials, err)
	}

	// 处理 AccessDenied 错误
	var accessDenied *types.AccessDenied
	if errors.As(err, &accessDenied) {
		return fmt.Errorf("%w: check your credentials and permissions: %w", ErrAccessDenied, err)
	}

	// 按 HTTP 状态码兜底识别（HeadObject 的 404/403 不带上述 typed error）
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) {
		switch responseErr.HTTPStatusCode() {
		case http.StatusNotFound:
			return fmt.Errorf("%w: %w", ErrObjectNotFound, err)
		case http.StatusUnauthorized:
			return fmt.Errorf("%w: %w", ErrInvalidCredentials, err)
		case http.StatusForbidden:
			return fmt.Errorf("%w: %w", ErrAccessDenied, err)
		}
	}

	return err
}

// conditionalWriteError 分类条件写（IfNoneMatch）PUT 的 provider 响应。
// 返回值：
//
//	"precondition_failed" → 412 PreconditionFailed：对象已存在
//	"conflict"            → 409 ConditionalRequestConflict：并发条件写
//	                        冲突（重试耗尽后到达此处的残余错误）
//	"unsupported"         → 501 NotImplemented：provider 明确不支持
//	                        条件写
//	""                    → 与条件写无关
func conditionalWriteError(err error) string {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "PreconditionFailed", "ConditionalRequestConflict", "NotImplemented":
			return map[string]string{
				"PreconditionFailed":          "precondition_failed",
				"ConditionalRequestConflict":  "conflict",
				"NotImplemented":              "unsupported",
			}[apiErr.ErrorCode()]
		}
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) {
		switch respErr.HTTPStatusCode() {
		case http.StatusPreconditionFailed:
			return "precondition_failed"
		case http.StatusConflict:
			return "conflict"
		case http.StatusNotImplemented:
			return "unsupported"
		}
	}
	return ""
}
