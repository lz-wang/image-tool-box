package stablefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"imagetoolbox/internal/filehash"
)

// leftoverSnapshots 统计系统临时目录下当前存在的源快照。
func leftoverSnapshots(t *testing.T) map[string]bool {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "itb-source-snapshot-*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	set := make(map[string]bool, len(matches))
	for _, path := range matches {
		set[path] = true
	}
	return set
}

func assertNoNewSnapshots(t *testing.T, before map[string]bool) {
	t.Helper()

	for path := range leftoverSnapshots(t) {
		if !before[path] {
			t.Errorf("snapshot left behind: %s", path)
		}
	}
}

func openSource(t *testing.T, path string) (*os.File, os.FileInfo) {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { file.Close() })
	initial, err := file.Stat()
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	return file, initial
}

// TestSnapshotStableContent 稳定源：快照内容、摘要、size 一一对应，
// 权限 0600，Open 可重新读取。
func TestSnapshotStableContent(t *testing.T) {
	before := leftoverSnapshots(t)
	path := filepath.Join(t.TempDir(), "source.txt")
	content := "hello stable world"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	file, initial := openSource(t, path)
	snapshot, err := Snapshot(path, file, initial)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	defer func() {
		if err := snapshot.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	sum, err := filehash.Sum(strings.NewReader(content), []filehash.Algorithm{filehash.SHA256})
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if snapshot.SHA256() != sum.Digests[filehash.SHA256] {
		t.Errorf("sha256 = %q, want %q", snapshot.SHA256(), sum.Digests[filehash.SHA256])
	}
	if snapshot.Size() != int64(len(content)) {
		t.Errorf("size = %d, want %d", snapshot.Size(), len(content))
	}

	info, err := os.Stat(snapshot.Path())
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("snapshot mode = %v, want 0600", info.Mode().Perm())
	}

	reader, err := snapshot.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got := make([]byte, len(content))
	if _, err := reader.Read(got); err != nil {
		reader.Close()
		t.Fatalf("read: %v", err)
	}
	reader.Close()
	if string(got) != content {
		t.Errorf("snapshot content = %q, want %q", got, content)
	}

	// 正常路径：Close 之后无快照残留
	if err := snapshot.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertNoNewSnapshots(t, before)
}

// TestSnapshotDetectsChangedSource 快照期间源文件发生可观察变化时报
// ErrSourceChanged 且不留快照残留。
func TestSnapshotDetectsChangedSource(t *testing.T) {
	before := leftoverSnapshots(t)
	path := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	file, initial := openSource(t, path)

	// "复制期间"内容被替换（size/modtime 变化）
	if err := os.WriteFile(path, []byte("replaced with different content"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	snapshot, err := Snapshot(path, file, initial)
	if err == nil || !strings.Contains(err.Error(), filehash.ErrSourceChanged.Error()) {
		t.Fatalf("err = %v, want source-changed failure", err)
	}
	if snapshot != nil {
		t.Errorf("failed snapshot must return nil, got %+v", snapshot)
	}
	assertNoNewSnapshots(t, before)
}

// TestSnapshotFailureCleansUpOnCopyError 复制失败（源是目录）时同样
// 清理快照。
func TestSnapshotFailureCleansUpOnCopyError(t *testing.T) {
	before := leftoverSnapshots(t)
	dir := t.TempDir()

	file, initial := openSource(t, dir)
	if _, err := Snapshot(dir, file, initial); err == nil {
		t.Fatal("expected copy failure for directory source")
	}
	assertNoNewSnapshots(t, before)
}

// TestCloseIdempotent Close 幂等：二次 Close 不报错，快照文件被删除。
func TestCloseIdempotent(t *testing.T) {
	before := leftoverSnapshots(t)
	path := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	file, initial := openSource(t, path)
	snapshot, err := Snapshot(path, file, initial)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	snapshotPath := snapshot.Path()
	if err := snapshot.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := snapshot.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if _, err := os.Stat(snapshotPath); !os.IsNotExist(err) {
		t.Errorf("snapshot must be removed, stat err = %v", err)
	}
	assertNoNewSnapshots(t, before)
}
