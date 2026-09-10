package s3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"imagetoolbox/internal/filehash"
)

// DownloadSchemaVersion 是 download --format json 的机器可读契约版本。
// v2：新增 status（downloaded/reused）与 content_type 字段，
// 支持 --expect-size / --expect-content-type / --if-exists。
const DownloadSchemaVersion = "itb.s3.download.v2"

// DownloadResult.Status 的取值。
const (
	StatusDownloaded = "downloaded"
)

// ErrExpectationMismatch 下载对象的实际状态与期望值不一致
//（--expect-size / --expect-content-type / 本地复用校验失败）。
var ErrExpectationMismatch = errors.New("object state does not match expectations")

// ErrReuseVerificationUnavailable 启用 --if-exists=verify 但没有任何
// 可证明内容一致的校验依据（--verify-sha256 或 --verify）。
// "文件存在就复用"绝不成立。
var ErrReuseVerificationUnavailable = errors.New("--if-exists=verify requires --verify-sha256 or --verify as a verification basis")

// ErrInvalidIfExists --if-exists 取值非法。
var ErrInvalidIfExists = errors.New("invalid --if-exists value")

// IfExistsBehavior 描述目标文件已存在时的处理策略。
type IfExistsBehavior string

const (
	// IfExistsReplace（默认）总是执行 GET 并覆盖本地目标，
	// 与 v0.9.x 行为一致。
	IfExistsReplace IfExistsBehavior = "replace"

	// IfExistsVerify 仅当调用方提供了可证明内容一致的校验依据时，
	// 才允许跳过 GET 复用本地副本；无法证明一致时报
	// ErrExpectationMismatch（本地副本存在但与期望不符）。
	IfExistsVerify IfExistsBehavior = "verify"
)

// DownloadOptions 下载选项
type DownloadOptions struct {
	// Verify 为 true 时读取对象 itb-sha256 用户 metadata，边下载边计算
	// SHA-256 并在结束时比对（io.MultiWriter，不二次读取本地文件）；
	// 对象缺少该 metadata 时直接报错。
	Verify bool

	// VerifySHA256 指定期望的十六进制 SHA-256（与 Verify 可同时使用），
	// 提供独立于对象 metadata 的 provider-neutral 完整性校验。
	VerifySHA256 string

	// ExpectSize 是期望的对象字节数。指针三态：nil 表示未指定
	//（0 字节对象合法，不能用零值表达"未指定"）。GET 响应头阶段
	// 与实际写入字节数各检查一次。
	ExpectSize *int64

	// ExpectContentType 是期望的对象 Content-Type。比较忽略参数部分
	//（; charset=...）与大小写。空串表示未指定。
	ExpectContentType string

	// IfExists 决定目标文件已存在时的策略：replace（默认，总是 GET
	// 覆盖）或 verify（本地副本通过校验则复用，status=reused）。
	IfExists IfExistsBehavior

	// Progress 接收大文件（>5MB）传输提示等进度信息，nil 表示不输出。
	// 进度信息不是执行结果，domain 不直接写 stdout，由 adapter 决定
	// 输出去向（CLI 传 os.Stderr，保持 stdout 只承载正式结果）。
	Progress io.Writer
}

// DownloadResult 下载结果
type DownloadResult struct {
	// SchemaVersion 机器可读契约版本（itb.s3.download.v2）
	SchemaVersion string `json:"schema_version"`

	// Key 下载的对象键
	Key string `json:"key"`

	// OutputPath 本地输出文件路径
	OutputPath string `json:"output_path"`

	// Status 是本次下载的结果状态：downloaded（执行了 GET）/ reused
	//（--if-exists=verify 命中，复用本地副本，v2 新增）
	Status string `json:"status"`

	// Size 实际写入（或复用副本）的字节数
	Size int64 `json:"size"`

	// SHA256 下载内容的 SHA-256；启用校验选项或复用本地副本时计算
	SHA256 string `json:"sha256,omitempty"`

	// ContentType 远端返回的 Content-Type；GET 响应携带时填充
	ContentType string `json:"content_type,omitempty"`
}

// Get 获取对象内容流（GetObject），调用方负责关闭返回的 Body。
// Download 以及其他需要流式消费对象内容的调用方可复用此函数，
// 避免"落盘临时文件 + 整文件读入内存"的开销。
func Get(ctx context.Context, client *Client, key string) (*s3.GetObjectOutput, error) {
	if key == "" {
		return nil, ErrMissingKey
	}

	out, err := client.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, WrapError(err)
	}
	return out, nil
}

// Download 从存储桶下载文件到本地路径。
//
// 内容先写入输出目录下的临时文件，成功后 rename 到目标路径：
// 下载中断、写盘失败或校验不通过时删除临时文件，目标路径不会留下
// partial 文件。
//
// 校验在流式下载中完成（io.MultiWriter(file, sha256)），不进行
// 第二遍本地文件读取；哈希不一致返回 ErrChecksumMismatch。
//
// 期望值检查：ExpectSize / ExpectContentType 在创建最终目标前先对
// GET 响应头检查一次，下载结束后再对实际写入字节检查一次；任一不
// 符返回 ErrExpectationMismatch，本次下载不留任何文件。
//
// --if-exists verify：只有调用方提供校验依据（--verify-sha256 或
// --verify）时才允许跳过 GET；本地副本 size/SHA-256 与期望一致时
// 复用（status=reused），存在但不一致返回 ErrExpectationMismatch，
// 本地不存在则正常下载。没有任何校验依据时直接报
// ErrReuseVerificationUnavailable——绝不"文件存在就复用"。
// 复用不降低校验强度：--verify 与 --expect-content-type 在复用路径
// 同样生效（先 HEAD 比对远端 itb-sha256 / Content-Type）；只有
// --verify-sha256（+ 可选 --expect-size）才允许零网络复用。
//
// 本函数不输出任何内容：结果通过 DownloadResult 返回（Size 是实际
// 写入本地的字节数），进度提示写入 opts.Progress，由 adapter
// （CLI/脚本）决定如何呈现。
func Download(ctx context.Context, client *Client, key string, outputPath string, opts *DownloadOptions) (*DownloadResult, error) {
	if key == "" {
		return nil, ErrMissingKey
	}

	var options DownloadOptions
	if opts != nil {
		options = *opts
	}

	if options.VerifySHA256 != "" {
		// 严格要求 64 个十六进制字符（32 字节）："0000" 这类短串
		// 不是 SHA-256 digest，必须在参数阶段拒绝，否则只能等下载
		// 完成后误报 checksum mismatch。
		if digest, err := hex.DecodeString(options.VerifySHA256); err != nil || len(digest) != sha256.Size {
			return nil, fmt.Errorf("%w: --verify-sha256 must be 64 hex characters, got %q", ErrInvalidSHA256, options.VerifySHA256)
		}
	}

	switch options.IfExists {
	case "", IfExistsReplace:
		options.IfExists = IfExistsReplace
	case IfExistsVerify:
	default:
		return nil, fmt.Errorf("%w: %q (supported: replace, verify)", ErrInvalidIfExists, options.IfExists)
	}

	// --if-exists=verify 快速路径：本地副本可证明一致时跳过 GET
	if options.IfExists == IfExistsVerify {
		reused, result, err := tryReuseLocalCopy(ctx, client, key, outputPath, &options)
		if err != nil {
			return nil, err
		}
		if reused {
			return result, nil
		}
		// 本地不存在或无法证明一致（且无依据可判不一致）→ 继续正常下载
	}

	// 创建输出目录
	outputDir := filepath.Dir(outputPath)
	if outputDir != "" && outputDir != "." {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// 先取对象流，失败时不创建任何文件
	out, err := Get(ctx, client, key)
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()

	// --verify 依赖对象自带的 itb-sha256 metadata；缺失时无法校验
	var metadataSHA256 string
	if options.Verify {
		metadataSHA256 = strings.ToLower(out.Metadata[MetadataSHA256Key])
		if metadataSHA256 == "" {
			return nil, fmt.Errorf("verify: object %q has no %s metadata (uploaded by another tool?)", key, MetadataSHA256Key)
		}
	}

	// 期望值检查（创建最终目标之前）：响应头声明的 size 与 content-type
	declared := int64(-1)
	if out.ContentLength != nil {
		declared = *out.ContentLength
		if options.ExpectSize != nil && declared != *options.ExpectSize {
			return nil, fmt.Errorf("%w: object %q content-length is %d, expected %d", ErrExpectationMismatch, key, declared, *options.ExpectSize)
		}
	}
	remoteContentType := aws.ToString(out.ContentType)
	if options.ExpectContentType != "" && !contentTypeMatches(remoteContentType, options.ExpectContentType) {
		return nil, fmt.Errorf("%w: object %q content-type is %q, expected %q", ErrExpectationMismatch, key, remoteContentType, options.ExpectContentType)
	}

	// 写入同目录临时文件，成功后 rename：失败路径一律清理临时文件，
	// 目标路径不留下 partial 内容
	tmp, err := os.CreateTemp(outputDir, ".itb-download-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	// 对象声明的大小只用于进度提示；实际结果以写入字节为准
	var progress io.Writer = options.Progress
	if declared > 5*1024*1024 && progress != nil {
		fmt.Fprintf(progress, "Downloading (%.2f MB)...\n", float64(declared)/(1024*1024))
	}

	// 边下载边计算哈希，不二次读取本地文件
	needHash := options.Verify || options.VerifySHA256 != ""
	hasher := sha256.New()
	writers := []io.Writer{tmp}
	if needHash {
		writers = append(writers, hasher)
	}

	written, err := io.Copy(io.MultiWriter(writers...), out.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// 校验失败时临时文件由 defer 清理，目标路径不留下 partial 内容
	var computed string
	if needHash {
		computed = hex.EncodeToString(hasher.Sum(nil))
		if metadataSHA256 != "" && computed != metadataSHA256 {
			return nil, fmt.Errorf("%w: object %q content is %s, metadata says %s", ErrChecksumMismatch, key, computed, metadataSHA256)
		}
		if options.VerifySHA256 != "" && computed != strings.ToLower(options.VerifySHA256) {
			return nil, fmt.Errorf("%w: object %q content is %s, expected %s", ErrChecksumMismatch, key, computed, strings.ToLower(options.VerifySHA256))
		}
	}
	if options.ExpectSize != nil && written != *options.ExpectSize {
		return nil, fmt.Errorf("%w: object %q downloaded %d bytes, expected %d", ErrExpectationMismatch, key, written, *options.ExpectSize)
	}

	if err := tmp.Chmod(0o644); err != nil {
		return nil, fmt.Errorf("failed to set file mode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("failed to close output file: %w", err)
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		return nil, fmt.Errorf("failed to finalize output file: %w", err)
	}
	committed = true

	return &DownloadResult{
		SchemaVersion: DownloadSchemaVersion,
		Key:           key,
		OutputPath:    outputPath,
		Status:        StatusDownloaded,
		Size:          written,
		SHA256:        computed,
		ContentType:   remoteContentType,
	}, nil
}

// tryReuseLocalCopy 尝试按 --if-exists=verify 语义复用本地副本。
// 返回 reused=false 表示应继续正常下载（本地副本不存在）。
//
// 组合语义（与正常下载路径的验证依据完全一致，不因复用而打折）：
//
//   - --verify 与 --expect-content-type 都要求远端状态在场，必须
//     先 HEAD：--verify 要比对远端 itb-sha256，--expect-content-type
//     要比对远端 Content-Type；
//   - 同时提供 --verify 与 --verify-sha256 时，本地副本必须同时
//     满足两个依据（且远端 itb-sha256 不得与显式 digest 矛盾）；
//   - 只有 --verify-sha256（+ 可选 --expect-size）才允许真正的
//     零网络复用（local-cache 语义）。
func tryReuseLocalCopy(ctx context.Context, client *Client, key, outputPath string, options *DownloadOptions) (bool, *DownloadResult, error) {
	if options.VerifySHA256 == "" && !options.Verify {
		return false, nil, ErrReuseVerificationUnavailable
	}

	// 需要远端状态的组合：--verify（比对 itb-sha256）或
	// --expect-content-type（比对 Content-Type）
	needHead := options.Verify || options.ExpectContentType != ""
	remote := (*StatInfo)(nil)
	var remoteSHA string
	if needHead {
		info, err := Stat(ctx, client, key)
		if err != nil {
			return false, nil, err
		}
		remote = info
		remoteSHA = strings.ToLower(info.Metadata[MetadataSHA256Key])
		if options.Verify && remoteSHA == "" {
			return false, nil, fmt.Errorf("%w: object %q has no %s metadata, cannot verify a local copy", ErrExpectationMismatch, key, MetadataSHA256Key)
		}
	}

	// 期望 SHA：显式 digest 与远端 itb-sha256 必须同时满足；
	// 两者同时在场且互相矛盾时没有本地副本能满足，直接失败
	expectedSHA := strings.ToLower(options.VerifySHA256)
	if expectedSHA == "" {
		expectedSHA = remoteSHA
	} else if remoteSHA != "" && remoteSHA != expectedSHA {
		return false, nil, fmt.Errorf("%w: object %q %s metadata is %s, expected %s", ErrExpectationMismatch, key, MetadataSHA256Key, remoteSHA, expectedSHA)
	}

	if _, err := os.Stat(outputPath); err != nil {
		if os.IsNotExist(err) {
			// 本地没有副本：正常下载（下载后校验选项照常生效）
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("failed to stat output file: %w", err)
	}

	// 单遍计算本地副本的 size 与 SHA-256
	local, err := filehash.SumFile(outputPath, []filehash.Algorithm{filehash.SHA256})
	if err != nil {
		return false, nil, err
	}
	localSHA := local.Digests[filehash.SHA256]

	if localSHA != expectedSHA {
		return false, nil, fmt.Errorf("%w: local copy %q sha256 is %s, expected %s", ErrExpectationMismatch, outputPath, localSHA, expectedSHA)
	}
	if options.ExpectSize != nil && local.BytesRead != *options.ExpectSize {
		return false, nil, fmt.Errorf("%w: local copy %q size is %d, expected %d", ErrExpectationMismatch, outputPath, local.BytesRead, *options.ExpectSize)
	}
	// needHead 与 ExpectContentType != "" 等价：远端状态必在场
	var contentType string
	if remote != nil {
		contentType = remote.ContentType
		if options.ExpectContentType != "" && !contentTypeMatches(contentType, options.ExpectContentType) {
			return false, nil, fmt.Errorf("%w: object %q content-type is %q, expected %q", ErrExpectationMismatch, key, contentType, options.ExpectContentType)
		}
	}

	return true, &DownloadResult{
		SchemaVersion: DownloadSchemaVersion,
		Key:           key,
		OutputPath:    outputPath,
		Status:        StatusReused,
		Size:          local.BytesRead,
		SHA256:        localSHA,
		ContentType:   contentType,
	}, nil
}

// contentTypeMatches 比较 Content-Type：忽略参数部分（; charset=...）
// 与大小写。
func contentTypeMatches(got, expected string) bool {
	base := func(ct string) string {
		if i := strings.IndexByte(ct, ';'); i >= 0 {
			ct = ct[:i]
		}
		return strings.ToLower(strings.TrimSpace(ct))
	}
	if expected == "" {
		return false
	}
	return base(got) == base(expected)
}
