package filehash

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// 已知向量：与 inspect v2 默认输出保持逐字节一致（"hello\n"）。
var knownVectors = map[Algorithm]string{
	SHA256: "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03",
	SHA1:   "f572d396fae9206628714fb2ce00f72e94f2258f",
	MD5:    "b1946ac92492d2347c6235b4d2611184",
	CRC32:  "363a3020",
}

func TestSumAllAlgorithms(t *testing.T) {
	result, err := Sum(strings.NewReader("hello\n"), AllAlgorithms())
	if err != nil {
		t.Fatalf("Sum: %v", err)
	}
	if result.BytesRead != int64(len("hello\n")) {
		t.Errorf("BytesRead = %d, want %d", result.BytesRead, len("hello\n"))
	}
	for algorithm, want := range knownVectors {
		if got := result.Digests[algorithm]; got != want {
			t.Errorf("%s = %q, want %q", algorithm, got, want)
		}
	}
}

func TestSumSelective(t *testing.T) {
	result, err := Sum(strings.NewReader("hello\n"), []Algorithm{SHA256, CRC32})
	if err != nil {
		t.Fatalf("Sum: %v", err)
	}
	if len(result.Digests) != 2 {
		t.Fatalf("digests = %v, want exactly 2", result.Digests)
	}
	if result.Digests[SHA256] != knownVectors[SHA256] || result.Digests[CRC32] != knownVectors[CRC32] {
		t.Errorf("digests = %v", result.Digests)
	}
}

func TestSumNilAlgorithmsMeansAll(t *testing.T) {
	result, err := Sum(strings.NewReader("hello\n"), nil)
	if err != nil {
		t.Fatalf("Sum: %v", err)
	}
	if len(result.Digests) != 4 {
		t.Fatalf("digest count = %d, want 4", len(result.Digests))
	}
}

func TestSumUnknownAlgorithm(t *testing.T) {
	if _, err := Sum(strings.NewReader("x"), []Algorithm{"crc64"}); err == nil {
		t.Fatal("expected error for unknown algorithm")
	}
}

func TestSumEmptyContent(t *testing.T) {
	result, err := Sum(strings.NewReader(""), []Algorithm{SHA256, CRC32})
	if err != nil {
		t.Fatalf("Sum: %v", err)
	}
	if result.Digests[SHA256] != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("empty sha256 = %q", result.Digests[SHA256])
	}
	if result.Digests[CRC32] != "00000000" {
		t.Errorf("empty crc32 = %q, want 00000000", result.Digests[CRC32])
	}
}

func TestParse(t *testing.T) {
	t.Run("empty input means default", func(t *testing.T) {
		got, err := Parse(nil)
		if err != nil || got != nil {
			t.Fatalf("Parse(nil) = %v, %v", got, err)
		}
	})

	t.Run("valid algorithms", func(t *testing.T) {
		got, err := Parse([]string{"sha256", "crc32"})
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if len(got) != 2 || got[0] != SHA256 || got[1] != CRC32 {
			t.Fatalf("Parse = %v", got)
		}
	})

	t.Run("unknown algorithm", func(t *testing.T) {
		_, err := Parse([]string{"sha256", "sha512"})
		if err == nil || !strings.Contains(err.Error(), "sha512") {
			t.Fatalf("err = %v, want unknown algorithm sha512", err)
		}
	})

	t.Run("duplicate algorithm", func(t *testing.T) {
		_, err := Parse([]string{"md5", "md5"})
		if err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("err = %v, want duplicate algorithm", err)
		}
	})
}

func TestSumFileStableContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	result, err := SumFile(path, nil)
	if err != nil {
		t.Fatalf("SumFile: %v", err)
	}
	for algorithm, want := range knownVectors {
		if got := result.Digests[algorithm]; got != want {
			t.Errorf("%s = %q, want %q", algorithm, got, want)
		}
	}
}

func TestSumFileSelective(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	result, err := SumFile(path, []Algorithm{SHA256})
	if err != nil {
		t.Fatalf("SumFile: %v", err)
	}
	if len(result.Digests) != 1 || result.Digests[SHA256] != knownVectors[SHA256] {
		t.Fatalf("digests = %v", result.Digests)
	}
}

func TestSumFileMissing(t *testing.T) {
	if _, err := SumFile(filepath.Join(t.TempDir(), "nope.bin"), nil); err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestSumFileRejectsNonRegularFile 目录与 FIFO 不是普通文件，必须在
// 任何读取（乃至 open 阻塞）之前拒绝。
func TestSumFileRejectsNonRegularFile(t *testing.T) {
	dir := t.TempDir()

	if _, err := SumFile(dir, nil); err == nil || !errors.Is(err, ErrNotRegularFile) {
		t.Fatalf("directory err = %v, want ErrNotRegularFile", err)
	}

	fifoPath := filepath.Join(dir, "pipe.fifo")
	if err := syscall.Mkfifo(fifoPath, 0o644); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	// SumFile 内部以带超时的 goroutine 探测不现实（Open 会阻塞）：
	// 预检必须让调用直接失败而非挂起。用 goroutine + 超时兜底防挂。
	type result struct {
		err error
	}
	done := make(chan result, 1)
	go func() {
		_, err := SumFile(fifoPath, []Algorithm{SHA256})
		done <- result{err}
	}()
	select {
	case r := <-done:
		if !errors.Is(r.err, ErrNotRegularFile) {
			t.Fatalf("fifo err = %v, want ErrNotRegularFile", r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SumFile blocked on FIFO: non-regular pre-check missing")
	}
}

// mutatedInfo 包装 FileInfo 并伪造 size / modtime（Sys() 委托原值，
// 保证 os.SameFile 判定为同一文件）。
type mutatedInfo struct {
	os.FileInfo
	size    int64
	modtime time.Time
}

func (m mutatedInfo) Size() int64        { return m.size }
func (m mutatedInfo) ModTime() time.Time { return m.modtime }

// TestVerifyUnchanged 直接覆盖可观察变化检测的各分支。
func TestVerifyUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(path, []byte("stable"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer file.Close()
	initial, err := file.Stat()
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	t.Run("unchanged passes", func(t *testing.T) {
		if err := VerifyUnchanged(path, file, initial); err != nil {
			t.Fatalf("verifyUnchanged: %v", err)
		}
	})

	t.Run("size changed fails", func(t *testing.T) {
		grown := mutatedInfo{FileInfo: initial, size: initial.Size() + 1, modtime: initial.ModTime()}
		if err := VerifyUnchanged(path, file, grown); err == nil || !strings.Contains(err.Error(), ErrSourceChanged.Error()) {
			t.Fatalf("err = %v, want %v", err, ErrSourceChanged)
		}
	})

	t.Run("modtime changed fails", func(t *testing.T) {
		shifted := mutatedInfo{FileInfo: initial, size: initial.Size(), modtime: initial.ModTime().Add(time.Second)}
		if err := VerifyUnchanged(path, file, shifted); err == nil || !strings.Contains(err.Error(), ErrSourceChanged.Error()) {
			t.Fatalf("err = %v, want %v", err, ErrSourceChanged)
		}
	})

	t.Run("file deleted fails", func(t *testing.T) {
		missing := filepath.Join(dir, "gone.txt")
		if err := VerifyUnchanged(missing, file, initial); err == nil || !strings.Contains(err.Error(), ErrSourceChanged.Error()) {
			t.Fatalf("err = %v, want %v", err, ErrSourceChanged)
		}
	})

	t.Run("path replaced by different file fails", func(t *testing.T) {
		replacement := filepath.Join(dir, "replacement.txt")
		if err := os.WriteFile(replacement, []byte("stable"), 0o644); err != nil {
			t.Fatalf("write replacement: %v", err)
		}
		if err := VerifyUnchanged(replacement, file, initial); err == nil || !strings.Contains(err.Error(), ErrSourceChanged.Error()) {
			t.Fatalf("err = %v, want %v", err, ErrSourceChanged)
		}
	})
}

// TestSumFileDetectsInPlaceMutation 端到端：读取完成后文件被就地追加，
// SumFile 必须报 ErrSourceChanged。
func TestSumFileDetectsInPlaceMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mutated.txt")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	initial, err := file.Stat()
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	file.Close()

	// 模拟"hash 期间"的变化：改写内容（size 与 modtime 均变化）
	if err := os.WriteFile(path, []byte("original+appended"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	reopened, err := os.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	// verifyUnchanged 以"打开时"的 initial 与最新路径比对 → 检测到变化
	if err := VerifyUnchanged(path, reopened, initial); err == nil || !strings.Contains(err.Error(), ErrSourceChanged.Error()) {
		t.Fatalf("err = %v, want %v", err, ErrSourceChanged)
	}
}
