package inspect

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"os"
)

func ComputeAllHashes(path string) (*HashInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	sha256Hash := sha256.New()
	sha1Hash := sha1.New()
	md5Hash := md5.New()
	var crc32Hash hash.Hash32 = crc32.NewIEEE()

	writer := io.MultiWriter(
		sha256Hash,
		sha1Hash,
		md5Hash,
		crc32Hash,
	)

	if _, err := io.Copy(writer, f); err != nil {
		return nil, fmt.Errorf("计算 hash 失败: %w", err)
	}

	return &HashInfo{
		SHA256: hex.EncodeToString(sha256Hash.Sum(nil)),
		SHA1:   hex.EncodeToString(sha1Hash.Sum(nil)),
		MD5:    hex.EncodeToString(md5Hash.Sum(nil)),
		CRC32:  fmt.Sprintf("%08x", crc32Hash.Sum32()),
	}, nil
}
