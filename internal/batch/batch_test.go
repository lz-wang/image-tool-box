package batch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.jpg"))
	mustWrite(t, filepath.Join(dir, "b.txt"))
	mustWrite(t, filepath.Join(dir, "nested", "c.png"))

	files, err := DiscoverFiles(Options{
		InputDir:  dir,
		Recursive: true,
		Glob:      "*.png",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 || filepath.ToSlash(files[0]) != "nested/c.png" {
		t.Fatalf("got %v", files)
	}
}

func TestProcessSkipExistingAndFailures(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	mustWrite(t, filepath.Join(inputDir, "a.jpg"))
	mustWrite(t, filepath.Join(inputDir, "b.jpg"))
	mustWrite(t, filepath.Join(outputDir, "a_done.jpg"))

	result, err := Process(Options{
		InputDir:     inputDir,
		OutputDir:    outputDir,
		Workers:      2,
		SkipExisting: true,
	}, func(rel string) string {
		return strings.TrimSuffix(rel, filepath.Ext(rel)) + "_done.jpg"
	}, func(inputPath, outputPath string) error {
		if filepath.Base(inputPath) == "b.jpg" {
			return fmt.Errorf("boom")
		}
		return os.WriteFile(outputPath, []byte("ok"), 0o644)
	})
	if err == nil {
		t.Fatal("expected batch failure")
	}
	if result.Skipped != 1 || result.Failed != 1 {
		t.Fatalf("got %+v", result)
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
