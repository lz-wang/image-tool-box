package nativebin

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"testing/fstest"
)

func TestExtractedBinaryName(t *testing.T) {
	got := extractedBinaryName(PNGQuant)
	if runtime.GOOS == "windows" {
		if got != "pngquant.exe" {
			t.Fatalf("expected Windows binary name to end with .exe, got %q", got)
		}
		return
	}
	if got != "pngquant" {
		t.Fatalf("expected non-Windows binary name without extension, got %q", got)
	}
}

func TestEnsureLazyContentAddressedAndCached(t *testing.T) {
	cacheDir := t.TempDir()
	withTestBinaries(t, cacheDir, []byte("first fake pngquant"))

	path, err := Ensure(PNGQuant)
	if err != nil {
		t.Fatalf("Ensure(PNGQuant): %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("extracted binary missing: %v", err)
	}
	for _, binType := range []ID{OxiPNG, DJPEG, CJPEG} {
		matches, _ := filepath.Glob(filepath.Join(cacheDir, "itb", "bins", getPlatformKey(), "*", extractedBinaryName(binType)))
		if len(matches) != 0 {
			t.Fatalf("%s should not be extracted by pngquant request: %v", binType, matches)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	pathAgain, err := Ensure(PNGQuant)
	if err != nil {
		t.Fatalf("second Ensure(PNGQuant): %v", err)
	}
	if pathAgain != path {
		t.Fatalf("cache path changed: got %q, want %q", pathAgain, path)
	}
	infoAgain, err := os.Stat(pathAgain)
	if err != nil {
		t.Fatal(err)
	}
	if !infoAgain.ModTime().Equal(info.ModTime()) {
		t.Fatal("cache hit rewrote the binary")
	}
}

func TestEnsureSameSizeDifferentContentUsesDifferentPaths(t *testing.T) {
	cacheDir := t.TempDir()
	first := []byte("same-length-content-a")
	second := []byte("same-length-content-b")
	if len(first) != len(second) {
		t.Fatal("test inputs must have the same size")
	}
	withTestBinaries(t, cacheDir, first)
	firstPath, err := Ensure(PNGQuant)
	if err != nil {
		t.Fatal(err)
	}
	withTestBinaries(t, cacheDir, second)
	secondPath, err := Ensure(PNGQuant)
	if err != nil {
		t.Fatal(err)
	}
	if firstPath == secondPath {
		t.Fatalf("same-size binaries used one cache path: %q", firstPath)
	}
}

func TestEnsureConcurrentExtraction(t *testing.T) {
	cacheDir := t.TempDir()
	withTestBinaries(t, cacheDir, []byte("concurrent fake pngquant"))

	const goroutines = 32
	paths := make(chan string, goroutines)
	errs := make(chan error, goroutines)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			path, err := Ensure(PNGQuant)
			if err != nil {
				errs <- err
				return
			}
			paths <- path
		}()
	}
	wg.Wait()
	close(paths)
	close(errs)
	for err := range errs {
		t.Errorf("Ensure: %v", err)
	}

	var first string
	for path := range paths {
		if first == "" {
			first = path
		} else if path != first {
			t.Errorf("got path %q, want %q", path, first)
		}
	}
	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}
	if string(data) != "concurrent fake pngquant" {
		t.Fatalf("unexpected extracted content: %q", data)
	}
}

func TestIsUsableBinaryFile(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(filePath, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !isUsableBinaryFile(info) {
		t.Fatal("executable regular file must be usable")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filePath, 0644); err != nil {
			t.Fatal(err)
		}
		info, err = os.Stat(filePath)
		if err != nil {
			t.Fatal(err)
		}
		if isUsableBinaryFile(info) {
			t.Fatal("non-executable Unix regular file must not be usable")
		}
	}

	dirInfo, err := os.Stat(filepath.Dir(filePath))
	if err != nil {
		t.Fatal(err)
	}
	if isUsableBinaryFile(dirInfo) {
		t.Fatal("directory must not be usable as a binary")
	}
}

func withTestBinaries(t *testing.T, cacheDir string, pngquant []byte) {
	t.Helper()
	platformPaths, ok := binaryPaths[getPlatformKey()]
	if !ok {
		t.Skipf("unsupported test platform: %s", getPlatformKey())
	}
	source := make(fstest.MapFS, len(platformPaths))
	for binType, path := range platformPaths {
		data := []byte("fake " + string(binType))
		if binType == PNGQuant {
			data = pngquant
		}
		source[path] = &fstest.MapFile{Data: data, Mode: fs.FileMode(0755)}
	}

	binariesMu.Lock()
	previousCacheBaseDir := cacheBaseDir
	cacheBaseDir = func() (string, error) { return cacheDir, nil }
	binariesMu.Unlock()
	Init(source)
	t.Cleanup(func() {
		binariesMu.Lock()
		cacheBaseDir = previousCacheBaseDir
		binariesMu.Unlock()
	})
}
