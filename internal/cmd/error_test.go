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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/smithy-go"

	"imagetoolbox/internal/s3"
)

// TestRequestsJSONOutput 锁定 transport 层窄检测：仅 `--format json`
// 与 `--format=json`（大小写不敏感）触发机器可读失败输出。
func TestRequestsJSONOutput(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"itb", "inspect", "--format", "json", "a.png"}, true},
		{[]string{"itb", "inspect", "--format=json", "a.png"}, true},
		{[]string{"itb", "inspect", "--format=JSON", "a.png"}, true},
		{[]string{"itb", "s3", "list", "-b", "b", "--format", "JSON"}, true},
		{[]string{"itb", "inspect", "--format", "table", "a.png"}, false},
		{[]string{"itb", "inspect", "--format=table", "a.png"}, false},
		{[]string{"itb", "inspect", "a.png"}, false},
		{[]string{"itb", "inspect", "--format"}, false},
		{[]string{"itb", "inspect", "--json", "a.png"}, false},
	}
	for _, tt := range tests {
		if got := requestsJSONOutput(tt.args); got != tt.want {
			t.Errorf("requestsJSONOutput(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestOperationError(t *testing.T) {
	if err := operationError("s3.upload", nil); err != nil {
		t.Fatalf("operationError(nil) = %v, want nil", err)
	}

	inner := fmt.Errorf("boom: %w", os.ErrNotExist)
	err := operationError("s3.upload", inner)
	var oe *OperationError
	if !errors.As(err, &oe) {
		t.Fatalf("expected *OperationError, got %T", err)
	}
	if oe.Operation != "s3.upload" {
		t.Errorf("operation = %q, want s3.upload", oe.Operation)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Error("OperationError must unwrap to the inner error")
	}
	if err.Error() != inner.Error() {
		t.Errorf("Error() = %q, want %q", err.Error(), inner.Error())
	}

	// 已标注操作名的错误不重复包装
	again := operationError("s3.list", err)
	var oe2 *OperationError
	if !errors.As(again, &oe2) || oe2.Operation != "s3.upload" {
		t.Fatalf("nested operation must keep the innermost label, got %v", again)
	}
}

func TestInferOperationPath(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"itb", "s3", "list", "--format", "json", "--page-size", "abc"}, "s3.list"},
		{[]string{"itb", "s3", "upload", "--bogus-flag"}, "s3.upload"},
		{[]string{"itb", "inspect", "--format=json", "a.png"}, "inspect"},
		{[]string{"itb", "--bad-flag"}, ""},
		{[]string{"itb"}, ""},
	}
	for _, tt := range tests {
		if got := inferOperationPath(testApp(), tt.args); got != tt.want {
			t.Errorf("inferOperationPath(%v) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

// TestClassifyErrorCodes 锁定 sentinel → 错误码映射。
func TestClassifyErrorCodes(t *testing.T) {
	notExist := fmt.Errorf("failed to open input file: %w", os.ErrNotExist)

	tests := []struct {
		name          string
		err           error
		wantCode      string
		wantRetryable bool
	}{
		{"missing endpoint", s3.ErrMissingEndpoint, CodeInvalidConfig, false},
		{"missing credentials", s3.ErrMissingCredentials, CodeInvalidCredentials, false},
		{"missing bucket", s3.ErrMissingBucket, CodeInvalidConfig, false},
		{"missing key", s3.ErrMissingKey, CodeInvalidArgument, false},
		{"file not found", notExist, CodeFileNotFound, false},
		{"object not found", fmt.Errorf("wrap: %w", s3.ErrObjectNotFound), CodeObjectNotFound, false},
		{"bucket not found", s3.ErrBucketNotFound, CodeBucketNotFound, false},
		{"access denied", s3.ErrAccessDenied, CodeAccessDenied, false},
		{"invalid metadata", s3.ErrInvalidMetadata, CodeInvalidArgument, false},
		{"invalid sha256", s3.ErrInvalidSHA256, CodeInvalidArgument, false},
		{"verify failed", s3.ErrVerifyFailed, CodeTargetConflict, false},
		{"checksum mismatch", s3.ErrChecksumMismatch, CodeChecksumMismatch, false},
		{"incomplete list", s3.ErrIncompleteList, CodeIncompleteList, true},
		{"deadline exceeded", fmt.Errorf("op: %w", context.DeadlineExceeded), CodeTimeout, true},
		{"canceled", context.Canceled, CodeInternal, false},
		{"generic", errors.New("something broke"), CodeInternal, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := classifyError(tt.err)
			if info.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", info.Code, tt.wantCode)
			}
			if info.Retryable != tt.wantRetryable {
				t.Errorf("retryable = %v, want %v", info.Retryable, tt.wantRetryable)
			}
			if info.Message == "" {
				t.Error("message must not be empty")
			}
		})
	}
}

// TestClassifyLocalFileError 本地 fs 错误携带原样消息（本地路径可透出）。
func TestClassifyLocalFileError(t *testing.T) {
	info := classifyError(os.ErrNotExist)
	if info.Code != CodeFileNotFound {
		t.Fatalf("code = %q, want %q", info.Code, CodeFileNotFound)
	}
	if info.Message != os.ErrNotExist.Error() {
		t.Errorf("message = %q, want %q", info.Message, os.ErrNotExist.Error())
	}
}

// TestClassifyNetErrors 本地网络错误分类为 E_NETWORK / E_TIMEOUT。
func TestClassifyNetErrors(t *testing.T) {
	timeoutErr := &timeoutNetError{}
	info := classifyError(fmt.Errorf("dial: %w", timeoutErr))
	if info.Code != CodeTimeout || !info.Retryable {
		t.Errorf("timeout error classified = %+v", info)
	}

	info = classifyError(fmt.Errorf("dial: %w", &plainNetError{}))
	if info.Code != CodeNetwork || !info.Retryable {
		t.Errorf("network error classified = %+v", info)
	}
}

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

type plainNetError struct{}

func (plainNetError) Error() string   { return "connection refused" }
func (plainNetError) Timeout() bool   { return false }
func (plainNetError) Temporary() bool { return true }

var _ net.Error = (*timeoutNetError)(nil)
var _ net.Error = (*plainNetError)(nil)

// TestClassifyProviderErrorSanitized 未映射的 S3 provider 错误只透出
// HTTP 状态与 provider code，消息为固定摘要，绝不携带 SDK 原始文本。
func TestClassifyProviderErrorSanitized(t *testing.T) {
	raw := errors.New(`operation error S3: ListObjectsV2, https response error StatusCode: 500, RequestID: XXXX, api error InternalError: We encountered an internal error`)
	info := classifyError(raw)
	if info.Code != CodeInternal {
		t.Fatalf("non-provider error should stay E_INTERNAL, got %q", info.Code)
	}

	wrapped := fmt.Errorf("wrap: %w", s3.ErrIncompleteList)
	info = classifyError(wrapped)
	if info.Code != CodeIncompleteList {
		t.Fatalf("code = %q, want %q", info.Code, CodeIncompleteList)
	}
	if strings.Contains(info.Message, "provider reported") {
		t.Errorf("sentinel message must be the fixed text, got %q", info.Message)
	}
}

// TestClassifyE_INCOMPLETEListCannotLeakRaw sentinel 命中时消息必须是
// 固定文案（错误契约的稳定面）。
func TestClassifyE_INCOMPLETEListCannotLeakRaw(t *testing.T) {
	raw := fmt.Errorf("%w: provider reported more objects but no usable continuation token", s3.ErrIncompleteList)
	info := classifyError(raw)
	if info.Message != "object listing could not be continued reliably" {
		t.Errorf("message = %q, want the fixed contract text", info.Message)
	}
}

// TestWriteMachineErrorShape 锁定 itb.error.v1 的 JSON 形状。
func TestWriteMachineErrorShape(t *testing.T) {
	var buf bytes.Buffer
	err := writeMachineError(&buf, MachineError{
		SchemaVersion: MachineErrorSchemaVersion,
		Operation:     "s3.download",
		Error: MachineErrorInfo{
			Code:     CodeChecksumMismatch,
			Message:  "Downloaded content does not match the expected SHA-256",
			Retryable: false,
		},
	})
	if err != nil {
		t.Fatalf("writeMachineError: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	if decoded["schema_version"] != "itb.error.v1" {
		t.Errorf("schema_version = %v", decoded["schema_version"])
	}
	if decoded["operation"] != "s3.download" {
		t.Errorf("operation = %v", decoded["operation"])
	}
	info, ok := decoded["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field = %v", decoded["error"])
	}
	if info["code"] != CodeChecksumMismatch || info["retryable"] != false {
		t.Errorf("error info = %v", info)
	}
	v, has := info["http_status"]
	if !has || v != nil {
		t.Errorf("http_status = %v (want explicit null)", v)
	}
	v, has = info["provider_code"]
	if !has || v != nil {
		t.Errorf("provider_code = %v (want explicit null)", v)
	}
}

// TestExecuteArgsJSONMachineError --format json 时失败输出为 stdout 上的
// 单份 itb.error.v1，返回 ErrReported，stderr 不再重复。
func TestExecuteArgsJSONMachineError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.png")
	var stdout, stderr bytes.Buffer

	err := ExecuteArgs(context.Background(), "test", []string{"itb", "inspect", "--format", "json", missing}, &stdout, &stderr)
	if !errors.Is(err, ErrReported) {
		t.Fatalf("err = %v, want ErrReported", err)
	}

	var decoded MachineError
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, stdout.String())
	}
	if decoded.SchemaVersion != MachineErrorSchemaVersion {
		t.Errorf("schema_version = %q", decoded.SchemaVersion)
	}
	if decoded.Operation != "inspect" {
		t.Errorf("operation = %q, want inspect", decoded.Operation)
	}
	if decoded.Error.Code != CodeFileNotFound {
		t.Errorf("code = %q, want %q", decoded.Error.Code, CodeFileNotFound)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty, got %q", stderr.String())
	}
}

// TestExecuteArgsFlagParseErrorJSON 参数解析阶段的失败同样输出机器错误，
// 且 urfave 的 "Incorrect Usage"/usage 不污染 stdout。
func TestExecuteArgsFlagParseErrorJSON(t *testing.T) {
	setS3Env(t, map[string]string{
		"ITB_S3_ENDPOINT":          "http://localhost:9000",
		"ITB_S3_ACCESS_KEY_ID":     "ak",
		"ITB_S3_SECRET_ACCESS_KEY": "sk",
		"ITB_S3_BUCKET":            "test",
	})

	var stdout, stderr bytes.Buffer
	err := ExecuteArgs(context.Background(), "test", []string{
		"itb", "s3", "list", "--format", "json", "--page-size", "abc",
	}, &stdout, &stderr)
	if !errors.Is(err, ErrReported) {
		t.Fatalf("err = %v, want ErrReported", err)
	}

	var decoded MachineError
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, stdout.String())
	}
	if decoded.Operation != "s3.list" {
		t.Errorf("operation = %q, want s3.list", decoded.Operation)
	}
	if decoded.Error.Code != CodeInvalidArgument {
		t.Errorf("code = %q, want %q", decoded.Error.Code, CodeInvalidArgument)
	}
	for _, banned := range []string{"Incorrect Usage", "USAGE", "OPTIONS"} {
		if strings.Contains(stdout.String(), banned) {
			t.Errorf("stdout must not contain usage text %q:\n%s", banned, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty, got %q", stderr.String())
	}
}

// TestExecuteArgsJSONSuccessFlushesHelp --help 与 --format json 组合时
// help 输出照常回放（成功路径不受缓冲影响）。
func TestExecuteArgsJSONSuccessFlushesHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := ExecuteArgs(context.Background(), "test", []string{"itb", "inspect", "--format", "json", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "inspect") {
		t.Errorf("help output missing command name:\n%s", stdout.String())
	}
}

// TestExecuteArgsNonJSONKeepsLegacyBehavior 非 JSON 模式错误原样返回，
// stdout 不出现机器错误（行为与 v0.9.x 一致）。
func TestExecuteArgsNonJSONKeepsLegacyBehavior(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.png")
	var stdout, stderr bytes.Buffer

	err := ExecuteArgs(context.Background(), "test", []string{"itb", "inspect", missing}, &stdout, &stderr)
	if err == nil || errors.Is(err, ErrReported) {
		t.Fatalf("err = %v, want raw error", err)
	}
	if strings.Contains(stdout.String(), MachineErrorSchemaVersion) {
		t.Errorf("stdout must not carry machine error in non-JSON mode:\n%s", stdout.String())
	}
}

// TestExecuteArgsS3ProviderErrorDetail 指向 fake S3 的 list --format json
// 在 provider 5xx 时输出 http_status / provider_code，且消息被清洗。
func TestExecuteArgsS3ProviderErrorDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `<Error><Code>InternalError</Code><Message>boom</Message><RequestID>req-123</RequestID></Error>`)
	}))
	t.Cleanup(srv.Close)

	var stdout, stderr bytes.Buffer
	err := ExecuteArgs(context.Background(), "test", []string{
		"itb", "s3", "list", "--format", "json",
		"--endpoint", srv.URL,
		"--access-key", "ak",
		"--secret-key", "sk-secret-value",
		"--bucket", "test-bucket",
		"--force-path-style",
	}, &stdout, &stderr)
	if !errors.Is(err, ErrReported) {
		t.Fatalf("err = %v, want ErrReported", err)
	}

	var decoded MachineError
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if decoded.Operation != "s3.list" {
		t.Errorf("operation = %q", decoded.Operation)
	}
	if decoded.Error.HTTPStatus == nil || *decoded.Error.HTTPStatus != 500 {
		t.Errorf("http_status = %v, want 500", decoded.Error.HTTPStatus)
	}
	if decoded.Error.ProviderCode == nil || *decoded.Error.ProviderCode != "InternalError" {
		t.Errorf("provider_code = %v, want InternalError", decoded.Error.ProviderCode)
	}
	if !decoded.Error.Retryable {
		t.Error("provider 5xx must be retryable")
	}
	if decoded.Error.Code != CodeNetwork {
		t.Errorf("code = %q, want %q", decoded.Error.Code, CodeNetwork)
	}
	// 安全边界：消息是固定摘要，凭据与原始 provider 文本不得出现
	out := stdout.String()
	if strings.Contains(out, "sk-secret-value") {
		t.Error("machine error must never contain the secret key")
	}
	if strings.Contains(out, "RequestID") || strings.Contains(out, "We encountered") {
		t.Errorf("machine error must not carry raw provider text:\n%s", out)
	}
}

// TestS3DetailFromError 锁定 provider 摘要提取的边界。
func TestS3DetailFromError(t *testing.T) {
	if _, found := s3.DetailFromError(nil); found {
		t.Error("nil error must not carry detail")
	}
	if _, found := s3.DetailFromError(os.ErrNotExist); found {
		t.Error("local error must not carry provider detail")
	}

	// smithy APIError 带 provider code
	apiErr := fmt.Errorf("wrap: %w", &fakeSmithyError{code: "SlowDown"})
	detail, found := s3.DetailFromError(apiErr)
	if !found || detail.ProviderCode == nil || *detail.ProviderCode != "SlowDown" {
		t.Fatalf("detail = %+v, found = %v", detail, found)
	}
	if !detail.Retryable {
		t.Error("SlowDown must be retryable")
	}
	if !detail.Throttled {
		t.Error("SlowDown must be marked throttled")
	}

	// 服务端临时故障可重试但不是限流
	detail, _ = s3.DetailFromError(&fakeSmithyError{code: "InternalError"})
	if !detail.Retryable || detail.Throttled {
		t.Errorf("InternalError must be retryable but not throttled, got %+v", detail)
	}

	// fs.ErrNotExist 不误判为 provider 错误
	var _ error = fs.ErrNotExist
}

// TestClassifyInvalidArgumentError Action 内 typed 参数错误必须归类为
// E_INVALID_ARGUMENT，而不是兜底 E_INTERNAL。
func TestClassifyInvalidArgumentError(t *testing.T) {
	info := classifyError(invalidArgument("需要提供 <src>"))
	if info.Code != CodeInvalidArgument {
		t.Fatalf("code = %q, want %q", info.Code, CodeInvalidArgument)
	}
	if info.Message != "需要提供 <src>" {
		t.Errorf("message = %q", info.Message)
	}

	wrapped := fmt.Errorf("compress: %w", invalidArgument("--in-place 不能与 <dst> 同时使用"))
	if info := classifyError(wrapped); info.Code != CodeInvalidArgument {
		t.Errorf("wrapped code = %q, want %q", info.Code, CodeInvalidArgument)
	}
}

// TestExecuteArgsActionArgErrorJSON --format json 下 Action 内的参数
// 错误（缺 operand）同样输出 E_INVALID_ARGUMENT。
func TestExecuteArgsActionArgErrorJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := ExecuteArgs(context.Background(), "test", []string{"itb", "inspect", "--format", "json"}, &stdout, &stderr)
	if !errors.Is(err, ErrReported) {
		t.Fatalf("err = %v, want ErrReported", err)
	}

	var decoded MachineError
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, stdout.String())
	}
	if decoded.Operation != "inspect" {
		t.Errorf("operation = %q, want inspect", decoded.Operation)
	}
	if decoded.Error.Code != CodeInvalidArgument {
		t.Errorf("code = %q, want %q", decoded.Error.Code, CodeInvalidArgument)
	}
	if !strings.Contains(decoded.Error.Message, "<src>") {
		t.Errorf("message = %q, want the operand hint", decoded.Error.Message)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty, got %q", stderr.String())
	}
}

// TestClassifyThrottledProviderError 限流 provider 错误稳定映射为
// E_THROTTLED；一般可重试的 5xx 保持 E_NETWORK。
func TestClassifyThrottledProviderError(t *testing.T) {
	info := classifyError(fmt.Errorf("wrap: %w", &fakeSmithyError{code: "SlowDown"}))
	if info.Code != CodeThrottled || !info.Retryable {
		t.Errorf("SlowDown classified = %+v", info)
	}
	if info.ProviderCode == nil || *info.ProviderCode != "SlowDown" {
		t.Errorf("provider_code = %v, want SlowDown", info.ProviderCode)
	}

	info = classifyError(fmt.Errorf("wrap: %w", &fakeSmithyError{code: "InternalError"}))
	if info.Code != CodeNetwork || !info.Retryable {
		t.Errorf("InternalError classified = %+v", info)
	}
}

// TestClassifyProviderInvalidCredentials provider 判定凭证无效（键不存在、
// 签名不匹配、token 过期）映射为 E_INVALID_CREDENTIALS。
func TestClassifyProviderInvalidCredentials(t *testing.T) {
	for _, code := range []string{"InvalidAccessKeyId", "SignatureDoesNotMatch", "ExpiredToken", "ExpiredTokenException"} {
		info := classifyError(s3.WrapError(&fakeSmithyError{code: code}))
		if info.Code != CodeInvalidCredentials {
			t.Errorf("%s code = %q, want %q", code, info.Code, CodeInvalidCredentials)
		}
		if info.ProviderCode == nil || *info.ProviderCode != code {
			t.Errorf("%s provider_code = %v, want %q", code, info.ProviderCode, code)
		}
	}
}

// TestExecuteArgsS3StatusClassifications 真实 HTTP 语义下的稳定错误码
// 映射：限流 → E_THROTTLED、5xx → E_NETWORK、403 → E_ACCESS_DENIED、
// 401/凭证码 → E_INVALID_CREDENTIALS、404 → E_OBJECT_NOT_FOUND。
func TestExecuteArgsS3StatusClassifications(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		xmlCode string
		want    string
		retry   bool
	}{
		{"限流", http.StatusServiceUnavailable, "SlowDown", CodeThrottled, true},
		{"服务端故障", http.StatusInternalServerError, "InternalError", CodeNetwork, true},
		{"拒绝访问", http.StatusForbidden, "AccessDenied", CodeAccessDenied, false},
		{"凭证无效", http.StatusUnauthorized, "InvalidAccessKeyId", CodeInvalidCredentials, false},
		{"对象不存在", http.StatusNotFound, "NoSuchKey", CodeObjectNotFound, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tt.status)
				fmt.Fprintf(w, `<Error><Code>%s</Code><Message>raw provider text</Message></Error>`, tt.xmlCode)
			}))
			t.Cleanup(srv.Close)

			var stdout, stderr bytes.Buffer
			err := ExecuteArgs(context.Background(), "test", []string{
				"itb", "s3", "list", "--format", "json", "--max-attempts", "1",
				"--endpoint", srv.URL,
				"--access-key", "ak",
				"--secret-key", "sk-secret-value",
				"--bucket", "test-bucket",
				"--force-path-style",
			}, &stdout, &stderr)
			if !errors.Is(err, ErrReported) {
				t.Fatalf("err = %v, want ErrReported", err)
			}

			var decoded MachineError
			if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
				t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
			}
			if decoded.Error.Code != tt.want {
				t.Errorf("code = %q, want %q", decoded.Error.Code, tt.want)
			}
			if decoded.Error.Retryable != tt.retry {
				t.Errorf("retryable = %v, want %v", decoded.Error.Retryable, tt.retry)
			}
			if decoded.Error.ProviderCode == nil || *decoded.Error.ProviderCode != tt.xmlCode {
				t.Errorf("provider_code = %v, want %q", decoded.Error.ProviderCode, tt.xmlCode)
			}
			if strings.Contains(stdout.String(), "raw provider text") || strings.Contains(stdout.String(), "sk-secret-value") {
				t.Errorf("machine error must stay sanitized:\n%s", stdout.String())
			}
		})
	}
}

type fakeSmithyError struct{ code string }

func (e *fakeSmithyError) Error() string        { return "api error " + e.code }
func (e *fakeSmithyError) ErrorCode() string    { return e.code }
func (e *fakeSmithyError) ErrorMessage() string { return "throttled" }
func (e *fakeSmithyError) ErrorFault() smithy.ErrorFault {
	return smithy.FaultClient
}
