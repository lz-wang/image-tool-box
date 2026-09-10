package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"imagetoolbox/internal/filehash"
	"imagetoolbox/internal/s3"
)

// MachineErrorSchemaVersion 是 CLI 失败输出（stdout）的机器可读契约版本。
// 当请求了 --format json 时，任何失败都会以该结构输出到 stdout，
// stderr 不再重复打印同一错误。
const MachineErrorSchemaVersion = "itb.error.v1"

// 稳定错误码：脚本消费方依赖 itb.error.v1 的 code 判断失败类别。
const (
	CodeInvalidArgument       = "E_INVALID_ARGUMENT"
	CodeInvalidConfig         = "E_INVALID_CONFIG"
	CodeFileNotFound          = "E_FILE_NOT_FOUND"
	CodeFileRead              = "E_FILE_READ"
	CodeSourceChanged         = "E_SOURCE_CHANGED"
	CodeObjectNotFound        = "E_OBJECT_NOT_FOUND"
	CodeBucketNotFound        = "E_BUCKET_NOT_FOUND"
	CodeAccessDenied          = "E_ACCESS_DENIED"
	CodeInvalidCredentials    = "E_INVALID_CREDENTIALS"
	CodeTimeout               = "E_TIMEOUT"
	CodeNetwork               = "E_NETWORK"
	CodeThrottled             = "E_THROTTLED"
	CodeChecksumMismatch      = "E_CHECKSUM_MISMATCH"
	CodeTargetConflict        = "E_TARGET_CONFLICT"
	CodeUnsupportedCapability = "E_UNSUPPORTED_CAPABILITY"
	CodeIncompleteList        = "E_INCOMPLETE_LIST"
	CodeInternal              = "E_INTERNAL"
)

// MachineErrorInfo 错误详情。http_status / provider_code 缺失时为 null。
type MachineErrorInfo struct {
	Code         string  `json:"code"`
	Message      string  `json:"message"`
	Retryable    bool    `json:"retryable"`
	HTTPStatus   *int    `json:"http_status"`
	ProviderCode *string `json:"provider_code"`
}

// MachineError itb.error.v1 的完整结构。
type MachineError struct {
	SchemaVersion string           `json:"schema_version"`
	Operation     string           `json:"operation"`
	Error         MachineErrorInfo `json:"error"`
}

// OperationError 标记一次 Action 失败及其操作名（如 "s3.upload"）。
type OperationError struct {
	Operation string
	Err       error
}

func (e *OperationError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *OperationError) Unwrap() error { return e.Err }

// InvalidArgumentError 标记 Action 内识别出的参数错误（operand 数量
// 不符、flag 组合冲突等）。没有这个类型，这类错误会被 classifyError
// 兜底为 E_INTERNAL——参数错误必须是 E_INVALID_ARGUMENT。
type InvalidArgumentError struct {
	Err error
}

func (e *InvalidArgumentError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *InvalidArgumentError) Unwrap() error { return e.Err }

// invalidArgument 构造 Action 内参数错误；消息面向用户，可安全透出。
func invalidArgument(format string, args ...any) error {
	return &InvalidArgumentError{Err: fmt.Errorf(format, args...)}
}

// operationError 包装 Action 错误；err 为 nil 时返回 nil。
func operationError(operation string, err error) error {
	if err == nil {
		return nil
	}
	// 已带操作名的错误保留最内层操作（子命令不应覆盖父命令标注）
	var oe *OperationError
	if errors.As(err, &oe) {
		return err
	}
	return &OperationError{Operation: operation, Err: err}
}

// ErrReported 表示失败信息已按 itb.error.v1 输出到 stdout，
// main 不得再向 stderr 重复打印。
var ErrReported = errors.New("error already reported in machine-readable form")

// Execute 运行根命令（使用进程的 stdin/stdout/stderr）。
func Execute(ctx context.Context, version string) error {
	return ExecuteArgs(ctx, version, os.Args, os.Stdout, os.Stderr)
}

// ExecuteArgs 以显式参数运行根命令并统一处理失败输出。
//
// 非 JSON 模式行为与 v0.9.x 一致：错误原样返回，由调用方打印到 stderr。
// 当请求了 --format json（transport 层窄检测）时，所有失败统一以
// itb.error.v1 输出到 stdout 并返回 ErrReported；urfave 自身的
// "Incorrect Usage" 与 usage 输出被缓冲吞掉，保证 stdout 恰好只有
// 一份 JSON 文档。
func ExecuteArgs(ctx context.Context, version string, args []string, stdout, stderr io.Writer) error {
	app := New(version)

	if !requestsJSONOutput(args) {
		app.Writer = stdout
		app.ErrWriter = stderr
		return app.Run(ctx, args)
	}

	// JSON 模式：缓冲 urfave 的 help/usage 输出，错误路径全部丢弃，
	// 成功路径（如 --help 与 --format json 组合）原样回放
	var buf bytes.Buffer
	app.Writer = &buf
	app.ErrWriter = &buf

	err := app.Run(ctx, args)
	if err == nil {
		_, _ = io.Copy(stdout, &buf)
		return nil
	}

	operation := operationOf(args, err)
	info := classifyError(err)
	_ = writeMachineError(stdout, MachineError{
		SchemaVersion: MachineErrorSchemaVersion,
		Operation:     operation,
		Error:         info,
	})
	return ErrReported
}

// requestsJSONOutput 做非常窄的 transport 层检测：args 是否请求了
// `--format json` / `--format=json`。不解析 flag 语义，仅用于决定
// 失败输出的呈现形式。
func requestsJSONOutput(args []string) bool {
	for i, arg := range args {
		if arg == "--format" && i+1 < len(args) && strings.EqualFold(args[i+1], "json") {
			return true
		}
		if strings.HasPrefix(arg, "--format=") && strings.EqualFold(strings.TrimPrefix(arg, "--format="), "json") {
			return true
		}
	}
	return false
}

// operationOf 确定失败的 operation 标注：优先 Action 标注的
// OperationError.Operation，参数解析阶段的失败从 argv 推断命令路径。
func operationOf(args []string, err error) string {
	var oe *OperationError
	if errors.As(err, &oe) && oe.Operation != "" {
		return oe.Operation
	}
	return inferOperationPath(New(versionUnknown), args)
}

// versionUnknown 仅供 operationOf 推断命令树使用，不参与执行。
const versionUnknown = "dev"

// inferOperationPath 沿 argv 走命令树，返回点分路径（如 "s3.list"）。
// 非命令 token（operand/flag）终止推断；无法识别时返回空串。
func inferOperationPath(app *cli.Command, args []string) string {
	cur := app
	var path []string
	for _, arg := range args {
		if arg == app.Name {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		var next *cli.Command
		for _, sub := range cur.Commands {
			if sub.Name == arg {
				next = sub
				break
			}
		}
		if next == nil {
			break
		}
		path = append(path, next.Name)
		cur = next
	}
	return strings.Join(path, ".")
}

// s3SentinelMapping 把 S3 领域 sentinel 错误映射为稳定错误码。
// message 一律使用固定文案，绝不携带 AWS SDK 的原始错误文本，
// 防止 provider 内部细节进入机器可读输出。
var s3SentinelMapping = []struct {
	sentinel  error
	code      string
	message   string
	retryable bool
}{
	{s3.ErrMissingEndpoint, CodeInvalidConfig, "S3 endpoint is not configured", false},
	{s3.ErrMissingCredentials, CodeInvalidCredentials, "S3 credentials are missing or incomplete", false},
	{s3.ErrInvalidCredentials, CodeInvalidCredentials, "credentials were rejected by the provider", false},
	{s3.ErrInvalidConfig, CodeInvalidConfig, "invalid S3 configuration", false},
	{s3.ErrInvalidOptions, CodeInvalidArgument, "invalid operation options", false},
	{s3.ErrMissingBucket, CodeInvalidConfig, "S3 bucket is not configured", false},
	{s3.ErrMissingKey, CodeInvalidArgument, "object key is required", false},
	{s3.ErrMissingInput, CodeInvalidArgument, "input file path is required", false},
	{s3.ErrFileNotFound, CodeFileNotFound, "input file not found", false},
	{s3.ErrObjectNotFound, CodeObjectNotFound, "object not found in bucket", false},
	{s3.ErrBucketNotFound, CodeBucketNotFound, "bucket not found", false},
	{s3.ErrAccessDenied, CodeAccessDenied, "access denied; check your credentials and permissions", false},
	{s3.ErrInvalidMetadata, CodeInvalidArgument, "invalid object metadata", false},
	{s3.ErrReservedMetadataKey, CodeInvalidArgument, "reserved metadata key", false},
	{s3.ErrInvalidSHA256, CodeInvalidArgument, "invalid SHA-256 digest", false},
	{s3.ErrSkipStrategyConflict, CodeInvalidArgument, "only one skip strategy can be enabled", false},
	{s3.ErrUnsupportedCapability, CodeUnsupportedCapability, "provider does not support the requested capability", false},
	{filehash.ErrSourceChanged, CodeSourceChanged, "source file changed while being read", false},
	{filehash.ErrNotRegularFile, CodeFileRead, "source is not a regular file", false},
	{s3.ErrVerifyFailed, CodeTargetConflict, "remote object state does not match this upload", false},
	{s3.ErrChecksumMismatch, CodeChecksumMismatch, "downloaded content does not match the expected SHA-256", false},
	{s3.ErrExpectationMismatch, CodeTargetConflict, "object state does not match the expected values", false},
	{s3.ErrReuseVerificationUnavailable, CodeInvalidArgument, "--if-exists=verify requires --verify-sha256 or --verify as a verification basis", false},
	{s3.ErrInvalidIfExists, CodeInvalidArgument, "invalid --if-exists value", false},
	{s3.ErrIncompleteList, CodeIncompleteList, "object listing could not be continued reliably", true},
}

// classifyError 把任意错误归类为稳定错误码 + 安全消息。
//
// 消息安全策略：
//   - 命中领域 sentinel：固定文案，不携带 provider 原始文本；
//   - provider（S3 API）错误：只输出 HTTP 状态与 provider code 字段，
//     消息使用固定摘要；
//   - 本地错误（文件、参数）：原样携带 err.Error()（不含凭据）；
//   - 兜底 E_INTERNAL。
func classifyError(err error) MachineErrorInfo {
	if err == nil {
		return MachineErrorInfo{Code: CodeInternal, Message: "no error"}
	}

	info := MachineErrorInfo{}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		info.Code = CodeTimeout
		info.Message = "operation timed out"
		info.Retryable = true
		return info
	case errors.Is(err, context.Canceled):
		info.Code = CodeInternal
		info.Message = "operation canceled"
		return info
	}

	// Action 内的 typed 参数错误：operand 数量、flag 组合冲突等
	var iae *InvalidArgumentError
	if errors.As(err, &iae) {
		info.Code = CodeInvalidArgument
		info.Message = err.Error()
		return info
	}

	for _, m := range s3SentinelMapping {
		if errors.Is(err, m.sentinel) {
			info.Code = m.code
			info.Message = m.message
			info.Retryable = m.retryable
			return applyProviderDetail(err, info)
		}
	}

	// 本地文件系统错误：消息可安全携带本地路径
	if errors.Is(err, fs.ErrNotExist) {
		info.Code = CodeFileNotFound
		info.Message = err.Error()
		return info
	}
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrInvalid) {
		info.Code = CodeFileRead
		info.Message = err.Error()
		return info
	}

	// 未被 sentinel 覆盖的 S3 provider 错误：只透出状态码与 provider code
	if detail, found := s3.DetailFromError(err); found {
		info.Code = CodeNetwork
		info.Message = "S3 provider returned an error"
		info.Retryable = detail.Retryable
		if detail.Throttled {
			// 限流/过载是独立于一般网络错误的类别（脚本据此决定
			// 指数退避而非立即重试）
			info.Code = CodeThrottled
		}
		return applyDetail(info, detail)
	}

	// 本地网络错误
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			info.Code = CodeTimeout
			info.Message = "network timeout"
		} else {
			info.Code = CodeNetwork
			info.Message = "network communication error"
		}
		info.Retryable = true
		return info
	}

	// urfave/cli 的参数解析错误没有类型化错误，按其稳定前缀窄识别
	//（发生在 Action 之前的失败同样归类为参数错误）
	if isFlagParseErrorText(err.Error()) {
		info.Code = CodeInvalidArgument
		info.Message = err.Error()
		return info
	}

	info.Code = CodeInternal
	info.Message = err.Error()
	return info
}

// flagParseErrorPrefixes 是 urfave/cli 参数解析错误文案的稳定前缀。
var flagParseErrorPrefixes = []string{
	"invalid value ",
	"flag provided but not defined",
	"flag needs an argument",
}

func isFlagParseErrorText(message string) bool {
	for _, prefix := range flagParseErrorPrefixes {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
}

// applyProviderDetail 附上 provider 摘要字段；限流错误升级为
// E_THROTTLED 且可重试。
func applyProviderDetail(err error, info MachineErrorInfo) MachineErrorInfo {
	detail, found := s3.DetailFromError(err)
	if !found {
		return info
	}
	if detail.Retryable {
		info.Retryable = true
	}
	if detail.Throttled && info.Code == CodeNetwork {
		info.Code = CodeThrottled
	}
	return applyDetail(info, detail)
}

func applyDetail(info MachineErrorInfo, detail s3.ErrorDetail) MachineErrorInfo {
	info.HTTPStatus = detail.HTTPStatus
	info.ProviderCode = detail.ProviderCode
	return info
}

// writeMachineError 以 JSON 编码输出 itb.error.v1 文档。
func writeMachineError(w io.Writer, me MachineError) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(me)
}
