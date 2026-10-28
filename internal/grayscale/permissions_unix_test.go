//go:build unix

package grayscale

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOutputPermissions(t *testing.T) {
	if os.Getenv("ITB_TEST_GRAYSCALE_UMASK_CHILD") != "1" {
		// umask is process-wide: exercise it only in an isolated test process.
		cmd := exec.Command(os.Args[0], "-test.run=^TestOutputPermissions$", "-test.v")
		cmd.Env = append(os.Environ(), "ITB_TEST_GRAYSCALE_UMASK_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("permission subprocess: %v\n%s", err, out)
		}
		return
	}
	syscall.Umask(0o077)
	dir := t.TempDir()
	src := writeSource(t, dir)
	if err := os.Chmod(src, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		existing bool
		mode     os.FileMode
	}{
		{"new", false, 0o600}, {"private", true, 0o600}, {"read only", true, 0o400}, {"shared", true, 0o640},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := filepath.Join(dir, tc.name+".png")
			if tc.existing {
				if err := os.WriteFile(dst, []byte("old"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(dst, tc.mode); err != nil {
					t.Fatal(err)
				}
			}
			if err := ConvertFile(src, dst, Options{}); err != nil {
				t.Fatal(err)
			}
			assertMode(t, dst, tc.mode)
		})
	}
	t.Run("failure retains old content and mode", func(t *testing.T) {
		dst := filepath.Join(dir, "failure.png")
		if err := os.WriteFile(dst, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dst, 0o400); err != nil {
			t.Fatal(err)
		}
		failure := errors.New("encode failed")
		if err := commitFile(src, dst, func(w io.Writer) error {
			if _, err := w.Write([]byte("partial")); err != nil {
				return err
			}
			return failure
		}); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		assertMode(t, dst, 0o400)
		data, err := os.ReadFile(dst)
		if err != nil || string(data) != "keep" {
			t.Fatal(string(data), err)
		}
	})
	t.Run("symlink replacement stays private", func(t *testing.T) {
		target := filepath.Join(dir, "unrelated.png")
		dst := filepath.Join(dir, "link.png")
		if err := os.WriteFile(target, []byte("unrelated"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(target, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, dst); err != nil {
			t.Fatal(err)
		}
		if err := ConvertFile(src, dst, Options{}); err != nil {
			t.Fatal(err)
		}
		assertMode(t, dst, 0o600)
		assertMode(t, target, 0o644)
		data, err := os.ReadFile(target)
		if err != nil || string(data) != "unrelated" {
			t.Fatal(string(data), err)
		}
	})
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := stat.Mode().Perm(); got != want {
		t.Fatalf("%s mode %04o want %04o", path, got, want)
	}
}
