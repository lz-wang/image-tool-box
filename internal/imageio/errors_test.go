package imageio

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveOutputErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, filepath.Join(file, "out.png")} {
		t.Run(path, func(t *testing.T) {
			err := SaveWithFormat(path, image.NewGray(image.Rect(0, 0, 1, 1)), FormatPNG, SaveOptions{})
			var outputErr *OutputError
			var pathErr *os.PathError
			if !errors.As(err, &outputErr) || !errors.As(err, &pathErr) {
				t.Fatalf("error chain: %T %v", err, err)
			}
		})
	}
	t.Run("wrapped cause", func(t *testing.T) {
		cause := errors.New("disk is full")
		err := &OutputError{Err: cause}
		if !errors.Is(err, cause) {
			t.Fatal("cause lost")
		}
	})
}
