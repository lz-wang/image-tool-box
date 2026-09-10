// Package filehash 提供跨命令共享的文件内容哈希能力。
//
// 它是内部 utility：只暴露流式多算法摘要计算与"读取期间源文件未发生
// 可观察变化"的保证，不提供 CLI 命令。inspect / compress / S3 上传等
// 命令共用同一实现，避免各处重复维护哈希管道。
package filehash

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
)

// Algorithm 哈希算法名称（即 CLI --hash 的取值）。
type Algorithm string

const (
	SHA256 Algorithm = "sha256"
	SHA1   Algorithm = "sha1"
	MD5    Algorithm = "md5"
	CRC32  Algorithm = "crc32"
)

// ErrSourceChanged 读取期间源文件发生了可观察变化（内容被修改、替换
// 或删除）。它只保证"可观察变化"检测（size/modtime/inode），不宣称
// 能检测保留 size 与 modtime 的恶意并发修改。
var ErrSourceChanged = errors.New("source file changed while being read")

// ErrNotRegularFile 输入路径不是普通文件（目录、FIFO、设备等）。
// FIFO 必须在 open 之前拒绝——open 一个没有写端的 FIFO 会无限阻塞。
var ErrNotRegularFile = errors.New("source is not a regular file")

// AllAlgorithms 是未做选择性指定时的默认算法集合（历史行为：全部计算）。
func AllAlgorithms() []Algorithm {
	return []Algorithm{SHA256, SHA1, MD5, CRC32}
}

// Parse 将 CLI 传入的算法名解析为 Algorithm；未知算法报错并列出合法值。
func Parse(values []string) ([]Algorithm, error) {
	if len(values) == 0 {
		return nil, nil
	}
	seen := make(map[Algorithm]bool, len(values))
	algorithms := make([]Algorithm, 0, len(values))
	for _, value := range values {
		algorithm := Algorithm(value)
		switch algorithm {
		case SHA256, SHA1, MD5, CRC32:
		default:
			return nil, fmt.Errorf("unknown hash algorithm %q (supported: sha256, sha1, md5, crc32)", value)
		}
		if seen[algorithm] {
			return nil, fmt.Errorf("duplicate hash algorithm %q", value)
		}
		seen[algorithm] = true
		algorithms = append(algorithms, algorithm)
	}
	return algorithms, nil
}

// Result 一次摘要计算的结果。
type Result struct {
	// BytesRead 是参与哈希的字节数
	BytesRead int64

	// Digests 是各算法的十六进制摘要；CRC32 为 8 位十六进制
	//（IEEE 多项式，与 hash/crc32 的 Sum32 一致）
	Digests map[Algorithm]string
}

// Sum 流式计算 r 的多算法摘要，单次读取完成。
func Sum(r io.Reader, algorithms []Algorithm) (Result, error) {
	if len(algorithms) == 0 {
		algorithms = AllAlgorithms()
	}

	hashers := make(map[Algorithm]hash.Hash, len(algorithms))
	writers := make([]io.Writer, 0, len(algorithms))
	for _, algorithm := range algorithms {
		if _, exists := hashers[algorithm]; exists {
			continue
		}
		switch algorithm {
		case SHA256:
			hashers[algorithm] = sha256.New()
		case SHA1:
			hashers[algorithm] = sha1.New()
		case MD5:
			hashers[algorithm] = md5.New()
		case CRC32:
			hashers[algorithm] = crc32.NewIEEE()
		default:
			return Result{}, fmt.Errorf("unknown hash algorithm %q (supported: sha256, sha1, md5, crc32)", algorithm)
		}
		writers = append(writers, hashers[algorithm])
	}

	written, err := io.Copy(io.MultiWriter(writers...), r)
	if err != nil {
		return Result{}, fmt.Errorf("failed to hash content: %w", err)
	}

	digests := make(map[Algorithm]string, len(hashers))
	for algorithm, hasher := range hashers {
		// CRC32 的 Sum(nil) 是 4 字节大端编码，hex 编码后与
		// %08x 格式化的 Sum32 完全一致（8 位十六进制）
		digests[algorithm] = hex.EncodeToString(hasher.Sum(nil))
	}
	return Result{BytesRead: written, Digests: digests}, nil
}

// SumFile 计算文件内容的多算法摘要，并保证结果对应一次完整、未被
// 可观察打断的读取：
//
//  1. 预检路径必须指向普通文件（FIFO 在 open 阶段就会阻塞，必须
//     先用 Stat 拒绝；目录/FIFO/设备一律 ErrNotRegularFile）；
//  2. 打开文件，保存初始 FileInfo（再查一次 mode，双保险）；
//  3. 流式哈希（单次读取）；
//  4. 再次读取同一 FD 的 Stat 与原路径的 Stat；
//  5. 以 os.SameFile + size/modtime 检测可观察变化。
//
// 检测到变化（内容被就地修改、路径被 rename 替换或文件被删除）时返回
// ErrSourceChanged，摘要不可信。注意：无法检测保留 size 与 modtime 的
// 恶意并发修改。
func SumFile(path string, algorithms []Algorithm) (Result, error) {
	// open 之前先拒绝非普通文件：os.Open 一个 FIFO 会阻塞到有写端出现
	if info, err := os.Stat(path); err == nil && !info.Mode().IsRegular() {
		return Result{}, fmt.Errorf("%w: %s", ErrNotRegularFile, path)
	}

	file, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer file.Close()

	initial, err := file.Stat()
	if err != nil {
		return Result{}, fmt.Errorf("failed to stat input file: %w", err)
	}
	if !initial.Mode().IsRegular() {
		return Result{}, fmt.Errorf("%w: %s", ErrNotRegularFile, path)
	}

	result, err := Sum(file, algorithms)
	if err != nil {
		return Result{}, err
	}

	if err := VerifyUnchanged(path, file, initial); err != nil {
		return Result{}, err
	}
	return result, nil
}

// VerifyUnchanged 比对初始 FileInfo、当前 FD 与当前路径，报告读取期间
// 的可观察变化（内容被就地修改、路径被 rename 替换或文件被删除）。
// 供 SumFile 与需要"边读边做其他事"的调用方（如 S3 上传快照）复用。
func VerifyUnchanged(path string, file *os.File, initial fs.FileInfo) error {
	current, err := file.Stat()
	if err != nil {
		return fmt.Errorf("%w: failed to re-stat open file: %v", ErrSourceChanged, err)
	}
	// FD 仍指向同一文件时才可比对 size/modtime（FD 不会因路径替换改变，
	// 这一步探测的是"打开后内容被就地修改"）
	if !os.SameFile(initial, current) ||
		initial.Size() != current.Size() ||
		!initial.ModTime().Equal(current.ModTime()) {
		return ErrSourceChanged
	}

	// 原路径探测：路径被 rename/删除替换时，新路径的 inode 与打开时的
	// FD 不同，摘要不再对应调用方可见的那个文件
	latest, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrSourceChanged
		}
		return fmt.Errorf("failed to stat input file: %w", err)
	}
	if !os.SameFile(initial, latest) ||
		initial.Size() != latest.Size() ||
		!initial.ModTime().Equal(latest.ModTime()) {
		return ErrSourceChanged
	}
	return nil
}
