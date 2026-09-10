package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"imagetoolbox/internal/stablefile"
)

// MetadataSHA256Key 上传时写入对象用户 metadata 的内容 SHA-256 键名。
// --skip-unchanged 依赖该值判断远端对象与本地是否一致；
// 不使用 ETag 做该判断（multipart 上传、SSE 与部分 S3 兼容实现下
// ETag 不是可靠的内容哈希）。
const MetadataSHA256Key = "itb-sha256"

// 机器可读输出契约（--format json）的 schema 版本。脚本消费方依赖
// JSON 结构稳定时，应以 schema_version 判断契约版本，而不是解析
// 终端文本。stat 与 upload/download 各自独立演进。
// v2：新增 status 字段（uploaded/skipped/reused），skipped/reason
// 兼容保留。
const (
	UploadSchemaVersion = "itb.s3.upload.v2"
)

// UploadResult.Status 的取值。
const (
	StatusUploaded = "uploaded"
	StatusSkipped  = "skipped"
	StatusReused   = "reused"
)

// ErrSkipStrategyConflict 同时指定多个互斥的跳过策略
//（--skip-existing / --skip-unchanged / --skip-matching）。
var ErrSkipStrategyConflict = errors.New("only one skip strategy can be enabled")

// UploadOptions 上传选项
type UploadOptions struct {
	ContentType string

	// CacheControl / ContentDisposition / ContentEncoding 写入对象的
	// 标准 HTTP 响应头（如 Cache-Control: no-cache），留空则不设置。
	CacheControl       string
	ContentDisposition string
	ContentEncoding    string

	// Metadata 写入对象的自定义用户 metadata（x-amz-meta-*）。
	// 键经 NormalizeMetadata 归一化；itb-sha256 为保留键。
	Metadata map[string]string

	// Progress 接收大文件（>5MB）传输提示等进度信息，nil 表示不输出。
	// 进度信息不是执行结果，domain 不直接写 stdout，由 adapter 决定
	// 输出去向（CLI 传 os.Stderr，保持 stdout 只承载正式结果）。
	Progress io.Writer

	// SkipExisting 为 true 时，对象键已存在即跳过上传（同名跳过）。
	SkipExisting bool

	// SkipUnchanged 为 true 时，仅当远端 metadata 中的 itb-sha256
	// 与本地文件 SHA-256 一致才跳过上传（内容一致跳过）。
	// 语义锁定不变。
	SkipUnchanged bool

	// SkipMatching 为 true 时，远端对象完整状态与本次上传预期一致才
	// 跳过（复用远端对象）：始终比对 SHA-256、Content-Length、
	// Content-Type；调用方显式指定的 Cache-Control、Content-Disposition、
	// Content-Encoding 与 metadata 也必须匹配（requested subset
	// matching：远端多出的 metadata 不影响匹配，未指定的 header 表示
	// don't care 而非"要求为空"）。
	SkipMatching bool

	// Verify 为 true 时，PUT 成功后追加 1 次 HeadObject，比对远端
	// size / Content-Type / 标准 HTTP 头 / metadata 与本次 PUT 的
	// 预期是否一致，不一致返回 ErrVerifyFailed。
	// 跳过语义命中时不产生额外请求（HEAD preflight 已证明对象状态）。
	Verify bool

	// IfExists 决定上传目标的写入策略：replace（默认，无条件覆盖，
	// v0.9.x 行为）或 verify（不可覆盖条件上传）。
	//
	// verify 使用 PutObjectInput.IfNoneMatch="*"（条件写）实现：
	// 对象不存在 → PUT 成功；已存在 → provider 返回 412，随后 HEAD
	// 按完整状态匹配（matchesExpectedState，与 --skip-matching 同一
	// 事实来源）决定 reused 或 E_TARGET_CONFLICT。绝不以
	// "HEAD + 判断 + PUT" 模拟——那存在 TOCTOU 竞态。
	// 并发冲突 409 ConditionalRequestConflict 由 SDK retryer 重试。
	IfExists IfExistsBehavior
}

// UploadResult 上传结果
type UploadResult struct {
	// SchemaVersion 机器可读契约版本（itb.s3.upload.v2）
	SchemaVersion string `json:"schema_version"`

	// Key 实际写入的对象键
	Key string `json:"key"`

	// Size 上传对象的字节数；命中跳过规则时为本地输入文件的字节数
	//（skip-existing 时 SHA256 未知但 Size 已由 file.Stat() 得知，
	// 不输出误导性的 size: 0）
	Size int64 `json:"size"`

	// SHA256 本地文件内容 SHA-256，同时写入 itb-sha256 metadata；
	// skip-existing 命中时未计算、留空
	SHA256 string `json:"sha256"`

	// Status 是本次上传的结果状态：uploaded（已上传）/ skipped
	//（命中 skip-existing/skip-unchanged）/ reused（远端状态与预期
	// 完全一致，复用远端对象，v2 新增）
	Status string `json:"status"`

	// Skipped 表示未执行上传。兼容保留：新脚本应改读 status。
	Skipped bool `json:"skipped"`

	// Reason 跳过原因，仅 Skipped 为 true 时有值
	Reason string `json:"reason,omitempty"`
}

// Upload 上传文件到存储桶。
//
// 执行顺序：open → HEAD preflight（仅启用跳过语义时）→ 复制到私有
// 临时快照（单遍 SHA-256）→ 源文件变化检测 → 从快照 PUT。
//
// 快照保证：itb-sha256 metadata 与实际 PUT body 严格对应（两者来自
// 同一次读取）、AWS SDK retry 每次 rewind 读取的都是同一份稳定数据、
// 原文件在快照之后的任何变化都不影响本次上传、原路径只读取一次。
// 快照期间源文件发生可观察变化时整个上传失败（ErrSourceChanged），
// 不上传内容不一致的数据。
//
// HEAD preflight 必须先于快照：--skip-existing 命中时直接返回，
// 0 字节本地读取；--skip-unchanged 复用同一次 HEAD 结果与快照哈希
// 比对，单次上传最多 1 × HEAD + 1 × PUT。--verify 在 PUT 成功后
// 追加 1 次 HEAD 回读校验（skip 命中时不追加，preflight HEAD 已
// 证明对象状态）。
//
// 上传时把本地文件 SHA-256 写入 itb-sha256 用户 metadata，供后续
// --skip-unchanged 比对。默认无条件覆盖已存在对象；
// SkipExisting/SkipUnchanged 只增加跳过语义，不改变默认行为。
//
// Content-Type 基于快照内容检测（保证与实际上传内容一致）；扩展名
// 兜底仍使用原始文件名。
//
// 本函数不输出任何内容：结果通过 UploadResult 返回，进度提示写入
// opts.Progress，由 adapter（CLI/脚本）决定如何呈现。
func Upload(ctx context.Context, client *Client, inputPath string, key string, opts *UploadOptions) (*UploadResult, error) {
	if inputPath == "" {
		return nil, ErrMissingInput
	}
	if key == "" {
		return nil, ErrMissingKey
	}
	if opts != nil {
		strategies := 0
		for _, enabled := range []bool{opts.SkipExisting, opts.SkipUnchanged, opts.SkipMatching} {
			if enabled {
				strategies++
			}
		}
		if strategies > 1 {
			return nil, ErrSkipStrategyConflict
		}
	}

	// 用户 metadata 在任何网络请求之前完成归一化校验，
	// 非法参数不产生副作用。
	var metadata map[string]string
	if opts != nil {
		normalized, err := NormalizeMetadata(opts.Metadata)
		if err != nil {
			return nil, err
		}
		metadata = normalized
	}

	// 打开文件但不读取内容，输入文件不存在时立即报错
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// HEAD preflight 必须发生在快照之前
	var remote *StatInfo

	if opts != nil && (opts.SkipExisting || opts.SkipUnchanged || opts.SkipMatching) {
		remote, err = statUploadTarget(ctx, client, key)
		if err != nil {
			return nil, err
		}

		if remote != nil && opts.SkipExisting {
			// 同名即跳过，远端内容与本地无关；SHA256 未计算留空，
			// 但 Size 已由 file.Stat() 得知，填充以免 JSON 消费方
			// 看到误导性的 size: 0
			return &UploadResult{
				SchemaVersion: UploadSchemaVersion,
				Key:           key,
				Size:          fileInfo.Size(),
				Status:        StatusSkipped,
				Skipped:       true,
				Reason:        "object already exists",
			}, nil
		}
	}

	// 稳定快照：复制到私有临时文件并单遍计算 SHA-256，随后检测源文件
	// 可观察变化。失败路径全部由 stablefile 清理快照。
	snapshot, err := stablefile.Snapshot(inputPath, file, fileInfo)
	if err != nil {
		return nil, err
	}
	defer snapshot.Close()
	snapshotPath, sha256Value := snapshot.Path(), snapshot.SHA256()

	if opts != nil && opts.SkipUnchanged && isUnchanged(remote, sha256Value) {
		// 命中即远端 itb-sha256 与本地一致，Size 与 SHA256 都是确切值
		return &UploadResult{
			SchemaVersion: UploadSchemaVersion,
			Key:           key,
			Size:          fileInfo.Size(),
			SHA256:        sha256Value,
			Status:        StatusSkipped,
			Skipped:       true,
			Reason:        "content unchanged (itb-sha256 match)",
		}, nil
	}

	// 从快照重新打开作为 PUT body：*os.File 实现 io.Seeker，
	// SDK retry 可以安全 rewind 重读同一份稳定数据
	body, err := os.Open(snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("failed to reopen upload snapshot: %w", err)
	}
	defer body.Close()

	// 读取快照头部做内容检测（与实际上传内容严格一致）
	var header [sniffLen]byte
	headerSize, err := body.ReadAt(header[:], 0)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read upload snapshot header: %w", err)
	}

	// Content-Type 内容优先：显式 --content-type > 快照 magic sniff >
	// 原始文件名扩展名兜底，防止 HTML/XML 错误页借 .jpg 扩展名以
	// image/jpeg 上传。
	var explicitContentType string
	var ifExists IfExistsBehavior
	if opts != nil {
		explicitContentType = opts.ContentType
		ifExists = opts.IfExists
	}
	switch ifExists {
	case "", IfExistsReplace:
		ifExists = IfExistsReplace
	case IfExistsVerify:
	default:
		return nil, fmt.Errorf("%w: %q (supported: replace, verify)", ErrInvalidIfExists, ifExists)
	}
	contentType := ResolveContentType(header[:headerSize], filepath.Base(inputPath), explicitContentType)

	fileSize := fileInfo.Size()

	// 执行上传：itb-sha256 与用户 metadata 合并写入，用户不可覆盖
	// 保留键（NormalizeMetadata 已拒绝）。
	objectMetadata := make(map[string]string, len(metadata)+1)
	maps.Copy(objectMetadata, metadata)
	objectMetadata[MetadataSHA256Key] = sha256Value

	// 本次上传的完整预期状态：--skip-matching 的跳过判定与 --verify
	// 的回读校验共用同一比较逻辑（唯一事实来源）。
	var cacheControl, contentDisposition, contentEncoding string
	if opts != nil {
		cacheControl = opts.CacheControl
		contentDisposition = opts.ContentDisposition
		contentEncoding = opts.ContentEncoding
	}
	expect := uploadExpectations{
		Size:               fileSize,
		ContentType:        contentType,
		CacheControl:       cacheControl,
		ContentDisposition: contentDisposition,
		ContentEncoding:    contentEncoding,
		Metadata:           objectMetadata,
	}

	// --skip-matching 命中：远端已是完整预期状态，复用远端对象
	if opts != nil && opts.SkipMatching && matchesExpectedState(remote, expect) {
		return &UploadResult{
			SchemaVersion: UploadSchemaVersion,
			Key:           key,
			Size:          fileSize,
			SHA256:        sha256Value,
			Status:        StatusReused,
			Skipped:       true,
			Reason:        "remote object state matches (sha256/size/content-type/headers)",
		}, nil
	}

	// 如果文件大于 5MB，向 Progress 输出传输提示
	var progress io.Writer
	if opts != nil {
		progress = opts.Progress
	}
	if fileSize > 5*1024*1024 && progress != nil {
		fmt.Fprintf(progress, "Uploading %s (%.2f MB)...\n", inputPath, float64(fileSize)/(1024*1024))
	}

	putInput := &s3.PutObjectInput{
		Bucket:      aws.String(client.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
		Metadata:    objectMetadata,
	}
	if ifExists == IfExistsVerify {
		// 不可覆盖条件上传：直接 PUT If-None-Match="*"，由 provider
		// 原子判定"不存在才写入"，绝不模拟 HEAD + 判断 + PUT
		putInput.IfNoneMatch = aws.String("*")
	}
	if opts != nil {
		if opts.CacheControl != "" {
			putInput.CacheControl = aws.String(opts.CacheControl)
		}
		if opts.ContentDisposition != "" {
			putInput.ContentDisposition = aws.String(opts.ContentDisposition)
		}
		if opts.ContentEncoding != "" {
			putInput.ContentEncoding = aws.String(opts.ContentEncoding)
		}
	}

	_, err = client.client.PutObject(ctx, putInput)
	if err != nil {
		if ifExists == IfExistsVerify {
			switch conditionalWriteError(err) {
			case "precondition_failed":
				// 412：对象已存在。HEAD 后按完整预期状态匹配决定
				// reused / 冲突（与 --skip-matching 唯一事实来源）
				remote, statErr := statUploadTarget(ctx, client, key)
				if statErr != nil {
					return nil, statErr
				}
				if matchesExpectedState(remote, expect) {
					return &UploadResult{
						SchemaVersion: UploadSchemaVersion,
						Key:           key,
						Size:          fileSize,
						SHA256:        sha256Value,
						Status:        StatusReused,
						Skipped:       true,
						Reason:        "object already exists with the expected state",
					}, nil
				}
				return nil, fmt.Errorf("%w: object %q already exists and differs from the expected state", ErrExpectationMismatch, key)
			case "conflict":
				// 409 ConditionalRequestConflict：并发条件写冲突，
				// SDK retryer 重试耗尽后的残余错误
				return nil, fmt.Errorf("%w: concurrent conditional writes on %q; retries exhausted", ErrExpectationMismatch, key)
			case "unsupported":
				// provider 明确不支持条件写：绝不降级为 HEAD + PUT
				return nil, fmt.Errorf("%w: conditional upload (If-None-Match) rejected by provider; refusing to fall back to an unsafe non-conditional PUT", ErrUnsupportedCapability)
			}
		}
		return nil, WrapError(err)
	}

	if opts != nil && opts.Verify {
		if err := verifyUpload(ctx, client, key, expect); err != nil {
			return nil, err
		}
	}

	return &UploadResult{SchemaVersion: UploadSchemaVersion, Key: key, Size: fileSize, SHA256: sha256Value, Status: StatusUploaded}, nil
}

// uploadExpectations 记录本次 PUT 写入的对象属性，供 --skip-matching
// 的跳过判定与 --verify 的 HEAD 回读比对共用。
type uploadExpectations struct {
	Size               int64
	ContentType        string
	CacheControl       string
	ContentDisposition string
	ContentEncoding    string
	Metadata           map[string]string
}

// expectedStateMismatch 比对远端对象状态与本次上传的预期，返回第一处
// 不一致的描述；完全一致返回空串。这是 upload 状态比较的唯一事实来源：
//
//   - SHA-256（itb-sha256 metadata）、Content-Length、Content-Type
//     始终比对；
//   - Cache-Control / Content-Disposition / Content-Encoding 仅在
//     调用方显式指定时比对（未指定 = don't care，不要求远端为空）；
//   - metadata 采用 requested subset matching：预期中的每个键都必须
//     原样在场且相等，远端多出的 metadata 不影响匹配。
func expectedStateMismatch(remote *StatInfo, expect uploadExpectations) string {
	if remote == nil {
		return "remote object does not exist"
	}
	if remote.Size != expect.Size {
		return fmt.Sprintf("content-length: got %d, want %d", remote.Size, expect.Size)
	}
	if remote.ContentType != expect.ContentType {
		return fmt.Sprintf("content-type: got %q, want %q", remote.ContentType, expect.ContentType)
	}
	for _, field := range []struct {
		name string
		got  string
		want string
	}{
		{"cache-control", remote.CacheControl, expect.CacheControl},
		{"content-disposition", remote.ContentDisposition, expect.ContentDisposition},
		{"content-encoding", remote.ContentEncoding, expect.ContentEncoding},
	} {
		if field.want == "" {
			continue
		}
		if field.got != field.want {
			return fmt.Sprintf("%s: got %q, want %q", field.name, field.got, field.want)
		}
	}

	// metadata 键在写入与 HEAD 回读时均为小写；逐键精确比对，
	// itb-sha256 与所有用户 metadata 都必须原样在场。
	for k, want := range expect.Metadata {
		got, ok := remote.Metadata[k]
		if !ok {
			return fmt.Sprintf("metadata %q missing on remote object", k)
		}
		if got != want {
			return fmt.Sprintf("metadata %q: got %q, want %q", k, got, want)
		}
	}
	return ""
}

// matchesExpectedState 报告远端对象是否已处于本次上传的完整预期状态。
func matchesExpectedState(remote *StatInfo, expect uploadExpectations) bool {
	return expectedStateMismatch(remote, expect) == ""
}

// verifyUpload 对刚上传的对象执行 1 次 HeadObject，比对远端返回的
// header/metadata 与 PUT 预期是否一致，不一致返回 ErrVerifyFailed。
//
// 注意：HEAD verify 只能证明 metadata/header 与预期一致，不等于
// body SHA-256 校验；body 完整性校验由 download 的校验选项承担。
func verifyUpload(ctx context.Context, client *Client, key string, expect uploadExpectations) error {
	info, err := Stat(ctx, client, key)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}

	if detail := expectedStateMismatch(info, expect); detail != "" {
		return fmt.Errorf("%w: %s", ErrVerifyFailed, detail)
	}
	return nil
}

// statUploadTarget 对上传目标执行 1 次 HeadObject preflight。
// 对象不存在（404）返回 nil，由调用方继续上传；
// 403 等权限错误原样返回，绝不当作"不存在"。
func statUploadTarget(ctx context.Context, client *Client, key string) (*StatInfo, error) {
	info, err := Stat(ctx, client, key)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return info, nil
}

// isUnchanged 判断远端对象的 itb-sha256 metadata 与本地哈希是否一致。
func isUnchanged(remote *StatInfo, localSHA256 string) bool {
	return remote != nil &&
		remote.Metadata != nil &&
		remote.Metadata[MetadataSHA256Key] == localSHA256
}
