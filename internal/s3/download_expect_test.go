package s3

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// newDownloadHeaderServer 启动可自定义 Content-Length / Content-Type
// 的 GET 模拟服务器，用于期望值检查测试。
func newDownloadHeaderServer(t *testing.T, body []byte, declaredSize *int64, contentType string) (*requestRecorder, *Client) {
	t.Helper()

	rec := &requestRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		size := int64(len(body))
		if declaredSize != nil {
			size = *declaredSize
		}
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		rec.record(r.Method, "")
	}))
	t.Cleanup(srv.Close)

	cfg := &Config{
		Endpoint: srv.URL, AccessKeyID: "ak", SecretAccessKey: "sk",
		Region: "us-east-1", Bucket: "test-bucket", ForcePathStyle: true,
	}
	client, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return rec, client
}

func int64Ptr(v int64) *int64 { return &v }

// TestDownloadExpectSizeHeaderMismatch 响应头阶段 size 不一致：
// 在创建目标文件之前失败，不留任何文件。
func TestDownloadExpectSizeHeaderMismatch(t *testing.T) {
	rec, client := newDownloadHeaderServer(t, []byte(helloContent), nil, "")
	output := filepath.Join(t.TempDir(), "out.txt")

	_, err := Download(context.Background(), client, "hello.txt", output, &DownloadOptions{ExpectSize: int64Ptr(999)})
	if err == nil || !containsError(err, ErrExpectationMismatch) {
		t.Fatalf("err = %v, want ErrExpectationMismatch", err)
	}
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Errorf("output must not exist, stat err = %v", statErr)
	}
	assertMethods(t, rec.snapshotMethods(), []string{http.MethodGet})
}

// TestDownloadExpectSizeActualBytesMismatch chunked 响应（无
// Content-Length 声明）时，期望值检查落在实际写入字节数上。
func TestDownloadExpectSizeActualBytesMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 不设置 Content-Length：走 chunked 编码，无响应头可查
		_, _ = w.Write([]byte(helloContent))
	}))
	t.Cleanup(srv.Close)

	cfg := &Config{
		Endpoint: srv.URL, AccessKeyID: "ak", SecretAccessKey: "sk",
		Region: "us-east-1", Bucket: "test-bucket", ForcePathStyle: true,
	}
	client, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	output := filepath.Join(t.TempDir(), "out.txt")
	_, err = Download(context.Background(), client, "hello.txt", output, &DownloadOptions{ExpectSize: int64Ptr(20)})
	if err == nil || !containsError(err, ErrExpectationMismatch) {
		t.Fatalf("err = %v, want ErrExpectationMismatch", err)
	}
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Errorf("no partial file may remain, stat err = %v", statErr)
	}
}

// TestDownloadExpectContentType 期望 Content-Type：参数与大小写不敏感。
func TestDownloadExpectContentType(t *testing.T) {
	t.Run("参数差异不敏感", func(t *testing.T) {
		_, client := newDownloadHeaderServer(t, []byte(helloContent), nil, "text/plain; charset=utf-8")
		output := filepath.Join(t.TempDir(), "out.txt")
		if _, err := Download(context.Background(), client, "k", output, &DownloadOptions{ExpectContentType: "Text/Plain"}); err != nil {
			t.Fatalf("Download: %v", err)
		}
	})

	t.Run("类型不一致报错", func(t *testing.T) {
		_, client := newDownloadHeaderServer(t, []byte(helloContent), nil, "text/html")
		output := filepath.Join(t.TempDir(), "out.txt")
		_, err := Download(context.Background(), client, "k", output, &DownloadOptions{ExpectContentType: "image/png"})
		if err == nil || !containsError(err, ErrExpectationMismatch) {
			t.Fatalf("err = %v, want ErrExpectationMismatch", err)
		}
	})
}

// TestDownloadZeroByteObjectWithExpectSize 0 字节对象合法：
// --expect-size 0 必须能成功下载。
func TestDownloadZeroByteObjectWithExpectSize(t *testing.T) {
	_, client := newDownloadHeaderServer(t, []byte{}, int64Ptr(0), "")
	output := filepath.Join(t.TempDir(), "empty.bin")

	result, err := Download(context.Background(), client, "empty.bin", output, &DownloadOptions{ExpectSize: int64Ptr(0)})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if result.Size != 0 {
		t.Errorf("size = %d, want 0", result.Size)
	}
}

// TestDownloadReuseLocalCopy --if-exists=verify：本地副本一致时复用，
// 0 × GET，status=reused。
func TestDownloadReuseLocalCopy(t *testing.T) {
	rec, client := newDownloadTestServer(t, []byte(helloContent))
	output := filepath.Join(t.TempDir(), "local.txt")
	if err := os.WriteFile(output, []byte(helloContent), 0o644); err != nil {
		t.Fatalf("seed local copy: %v", err)
	}

	result, err := Download(context.Background(), client, "hello.txt", output, &DownloadOptions{
		VerifySHA256: helloSHA256,
		IfExists:     IfExistsVerify,
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if result.Status != StatusReused {
		t.Fatalf("status = %q, want reused", result.Status)
	}
	if result.Size != int64(len(helloContent)) || result.SHA256 != helloSHA256 {
		t.Fatalf("result = %+v", result)
	}
	assertMethods(t, rec.snapshotMethods(), nil) // 0 × GET
}

// TestDownloadReuseDetectsDivergentLocalCopy 本地副本存在但不一致：
// ErrExpectationMismatch，不执行 GET。
func TestDownloadReuseDetectsDivergentLocalCopy(t *testing.T) {
	rec, client := newDownloadTestServer(t, []byte(helloContent))
	output := filepath.Join(t.TempDir(), "local.txt")
	if err := os.WriteFile(output, []byte("divergent"), 0o644); err != nil {
		t.Fatalf("seed local copy: %v", err)
	}

	_, err := Download(context.Background(), client, "hello.txt", output, &DownloadOptions{
		VerifySHA256: helloSHA256,
		IfExists:     IfExistsVerify,
	})
	if err == nil || !containsError(err, ErrExpectationMismatch) {
		t.Fatalf("err = %v, want ErrExpectationMismatch", err)
	}
	assertMethods(t, rec.snapshotMethods(), nil)
	// 本地内容保持原状
	if got, _ := os.ReadFile(output); string(got) != "divergent" {
		t.Errorf("local copy content = %q, want untouched", got)
	}
}

// TestDownloadReuseMissingLocalFallsBackToGet 本地不存在：正常下载。
func TestDownloadReuseMissingLocalFallsBackToGet(t *testing.T) {
	rec, client := newDownloadTestServer(t, []byte(helloContent))
	output := filepath.Join(t.TempDir(), "fresh.txt")

	result, err := Download(context.Background(), client, "hello.txt", output, &DownloadOptions{
		VerifySHA256: helloSHA256,
		IfExists:     IfExistsVerify,
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if result.Status != StatusDownloaded {
		t.Fatalf("status = %q, want downloaded", result.Status)
	}
	assertMethods(t, rec.snapshotMethods(), []string{http.MethodGet})
}

// TestDownloadReuseRequiresVerificationBasis 没有校验依据时直接报错，
// 绝不"文件存在就复用"。
func TestDownloadReuseRequiresVerificationBasis(t *testing.T) {
	rec, client := newDownloadTestServer(t, []byte(helloContent))
	output := filepath.Join(t.TempDir(), "local.txt")
	if err := os.WriteFile(output, []byte(helloContent), 0o644); err != nil {
		t.Fatalf("seed local copy: %v", err)
	}

	_, err := Download(context.Background(), client, "hello.txt", output, &DownloadOptions{IfExists: IfExistsVerify})
	if err == nil || !containsError(err, ErrReuseVerificationUnavailable) {
		t.Fatalf("err = %v, want ErrReuseVerificationUnavailable", err)
	}
	assertMethods(t, rec.snapshotMethods(), nil)
}

// TestDownloadReuseViaVerifyMetadata 只有 --verify：HEAD 获取远端
// itb-sha256 作为期望值后复用，0 × GET。
func TestDownloadReuseViaVerifyMetadata(t *testing.T) {
	state, client := newUploadVerifyTestServer(t)
	// 预置远端对象（带 itb-sha256）
	if _, err := Upload(context.Background(), client, writeUploadFixture(t), "hello.txt", nil); err != nil {
		t.Fatalf("seed upload: %v", err)
	}
	state.mu.Lock()
	state.methods = nil
	state.mu.Unlock()

	output := filepath.Join(t.TempDir(), "local.txt")
	if err := os.WriteFile(output, []byte(helloContent), 0o644); err != nil {
		t.Fatalf("seed local copy: %v", err)
	}

	result, err := Download(context.Background(), client, "hello.txt", output, &DownloadOptions{
		Verify:   true,
		IfExists: IfExistsVerify,
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if result.Status != StatusReused || result.SHA256 != helloSHA256 {
		t.Fatalf("result = %+v, want reused with sha", result)
	}
	methods := state.snapshotMethods()
	for _, m := range methods {
		if m == http.MethodGet {
			t.Fatalf("reuse must not GET, methods = %v", methods)
		}
	}
}

// TestDownloadInvalidIfExists 非法 --if-exists 取值在参数阶段报错。
func TestDownloadInvalidIfExists(t *testing.T) {
	_, client := newDownloadTestServer(t, []byte(helloContent))
	output := filepath.Join(t.TempDir(), "out.txt")

	_, err := Download(context.Background(), client, "hello.txt", output, &DownloadOptions{IfExists: "skip"})
	if err == nil || !containsError(err, ErrInvalidIfExists) {
		t.Fatalf("err = %v, want ErrInvalidIfExists", err)
	}
}

// TestDownloadV2ResultSchema v2 结果契约：status/content_type 字段。
func TestDownloadV2ResultSchema(t *testing.T) {
	_, client := newDownloadHeaderServer(t, []byte(helloContent), nil, "text/plain; charset=utf-8")
	output := filepath.Join(t.TempDir(), "out.txt")

	result, err := Download(context.Background(), client, "hello.txt", output, nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if result.SchemaVersion != "itb.s3.download.v2" {
		t.Errorf("schema_version = %q, want itb.s3.download.v2", result.SchemaVersion)
	}
	if result.Status != StatusDownloaded {
		t.Errorf("status = %q, want downloaded", result.Status)
	}
	if result.ContentType != "text/plain; charset=utf-8" {
		t.Errorf("content_type = %q", result.ContentType)
	}
	// 未启用校验选项时 sha256 留空（不额外计算，与 v1 行为一致）
	if result.SHA256 != "" {
		t.Errorf("sha256 = %q, want empty without verification options", result.SHA256)
	}
}

func containsError(err error, target error) bool {
	return errors.Is(err, target)
}

// seedRemoteObject 预置一个带 itb-sha256 的远端对象并清空方法记录，
// 供复用组合语义测试断言后续请求序列。
func seedRemoteObject(t *testing.T, contentType string, tamper func(h http.Header, size *int64)) (*objectState, *Client) {
	t.Helper()

	state, client := newUploadVerifyTestServer(t)
	state.tamper = tamper
	if _, err := Upload(context.Background(), client, writeUploadFixture(t), "hello.txt", &UploadOptions{ContentType: contentType}); err != nil {
		t.Fatalf("seed upload: %v", err)
	}
	state.mu.Lock()
	state.methods = nil
	state.mu.Unlock()
	return state, client
}

// TestDownloadReuseVerifyCombinationMustHead --verify 与 --verify-sha256
// 同时提供时复用必须执行 HEAD，本地副本要同时满足两个依据：
// 远端 itb-sha256 与显式 digest 互相矛盾时直接失败，绝不 GET。
func TestDownloadReuseVerifyCombinationMustHead(t *testing.T) {
	t.Run("两者一致且本地匹配则复用", func(t *testing.T) {
		state, client := seedRemoteObject(t, "text/plain", nil)
		output := filepath.Join(t.TempDir(), "local.txt")
		if err := os.WriteFile(output, []byte(helloContent), 0o644); err != nil {
			t.Fatalf("seed local copy: %v", err)
		}

		result, err := Download(context.Background(), client, "hello.txt", output, &DownloadOptions{
			Verify:       true,
			VerifySHA256: helloSHA256,
			IfExists:     IfExistsVerify,
		})
		if err != nil {
			t.Fatalf("Download: %v", err)
		}
		if result.Status != StatusReused || result.SHA256 != helloSHA256 {
			t.Fatalf("result = %+v, want reused", result)
		}
		assertMethods(t, state.snapshotMethods(), []string{http.MethodHead})
	})

	t.Run("远端 metadata 与显式 digest 矛盾则失败", func(t *testing.T) {
		state, client := seedRemoteObject(t, "text/plain", func(h http.Header, size *int64) {
			h.Set("x-amz-meta-itb-sha256", "deadbeef")
		})
		output := filepath.Join(t.TempDir(), "local.txt")
		if err := os.WriteFile(output, []byte(helloContent), 0o644); err != nil {
			t.Fatalf("seed local copy: %v", err)
		}

		_, err := Download(context.Background(), client, "hello.txt", output, &DownloadOptions{
			Verify:       true,
			VerifySHA256: helloSHA256,
			IfExists:     IfExistsVerify,
		})
		if err == nil || !containsError(err, ErrExpectationMismatch) {
			t.Fatalf("err = %v, want ErrExpectationMismatch", err)
		}
		assertMethods(t, state.snapshotMethods(), []string{http.MethodHead})
		// 本地内容保持原状
		if got, _ := os.ReadFile(output); string(got) != helloContent {
			t.Errorf("local copy content = %q, want untouched", got)
		}
	})
}

// TestDownloadReuseExpectContentTypeMustHead --expect-content-type 与
// --verify-sha256 组合：复用必须 HEAD 并校验远端 Content-Type，
// 不得因有显式 digest 而跳过。
func TestDownloadReuseExpectContentTypeMustHead(t *testing.T) {
	t.Run("类型不一致则失败", func(t *testing.T) {
		state, client := seedRemoteObject(t, "text/plain", nil)
		output := filepath.Join(t.TempDir(), "local.txt")
		if err := os.WriteFile(output, []byte(helloContent), 0o644); err != nil {
			t.Fatalf("seed local copy: %v", err)
		}

		_, err := Download(context.Background(), client, "hello.txt", output, &DownloadOptions{
			VerifySHA256:      helloSHA256,
			ExpectContentType: "image/png",
			IfExists:          IfExistsVerify,
		})
		if err == nil || !containsError(err, ErrExpectationMismatch) {
			t.Fatalf("err = %v, want ErrExpectationMismatch", err)
		}
		assertMethods(t, state.snapshotMethods(), []string{http.MethodHead})
	})

	t.Run("类型一致（参数不敏感）则复用并回填 content_type", func(t *testing.T) {
		state, client := seedRemoteObject(t, "text/plain; charset=utf-8", nil)
		output := filepath.Join(t.TempDir(), "local.txt")
		if err := os.WriteFile(output, []byte(helloContent), 0o644); err != nil {
			t.Fatalf("seed local copy: %v", err)
		}

		result, err := Download(context.Background(), client, "hello.txt", output, &DownloadOptions{
			VerifySHA256:      helloSHA256,
			ExpectContentType: "Text/Plain",
			IfExists:          IfExistsVerify,
		})
		if err != nil {
			t.Fatalf("Download: %v", err)
		}
		if result.Status != StatusReused {
			t.Fatalf("status = %q, want reused", result.Status)
		}
		if result.ContentType != "text/plain; charset=utf-8" {
			t.Errorf("content_type = %q, want remote value", result.ContentType)
		}
		assertMethods(t, state.snapshotMethods(), []string{http.MethodHead})
	})
}

// TestDownloadReuseVerifySHA256OnlyStaysZeroNetwork 只有 --verify-sha256
// （+可选 --expect-size）保持零网络复用：0 × HEAD、0 × GET。
func TestDownloadReuseVerifySHA256OnlyStaysZeroNetwork(t *testing.T) {
	rec, client := newDownloadTestServer(t, []byte(helloContent))
	output := filepath.Join(t.TempDir(), "local.txt")
	if err := os.WriteFile(output, []byte(helloContent), 0o644); err != nil {
		t.Fatalf("seed local copy: %v", err)
	}

	result, err := Download(context.Background(), client, "hello.txt", output, &DownloadOptions{
		VerifySHA256: helloSHA256,
		ExpectSize:   int64Ptr(int64(len(helloContent))),
		IfExists:     IfExistsVerify,
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if result.Status != StatusReused {
		t.Fatalf("status = %q, want reused", result.Status)
	}
	assertMethods(t, rec.snapshotMethods(), nil)
}
