package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"

	"github.com/deepteams/webp"

	"imagetoolbox/internal/filehash"
)

// 本文件是编译后 itb 二进制的 inspect / 错误契约 E2E。
// 它不依赖 MinIO 与真实压缩器：inspect 与错误输出是纯 Go 路径，
// 在任何 checkout（含未注入原生压缩器的全新构建）都必须真实执行。

// buildE2EBinary 编译 itb 二进制并返回路径。
func buildE2EBinary(t *testing.T) string {
	t.Helper()

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "itb")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build itb binary: %v\n%s", err, output)
	}
	return binary
}

// runE2E 执行二进制并返回 stdout/stderr 与退出状态。
func runE2E(t *testing.T, binary, dir string, args ...string) (string, string, int) {
	t.Helper()

	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("itb %s: %v", strings.Join(args, " "), err)
		}
		exitCode = exitErr.ExitCode()
	}
	return stdout.String(), stderr.String(), exitCode
}

// decodeSingleJSON 断言输出恰好是一份 JSON 文档。
func decodeSingleJSON(t *testing.T, output string) map[string]any {
	t.Helper()

	trimmed := strings.TrimSpace(output)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		t.Fatalf("stdout must be exactly one JSON document, got:\n%s", output)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		t.Fatalf("decode stdout JSON: %v\n%s", err, output)
	}
	// 再次解码整体并要求无尾随内容
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if dec.More() {
		t.Fatalf("stdout carries more than one JSON document:\n%s", output)
	}
	return decoded
}

// writeE2EImage 用真实编码器生成各格式 fixture。
func writeE2EImage(t *testing.T, dir, name string) string {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 6, 4))
	for y := range 4 {
		for x := range 6 {
			img.Set(x, y, color.NRGBA{R: uint8(x * 40), G: uint8(y * 60), B: 90, A: 255})
		}
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	defer f.Close()

	switch filepath.Ext(name) {
	case ".png":
		err = png.Encode(f, img)
	case ".jpg":
		err = jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
	case ".gif":
		err = gif.Encode(f, img, nil)
	case ".webp":
		err = webp.Encode(f, img, nil)
	case ".bmp":
		err = bmp.Encode(f, img)
	case ".tiff":
		err = tiff.Encode(f, img, nil)
	default:
		t.Fatalf("unsupported fixture format %q", name)
	}
	if err != nil {
		t.Fatalf("encode %s: %v", name, err)
	}
	return path
}

// TestInspectBinaryE2E 编译后二进制的 inspect 内容识别契约：
// 覆盖 PNG/JPEG/GIF/WebP/BMP/TIFF/SVG/伪装 SVG/损坏 TIFF 与选择性哈希。
func TestInspectBinaryE2E(t *testing.T) {
	binary := buildE2EBinary(t)
	dir := t.TempDir()

	writeSVGFixture := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	t.Run("光栅格式识别与结构校验", func(t *testing.T) {
		for name, wantFormat := range map[string]string{
			"photo.png": "png", "photo.jpg": "jpeg", "anim.gif": "gif",
			"photo.webp": "webp", "photo.bmp": "bmp", "scan.tiff": "tiff",
		} {
			writeE2EImage(t, dir, name)
			stdout, stderr, code := runE2E(t, binary, dir, "inspect", "--format", "json", "--no-hash", "--no-detail", name)
			if code != 0 || stderr != "" {
				t.Fatalf("inspect %s failed (code %d): %s / %s", name, code, stdout, stderr)
			}
			doc := decodeSingleJSON(t, stdout)
			content := doc["content"].(map[string]any)
			if content["format"] != wantFormat {
				t.Errorf("%s: content.format = %v, want %s", name, content["format"], wantFormat)
			}
			if content["recognized"] != true || content["decode_supported"] != true {
				t.Errorf("%s: content = %v", name, content)
			}
			if _, hasImage := doc["image"]; !hasImage {
				t.Errorf("%s: image object missing", name)
			}
			if doc["schema_version"] != "itb.inspect.v3" {
				t.Errorf("%s: schema_version = %v", name, doc["schema_version"])
			}
		}
	})

	t.Run("SVG 识别但不做光栅解码", func(t *testing.T) {
		writeSVGFixture("vector.svg", `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"></svg>`)
		stdout, _, code := runE2E(t, binary, dir, "inspect", "--format", "json", "--no-hash", "vector.svg")
		if code != 0 {
			t.Fatalf("inspect svg failed (code %d): %s", code, stdout)
		}
		doc := decodeSingleJSON(t, stdout)
		content := doc["content"].(map[string]any)
		if content["format"] != "svg" || content["decode_supported"] != false || content["recognized"] != true {
			t.Fatalf("content = %v", content)
		}
		if _, hasImage := doc["image"]; hasImage {
			t.Error("svg must not produce an image object")
		}
		if _, hasError := doc["error"]; hasError {
			t.Error("svg must not be reported as corrupt")
		}
	})

	t.Run("HTML 改名 .svg 不被识别", func(t *testing.T) {
		writeSVGFixture("page.svg", `<!DOCTYPE html><html><body>x</body></html>`)
		stdout, _, code := runE2E(t, binary, dir, "inspect", "--format", "json", "--no-hash", "page.svg")
		if code != 0 {
			t.Fatalf("non-strict inspect of html content must exit 0: %d %s", code, stdout)
		}
		doc := decodeSingleJSON(t, stdout)
		content := doc["content"].(map[string]any)
		if content["recognized"] != false {
			t.Fatalf("content = %v, want unrecognized", content)
		}
	})

	t.Run("损坏 TIFF：识别成功但结构校验失败", func(t *testing.T) {
		corrupt := append([]byte("II*\x00"), bytes.Repeat([]byte{0xFF}, 64)...)
		if err := os.WriteFile(filepath.Join(dir, "corrupt.tiff"), corrupt, 0o644); err != nil {
			t.Fatalf("write corrupt tiff: %v", err)
		}
		stdout, _, code := runE2E(t, binary, dir, "inspect", "--format", "json", "--no-hash", "corrupt.tiff")
		if code != 0 {
			t.Fatalf("non-strict inspect must exit 0: %d %s", code, stdout)
		}
		doc := decodeSingleJSON(t, stdout)
		if doc["content"].(map[string]any)["format"] != "tiff" {
			t.Fatalf("content = %v", doc["content"])
		}
		errObj, ok := doc["error"].(map[string]any)
		if !ok || errObj["code"] != "decode_config_failed" {
			t.Fatalf("error = %v, want decode_config_failed", doc["error"])
		}
	})

	t.Run("--hash sha256 选择性哈希", func(t *testing.T) {
		writeE2EImage(t, dir, "hashed.png")
		stdout, _, code := runE2E(t, binary, dir, "inspect", "--format", "json", "--no-detail", "--hash", "sha256", "hashed.png")
		if code != 0 {
			t.Fatalf("inspect failed (code %d): %s", code, stdout)
		}
		doc := decodeSingleJSON(t, stdout)
		hashes := doc["hashes"].(map[string]any)
		if len(hashes) != 1 {
			t.Fatalf("hashes = %v, want only sha256", hashes)
		}
		digest, _ := hashes["sha256"].(string)
		if len(digest) != 64 {
			t.Fatalf("sha256 = %v", hashes["sha256"])
		}
		_ = filehash.SHA256
	})

	t.Run("缺失文件 --format json 输出机器错误", func(t *testing.T) {
		stdout, stderr, code := runE2E(t, binary, dir, "inspect", "--format", "json", "missing.png")
		if code == 0 {
			t.Fatalf("expected non-zero exit, got 0: %s", stdout)
		}
		doc := decodeSingleJSON(t, stdout)
		if doc["schema_version"] != "itb.error.v1" {
			t.Fatalf("schema_version = %v, want itb.error.v1", doc["schema_version"])
		}
		if doc["operation"] != "inspect" {
			t.Errorf("operation = %v, want inspect", doc["operation"])
		}
		info := doc["error"].(map[string]any)
		if info["code"] != "E_FILE_NOT_FOUND" {
			t.Errorf("code = %v, want E_FILE_NOT_FOUND", info["code"])
		}
		if stderr != "" {
			t.Errorf("stderr must not duplicate the error, got %q", stderr)
		}
	})
}

// TestCompressBinaryE2EFailureKeepsDestination 编译后二进制的 compress
// 失败路径：目标保持原状、无临时残留。真实压缩器未注入时管线同样
// 失败，因此该测试在任何构建下都有效。
func TestCompressBinaryE2EFailureKeepsDestination(t *testing.T) {
	binary := buildE2EBinary(t)
	dir := t.TempDir()

	broken := append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte{0xFF}, 128)...)
	if err := os.WriteFile(filepath.Join(dir, "broken.png"), broken, 0o644); err != nil {
		t.Fatalf("write broken png: %v", err)
	}
	destination := filepath.Join(dir, "existing.png")
	if err := os.WriteFile(destination, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("seed destination: %v", err)
	}

	stdout, _, code := runE2E(t, binary, dir, "compress", "--format", "json", "broken.png", "existing.png")
	if code == 0 {
		t.Fatalf("expected failure, got exit 0: %s", stdout)
	}
	doc := decodeSingleJSON(t, stdout)
	if doc["schema_version"] != "itb.error.v1" || doc["operation"] != "compress" {
		t.Fatalf("doc = %v, want itb.error.v1 compress", doc)
	}

	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if string(got) != "keep me" {
		t.Fatalf("destination content = %q, want untouched", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".itb-compress-") {
			t.Errorf("temp file left behind: %s", entry.Name())
		}
	}
}
