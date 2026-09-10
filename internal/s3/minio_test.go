package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// 本测试针对真实 MinIO 验证 S3 兼容性（upload/stat/download/
// skip-existing/skip-unchanged/metadata/cache-control/overwrite/
// verify/delete + path-style）。MinIO 不可达时自动跳过：
//
//	ITB_TEST_MINIO_ENDPOINT     默认 http://127.0.0.1:9000
//	ITB_TEST_MINIO_ACCESS_KEY   默认 minioadmin
//	ITB_TEST_MINIO_SECRET_KEY   默认 minioadmin
//
// CI（.github/workflows/build-binaries.yml、release.yml）在 step 内以
// docker run 启动 MinIO（service container 不支持容器命令，无法传入
// server /data），并设置 ITB_REQUIRE_MINIO=1（strict 模式），使该测试
// 在每次 push 时真实执行，且 MinIO 起不来时必须失败而不是悄悄 skip，
// 防止 AWS SDK 升级或 S3 层重构在单测全部通过的情况下悄悄破坏
// MinIO 兼容性。

const minioTestBucket = "itb-test"

func minioTestConfig(t *testing.T) *Config {
	t.Helper()

	getenv := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}
	return &Config{
		Endpoint:        getenv("ITB_TEST_MINIO_ENDPOINT", "http://127.0.0.1:9000"),
		AccessKeyID:     getenv("ITB_TEST_MINIO_ACCESS_KEY", "minioadmin"),
		SecretAccessKey: getenv("ITB_TEST_MINIO_SECRET_KEY", "minioadmin"),
		Region:          "us-east-1",
		Bucket:          minioTestBucket,
		// MinIO 必须使用路径样式：整个测试套件都在 path-style 下运行
		ForcePathStyle: true,
	}
}

// skipIfMinIOUnreachable 以 TCP 探测 MinIO；不可达时默认跳过测试，
// 仅当 CI 设置 ITB_REQUIRE_MINIO=1（strict 模式）时改为失败——
// CI 的目标是持续验证 MinIO 兼容性，MinIO 起不来必须红而不是悄悄 skip。
// 只做连通性判断，鉴权问题留给后续真实断言暴露。
func skipIfMinIOUnreachable(t *testing.T, cfg *Config) {
	t.Helper()

	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		t.Skipf("invalid MinIO endpoint %q: %v", cfg.Endpoint, err)
	}
	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "https" {
			host = net.JoinHostPort(host, "443")
		} else {
			host = net.JoinHostPort(host, "80")
		}
	}
	conn, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		if minioRequired() {
			t.Fatalf("MinIO required (ITB_REQUIRE_MINIO=1) but unreachable at %s: %v", cfg.Endpoint, err)
		}
		t.Skipf("MinIO not reachable at %s (%v); start MinIO or set ITB_TEST_MINIO_ENDPOINT to run this integration test", cfg.Endpoint, err)
	}
	conn.Close()
}

// minioRequired 报告 MinIO 集成测试是否处于 strict 模式：CI workflow
// 设置 ITB_REQUIRE_MINIO=1，MinIO 不可达时测试必须失败；本地开发不设置，
// 不可达时优雅跳过。
func minioRequired() bool {
	return os.Getenv("ITB_REQUIRE_MINIO") == "1"
}

// TestMinIORequiredStrictMode 锁定 strict 模式的决策契约：
// 默认关闭（本地可跳过），ITB_REQUIRE_MINIO=1 时开启（CI 必须真实执行）。
func TestMinIORequiredStrictMode(t *testing.T) {
	t.Setenv("ITB_REQUIRE_MINIO", "")
	if minioRequired() {
		t.Fatal("minioRequired() = true by default, want false (local skip)")
	}

	t.Setenv("ITB_REQUIRE_MINIO", "1")
	if !minioRequired() {
		t.Fatal("minioRequired() = false with ITB_REQUIRE_MINIO=1, want true")
	}
}

// newMinIOTestClient 返回连接真实 MinIO 的客户端，并确保测试桶存在。
func newMinIOTestClient(t *testing.T) (*Client, string) {
	t.Helper()

	cfg := minioTestConfig(t)
	skipIfMinIOUnreachable(t, cfg)
	cfg.Normalize()

	ctx := context.Background()
	client, err := NewClient(ctx, cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if _, err := client.client.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: aws.String(client.bucket)}); err != nil {
		if _, cerr := client.client.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String(client.bucket)}); cerr != nil {
			t.Fatalf("create bucket %s: %v (HeadBucket: %v)", client.bucket, cerr, err)
		}
	}

	// 每次运行使用唯一 key 前缀，避免并行/重复运行的键冲突
	prefix := "itb-integration/" + strconv.FormatInt(time.Now().UnixNano(), 36) + "/"
	return client, prefix
}

func TestMinIOIntegration(t *testing.T) {
	client, prefix := newMinIOTestClient(t)
	ctx := context.Background()

	uploadFixture := func(name, content string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		return path
	}

	var (
		basicKey  = prefix + "hello.txt"
		basicPath string
	)

	t.Run("upload with metadata and cache-control", func(t *testing.T) {
		basicPath = uploadFixture("hello.txt", helloContent)
		result, err := Upload(ctx, client, basicPath, basicKey, &UploadOptions{
			CacheControl: "no-cache",
			Metadata:     map[string]string{"source-sha256": helloSHA256, "width": "1920"},
			Verify:       true, // PUT → HEAD：远端属性与本次上传一致
		})
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if result.Skipped || result.SHA256 != helloSHA256 {
			t.Fatalf("result = %+v", result)
		}
	})

	t.Run("stat reflects upload", func(t *testing.T) {
		info, err := Stat(ctx, client, basicKey)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if info.Size != int64(len(helloContent)) {
			t.Errorf("size = %d, want %d", info.Size, len(helloContent))
		}
		if info.ContentType != "text/plain; charset=utf-8" {
			t.Errorf("content-type = %q, want text/plain（内容检测，MinIO 回读）", info.ContentType)
		}
		if info.CacheControl != "no-cache" {
			t.Errorf("cache-control = %q, want no-cache", info.CacheControl)
		}
		if info.Metadata[MetadataSHA256Key] != helloSHA256 {
			t.Errorf("metadata itb-sha256 = %q, want %q", info.Metadata[MetadataSHA256Key], helloSHA256)
		}
		if info.Metadata["width"] != "1920" {
			t.Errorf("metadata width = %q, want 1920", info.Metadata["width"])
		}
	})

	t.Run("skip-existing skips second upload", func(t *testing.T) {
		result, err := Upload(ctx, client, basicPath, basicKey, &UploadOptions{SkipExisting: true})
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if !result.Skipped {
			t.Fatal("expected skip on existing key")
		}
	})

	t.Run("skip-unchanged skips identical content", func(t *testing.T) {
		result, err := Upload(ctx, client, basicPath, basicKey, &UploadOptions{SkipUnchanged: true})
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if !result.Skipped {
			t.Fatal("expected skip on unchanged content")
		}
	})

	t.Run("download with metadata verify", func(t *testing.T) {
		output := filepath.Join(t.TempDir(), "downloaded.txt")
		result, err := Download(ctx, client, basicKey, output, &DownloadOptions{Verify: true})
		if err != nil {
			t.Fatalf("Download --verify: %v", err)
		}
		if result.SHA256 != helloSHA256 {
			t.Errorf("downloaded sha256 = %q, want %q", result.SHA256, helloSHA256)
		}
		got, err := os.ReadFile(output)
		if err != nil {
			t.Fatalf("read downloaded: %v", err)
		}
		if string(got) != helloContent {
			t.Errorf("downloaded content = %q, want %q", got, helloContent)
		}
	})

	t.Run("overwrite replaces content and verify-sha256 validates", func(t *testing.T) {
		replacement := "replaced\n"
		path := uploadFixture("replaced.txt", replacement)
		result, err := Upload(ctx, client, path, basicKey, nil)
		if err != nil {
			t.Fatalf("Upload (overwrite): %v", err)
		}
		if result.Skipped {
			t.Fatal("default upload must overwrite, not skip")
		}

		output := filepath.Join(t.TempDir(), "overwritten.txt")
		if _, err := Download(ctx, client, basicKey, output, &DownloadOptions{VerifySHA256: result.SHA256}); err != nil {
			t.Fatalf("Download --verify-sha256: %v", err)
		}
		got, err := os.ReadFile(output)
		if err != nil {
			t.Fatalf("read downloaded: %v", err)
		}
		if string(got) != replacement {
			t.Errorf("content after overwrite = %q, want %q", got, replacement)
		}

		// 旧内容哈希必须失配（provider-neutral 校验真实生效）
		stale := filepath.Join(t.TempDir(), "stale.txt")
		if _, err := Download(ctx, client, basicKey, stale, &DownloadOptions{VerifySHA256: helloSHA256}); err == nil || !strings.Contains(err.Error(), ErrChecksumMismatch.Error()) {
			t.Fatalf("expected checksum mismatch for stale hash, got %v", err)
		}
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Errorf("stale download must not leave a partial file, stat err = %v", err)
		}
	})

	// ---- Commit 11 收口：分页 / skip-matching / 条件上传 / 期望值 ----

	t.Run("list pagination across pages", func(t *testing.T) {
		// 3 个对象 + PageSize 2 强制两页，不必创建 1001 个对象
		pagePrefix := prefix + "page/"
		uploaded := make([]string, 0, 3)
		for i := range 3 {
			key := pagePrefix + "obj-" + strconv.Itoa(i) + ".txt"
			path := uploadFixture("page-obj.txt", helloContent)
			if _, err := Upload(ctx, client, path, key, nil); err != nil {
				t.Fatalf("seed page object %d: %v", i, err)
			}
			uploaded = append(uploaded, key)
		}

		result, err := List(ctx, client, &ListOptions{Prefix: pagePrefix, PageSize: 2, All: true})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if result.Count != 3 || result.Pages != 2 || !result.Complete {
			t.Fatalf("result = count %d, pages %d, complete %v; want 3/2/true", result.Count, result.Pages, result.Complete)
		}

		singlePage, err := List(ctx, client, &ListOptions{Prefix: pagePrefix, PageSize: 2})
		if err != nil {
			t.Fatalf("List single page: %v", err)
		}
		if singlePage.Count != 2 || singlePage.Complete || singlePage.NextContinuationToken == "" {
			t.Fatalf("single page = count %d complete %v token %q", singlePage.Count, singlePage.Complete, singlePage.NextContinuationToken)
		}
	})

	t.Run("skip-matching reuses identical object", func(t *testing.T) {
		path := uploadFixture("match.txt", helloContent)
		key := prefix + "match.txt"
		if _, err := Upload(ctx, client, path, key, &UploadOptions{
			CacheControl: "no-cache",
			Metadata:     map[string]string{"width": "1920"},
		}); err != nil {
			t.Fatalf("seed upload: %v", err)
		}

		result, err := Upload(ctx, client, path, key, &UploadOptions{
			SkipMatching: true,
			CacheControl: "no-cache",
			Metadata:     map[string]string{"width": "1920"},
		})
		if err != nil {
			t.Fatalf("Upload skip-matching: %v", err)
		}
		if result.Status != StatusReused || !result.Skipped {
			t.Fatalf("result = %+v, want reused", result)
		}
	})

	t.Run("conditional if-exists verify upload", func(t *testing.T) {
		key := prefix + "immutable.bin"
		path := uploadFixture("immutable.txt", helloContent)

		created, err := Upload(ctx, client, path, key, &UploadOptions{IfExists: IfExistsVerify})
		if err != nil {
			t.Fatalf("conditional create: %v", err)
		}
		if created.Status != StatusUploaded {
			t.Fatalf("first upload status = %q, want uploaded", created.Status)
		}

		// 相同内容：412 → HEAD 匹配 → reused
		reused, err := Upload(ctx, client, path, key, &UploadOptions{IfExists: IfExistsVerify})
		if err != nil {
			t.Fatalf("conditional reuse: %v", err)
		}
		if reused.Status != StatusReused {
			t.Fatalf("second upload status = %q, want reused", reused.Status)
		}

		// 不同内容：412 → HEAD 不匹配 → E_TARGET_CONFLICT，远端不被覆盖
		other := uploadFixture("other.txt", "other content")
		if _, err := Upload(ctx, client, other, key, &UploadOptions{IfExists: IfExistsVerify}); err == nil || !strings.Contains(err.Error(), ErrExpectationMismatch.Error()) {
			t.Fatalf("err = %v, want expectation mismatch", err)
		}
		info, err := Stat(ctx, client, key)
		if err != nil {
			t.Fatalf("stat after conflict: %v", err)
		}
		if info.Metadata[MetadataSHA256Key] != helloSHA256 {
			t.Errorf("immutable object was overwritten: sha = %q", info.Metadata[MetadataSHA256Key])
		}
	})

	t.Run("download expect-size and content-type", func(t *testing.T) {
		// 自带对象：basicKey 经 overwrite 后内容已变，子测试不得依赖
		// 其历史状态
		key := prefix + "expect.txt"
		path := uploadFixture("expect.txt", helloContent)
		if _, err := Upload(ctx, client, path, key, nil); err != nil {
			t.Fatalf("seed upload: %v", err)
		}

		output := filepath.Join(t.TempDir(), "expected.txt")
		good := int64(len(helloContent))
		if _, err := Download(ctx, client, key, output, &DownloadOptions{
			ExpectSize:        &good,
			ExpectContentType: "text/plain",
		}); err != nil {
			t.Fatalf("Download with expectations: %v", err)
		}

		wrong := good + 1
		if _, err := Download(ctx, client, key, filepath.Join(t.TempDir(), "bad.txt"), &DownloadOptions{
			ExpectSize: &wrong,
		}); err == nil || !strings.Contains(err.Error(), ErrExpectationMismatch.Error()) {
			t.Fatalf("err = %v, want expectation mismatch", err)
		}
	})

	t.Run("download reuses verified local copy", func(t *testing.T) {
		key := prefix + "reuse.txt"
		path := uploadFixture("reuse.txt", helloContent)
		if _, err := Upload(ctx, client, path, key, nil); err != nil {
			t.Fatalf("seed upload: %v", err)
		}

		output := filepath.Join(t.TempDir(), "local.txt")
		if err := os.WriteFile(output, []byte(helloContent), 0o644); err != nil {
			t.Fatalf("seed local copy: %v", err)
		}

		result, err := Download(ctx, client, key, output, &DownloadOptions{
			VerifySHA256: helloSHA256,
			IfExists:     IfExistsVerify,
		})
		if err != nil {
			t.Fatalf("Download reuse: %v", err)
		}
		if result.Status != StatusReused {
			t.Fatalf("status = %q, want reused", result.Status)
		}

		// 不一致的本地副本必须报冲突而不是复用
		divergent := filepath.Join(t.TempDir(), "divergent.txt")
		if err := os.WriteFile(divergent, []byte("divergent"), 0o644); err != nil {
			t.Fatalf("seed divergent copy: %v", err)
		}
		if _, err := Download(ctx, client, key, divergent, &DownloadOptions{
			VerifySHA256: helloSHA256,
			IfExists:     IfExistsVerify,
		}); err == nil || !strings.Contains(err.Error(), ErrExpectationMismatch.Error()) {
			t.Fatalf("err = %v, want expectation mismatch", err)
		}
	})

	// 破坏性操作放在最后：basicKey 被此前的 download 子测试依赖，
	// 过早删除会让后续子测试误报"对象不存在"（该顺序问题曾被
	// Codecov 阻断的 CI 掩盖）
	t.Run("delete removes object", func(t *testing.T) {
		if err := Delete(ctx, client, basicKey, nil); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := Stat(ctx, client, basicKey); err == nil || !strings.Contains(err.Error(), ErrObjectNotFound.Error()) {
			t.Fatalf("expected not found after delete, got %v", err)
		}
	})
}

// TestMinIOCLIE2E 通过编译后的真实 itb 二进制验证 positional operand
// 接线，而不是只验证领域 API。
func TestMinIOCLIE2E(t *testing.T) {
	client, prefix := newMinIOTestClient(t)
	cfg := minioTestConfig(t)

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "itb")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build itb binary: %v\n%s", err, output)
	}

	tmp := t.TempDir()
	fixture := filepath.Join(tmp, "fixture.txt")
	if err := os.WriteFile(fixture, []byte(helloContent), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	baseEnv := append(os.Environ(),
		"ITB_S3_ENDPOINT="+cfg.Endpoint,
		"ITB_S3_ACCESS_KEY_ID="+cfg.AccessKeyID,
		"ITB_S3_SECRET_ACCESS_KEY="+cfg.SecretAccessKey,
		"ITB_S3_REGION="+cfg.Region,
		"ITB_S3_BUCKET="+cfg.Bucket,
		"ITB_S3_FORCE_PATH_STYLE=true",
	)
	type cliResult struct {
		stdout string
		stderr string
	}
	run := func(dir string, args ...string) cliResult {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = baseEnv
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			t.Fatalf("itb %s: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
		}
		return cliResult{stdout: stdout.String(), stderr: stderr.String()}
	}

	key := prefix + "fixture.txt"
	t.Cleanup(func() { _ = Delete(context.Background(), client, key, nil) })
	var upload struct {
		SchemaVersion string `json:"schema_version"`
		Key           string `json:"key"`
		Size          int64  `json:"size"`
		SHA256        string `json:"sha256"`
	}
	if err := json.Unmarshal([]byte(run(tmp, "s3", "upload", "--format", "json", fixture, key).stdout), &upload); err != nil {
		t.Fatalf("decode upload JSON: %v", err)
	}
	if upload.SchemaVersion != UploadSchemaVersion || upload.Key != key || upload.Size != int64(len(helloContent)) || upload.SHA256 != helloSHA256 {
		t.Fatalf("upload result = %+v", upload)
	}

	var stat struct {
		SchemaVersion string `json:"schema_version"`
		Key           string `json:"key"`
		Size          int64  `json:"size"`
	}
	if err := json.Unmarshal([]byte(run(tmp, "s3", "stat", "--format", "json", key).stdout), &stat); err != nil {
		t.Fatalf("decode stat JSON: %v", err)
	}
	if stat.SchemaVersion != StatSchemaVersion || stat.Key != key || stat.Size != int64(len(helloContent)) {
		t.Fatalf("stat result = %+v", stat)
	}

	downloaded := filepath.Join(tmp, "downloaded.txt")
	var download struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal([]byte(run(tmp, "s3", "download", "--verify", "--format", "json", key, downloaded).stdout), &download); err != nil {
		t.Fatalf("decode download JSON: %v", err)
	}
	if download.SchemaVersion != DownloadSchemaVersion {
		t.Fatalf("download schema_version = %q, want %q", download.SchemaVersion, DownloadSchemaVersion)
	}
	got, err := os.ReadFile(downloaded)
	if err != nil || string(got) != helloContent {
		t.Fatalf("downloaded content = %q, read error = %v", got, err)
	}
	if output := run(tmp, "s3", "list", "--format", "plain", prefix).stdout; !strings.Contains(output, key) {
		t.Fatalf("list output missing %q: %s", key, output)
	}
	var emptyList ListResult
	emptyPrefix := prefix + "empty/"
	if err := json.Unmarshal([]byte(run(tmp, "s3", "list", "--format", "json", emptyPrefix).stdout), &emptyList); err != nil {
		t.Fatalf("decode empty list JSON: %v", err)
	}
	if len(emptyList.Objects) != 0 || !emptyList.Complete || emptyList.SchemaVersion != ListSchemaVersion {
		t.Fatalf("empty list = %+v, want no objects with complete=true", emptyList)
	}

	var skipped struct {
		Skipped bool `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(run(tmp, "s3", "upload", "--skip-existing", "--format", "json", fixture, key).stdout), &skipped); err != nil {
		t.Fatalf("decode skipped upload JSON: %v", err)
	}
	if !skipped.Skipped {
		t.Fatalf("skip-existing result = %+v", skipped)
	}

	run(tmp, "s3", "delete", "-f", key)
	cmd := exec.Command(binary, "s3", "stat", key)
	cmd.Dir = tmp
	cmd.Env = baseEnv
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("stat after delete unexpectedly succeeded: %s", output)
	}

	// 默认 operand：upload <src> 使用 basename，download <key> 写入当前目录。
	// 文件名包含唯一后缀，避免覆盖自建 MinIO 测试桶中的固定对象。
	defaultName := "itb-cli-default-" + strconv.FormatInt(time.Now().UnixNano(), 36) + ".txt"
	defaultFixture := filepath.Join(tmp, defaultName)
	t.Cleanup(func() { _ = Delete(context.Background(), client, defaultName, nil) })
	if err := os.WriteFile(defaultFixture, []byte(helloContent), 0o644); err != nil {
		t.Fatalf("write default fixture: %v", err)
	}
	run(tmp, "s3", "upload", defaultName)
	if err := os.Remove(defaultFixture); err != nil {
		t.Fatalf("remove fixture before default download: %v", err)
	}
	run(tmp, "s3", "download", defaultName)
	if got, err := os.ReadFile(defaultFixture); err != nil || string(got) != helloContent {
		t.Fatalf("default downloaded content = %q, read error = %v", got, err)
	}
	run(tmp, "s3", "delete", "-f", defaultName)

	// ---- Commit 11 收口：stdout/stderr 契约与 CLI 分页 ----

	// 成功：stdout 恰好一份 JSON 文档
	successOut := run(tmp, "s3", "upload", "--format", "json", fixture, key)
	trimmed := strings.TrimSpace(successOut.stdout)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		t.Fatalf("success stdout must be one JSON document:\n%s", successOut.stdout)
	}
	if strings.Contains(successOut.stderr, "Upload completed") {
		t.Errorf("results must not leak to stderr: %q", successOut.stderr)
	}

	// 失败：stdout 恰好一份 itb.error.v1，stderr 无重复 raw error
	failCmd := exec.Command(binary, "s3", "stat", "--format", "json", prefix+"missing-object.bin")
	failCmd.Dir = tmp
	failCmd.Env = baseEnv
	var failOut, failErr bytes.Buffer
	failCmd.Stdout = &failOut
	failCmd.Stderr = &failErr
	if err := failCmd.Run(); err == nil {
		t.Fatal("stat of missing object must fail")
	}
	failTrimmed := strings.TrimSpace(failOut.String())
	var machineError struct {
		SchemaVersion string `json:"schema_version"`
		Operation     string `json:"operation"`
		Error         struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(failTrimmed), &machineError); err != nil {
		t.Fatalf("failure stdout is not one JSON document: %v\n%s", err, failOut.String())
	}
	if machineError.SchemaVersion != "itb.error.v1" || machineError.Operation != "s3.stat" || machineError.Error.Code != "E_OBJECT_NOT_FOUND" {
		t.Fatalf("machine error = %+v", machineError)
	}
	if strings.Contains(failErr.String(), "E_OBJECT_NOT_FOUND") {
		t.Errorf("stderr must not duplicate the machine error: %q", failErr.String())
	}

	// CLI list 分页：3 个对象 + --page-size 2 --all 强制两页
	pagePrefix := prefix + "clipage/"
	for i := range 3 {
		path := filepath.Join(tmp, "clipage.txt")
		if err := os.WriteFile(path, []byte(helloContent), 0o644); err != nil {
			t.Fatalf("write page fixture: %v", err)
		}
		run(tmp, "s3", "upload", path, pagePrefix+"obj-"+strconv.Itoa(i)+".txt")
	}
	var paged struct {
		Count    int  `json:"count"`
		Pages    int  `json:"pages"`
		Complete bool `json:"complete"`
	}
	if err := json.Unmarshal([]byte(run(tmp, "s3", "list", "--format", "json", "--page-size", "2", "--all", pagePrefix).stdout), &paged); err != nil {
		t.Fatalf("decode paged list JSON: %v", err)
	}
	if paged.Count != 3 || paged.Pages != 2 || !paged.Complete {
		t.Fatalf("paged list = %+v, want count 3 / pages 2 / complete", paged)
	}
}
