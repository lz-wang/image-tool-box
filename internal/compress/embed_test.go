package compress

import (
	"runtime"
	"testing"
)

func TestExtractedBinaryName(t *testing.T) {
	got := extractedBinaryName(PngQuant)

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
