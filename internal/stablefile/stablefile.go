// Package stablefile 提供"稳定源快照"内部原语。
//
// compress 与 s3 upload 都要求"摘要、内容检测、实际处理"严格基于
// 同一份数据：源文件在读取期间被替换时，报告的 SHA-256 就不再对应
// 实际处理的内容（TOCTOU）。Snapshot 把源文件复制到私有临时文件并
// 单遍计算 SHA-256，随后以 filehash.VerifyUnchanged 检测源文件的可
// 观察变化；快照完成之后的任何源变化都不影响本次处理。
//
// 这是仅供内部复用的实现细节：不提供 CLI 命令，不扩大公开功能
// 边界，只消除两个领域各自维护快照逻辑的重复。
package stablefile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"imagetoolbox/internal/filehash"
)

// File 是一份稳定的源数据快照。Path 返回快照文件路径（0600 权限，
// 位于系统临时目录），SHA256/Size 返回快照字节的摘要与长度；
// Close 删除快照，必须由调用方保证执行。
type File struct {
	path   string
	sha256 string
	size   int64
}

// Snapshot 把 file 当前内容复制到私有临时快照并单遍计算 SHA-256，
// 随后检测源文件（path，以 initial 为基准）是否发生可观察变化。
// 检测到变化（filehash.ErrSourceChanged）或任何复制失败时，快照
// 已被清理并返回错误——失败路径不留临时文件。
func Snapshot(path string, file *os.File, initial os.FileInfo) (*File, error) {
	tmp, err := os.CreateTemp("", "itb-source-snapshot-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create source snapshot: %w", err)
	}
	snapshotPath := tmp.Name()

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hasher), file)
	if err != nil {
		tmp.Close()
		os.Remove(snapshotPath)
		return nil, fmt.Errorf("failed to snapshot source file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(snapshotPath)
		return nil, fmt.Errorf("failed to close source snapshot: %w", err)
	}

	// 源文件在复制期间发生可观察变化时放弃：快照内容与调用方看到的
	// 文件不再对应，摘要也就失去了意义
	if err := filehash.VerifyUnchanged(path, file, initial); err != nil {
		os.Remove(snapshotPath)
		return nil, err
	}

	return &File{
		path:   snapshotPath,
		sha256: hex.EncodeToString(hasher.Sum(nil)),
		size:   size,
	}, nil
}

// Path 返回快照文件路径。
func (f *File) Path() string { return f.path }

// SHA256 返回快照内容的十六进制 SHA-256 摘要。
func (f *File) SHA256() string { return f.sha256 }

// Size 返回快照字节数。
func (f *File) Size() int64 { return f.size }

// Open 重新打开快照供读取；*os.File 实现 io.Seeker，可安全 rewind
//（如 AWS SDK retry）。调用方负责 Close。
func (f *File) Open() (*os.File, error) {
	return os.Open(f.path)
}

// Close 删除快照文件，幂等；已删除或再次 Close 返回 nil。
func (f *File) Close() error {
	if f.path == "" {
		return nil
	}
	path := f.path
	f.path = ""
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
