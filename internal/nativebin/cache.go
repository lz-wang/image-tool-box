package nativebin

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

func defaultCacheBaseDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err == nil {
		return cacheDir, nil
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("itb-%d", os.Getuid())), nil
}

func fileMatchesHash(path string, expected [sha256.Size]byte) (bool, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if !isUsableBinaryFile(info) {
		return false, nil
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return false, err
	}
	return bytes.Equal(hash.Sum(nil), expected[:]), nil
}

func isUsableBinaryFile(info fs.FileInfo) bool {
	if !info.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0111 != 0
}

func writeAtomically(dir, targetPath string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".extract-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		sum := sha256.Sum256(data)
		if match, matchErr := fileMatchesHash(targetPath, sum); matchErr == nil && match {
			return nil
		}
		return err
	}
	return nil
}
