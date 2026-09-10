package compress

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"imagetoolbox/internal/filehash"
	"imagetoolbox/internal/imageio"
	"imagetoolbox/internal/stablefile"
)

const DefaultQuality = 80

// Processor 名称锁定进机器可读输出契约（itb.compress.v1），不得随意改名。
const (
	ProcessorPNG  = "pngquant+oxipng"
	ProcessorJPEG = "djpeg+cjpeg"
)

// CompressSchemaVersion 是 compress --format json 的机器可读契约版本。
const CompressSchemaVersion = "itb.compress.v1"

// FileOptions 文件级压缩选项
type FileOptions struct {
	Quality int // 压缩质量 1-100
}

// Normalize applies the domain defaults shared by every adapter.
func (o *FileOptions) Normalize() {
	if o.Quality == 0 {
		o.Quality = DefaultQuality
	}
}

// Validate verifies file-level compression options.
func (o FileOptions) Validate() error {
	if o.Quality < 1 || o.Quality > 100 {
		return fmt.Errorf("压缩质量必须在 1-100 范围内: %d", o.Quality)
	}
	return nil
}

// Result 文件压缩结果
type Result struct {
	Format     string // 检测到的输入格式（png/jpeg）
	InputSize  int64  // 输入文件字节数
	OutputSize int64  // 输出文件字节数

	// InputSHA256 / OutputSHA256 是压缩前后内容的 SHA-256（单遍流式
	// 计算；输入侧由 SumFile 附带可观察变化检测）
	InputSHA256  string
	OutputSHA256 string

	// Quality 是实际生效的质量值（经 Normalize 归一化）
	Quality int

	// Processor 是执行本次压缩的处理器管线名（pngquant+oxipng 或
	// djpeg+cjpeg），锁定进机器可读契约
	Processor string

	// ElapsedMs 是压缩流程耗时（毫秒）
	ElapsedMs int64
}

// FileStat 是机器可读报告中单个文件的摘要。
type FileStat struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// CompressReport 是 compress --format json 的机器可读契约
//（itb.compress.v1）。
type CompressReport struct {
	SchemaVersion string   `json:"schema_version"`
	Input         FileStat `json:"input"`
	Output        FileStat `json:"output"`
	Quality       int      `json:"quality"`
	Processor     string   `json:"processor"`
	ElapsedMs     int64    `json:"elapsed_ms"`
}

// NewReport 由压缩结果构造机器可读报告。格式未知（压缩失败）时不可调用。
func NewReport(inputPath, outputPath string, r Result) CompressReport {
	return CompressReport{
		SchemaVersion: CompressSchemaVersion,
		Input:         FileStat{Path: inputPath, Format: r.Format, Size: r.InputSize, SHA256: r.InputSHA256},
		Output:        FileStat{Path: outputPath, Format: r.Format, Size: r.OutputSize, SHA256: r.OutputSHA256},
		Quality:       r.Quality,
		Processor:     r.Processor,
		ElapsedMs:     r.ElapsedMs,
	}
}

// CompressFile 检测输入图片格式（PNG/JPEG），执行对应的压缩管道并写入 outputPath。
// 供 CLI 与 Web API 共用。inputPath 与 outputPath 不得指向同一文件。
//
// 输入基于一份稳定快照处理（internal/stablefile）：源文件先复制到
// 私有临时快照并单遍计算 SHA-256，格式检测与压缩管道全部读取快照；
// 报告中的 input hash/size 由此与实际压缩内容严格对应（源文件在
// 快照期间发生可观察变化时直接失败）。报告中的 input path 仍是用户
// 原始路径。
//
// 输出采用安全提交流程：压缩结果先写入目标目录下的 .itb-compress-*
// 临时文件，成功并校验（可关闭、非空、格式正确）后原子 rename 到
// outputPath；任何失败都会删除临时文件——已存在的目标保持原状，
// 目标路径不会留下 partial 内容。原地覆盖输入仍须由调用方先写入另一
// 临时文件，再原子 rename 回输入路径。
func CompressFile(ctx context.Context, inputPath, outputPath string, opts FileOptions) (Result, error) {
	start := time.Now()
	ctx = commandContext(ctx)
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	opts.Normalize()
	if err := opts.Validate(); err != nil {
		return Result{}, err
	}
	if outputPath == "" {
		return Result{}, fmt.Errorf("必须指定输出文件路径")
	}

	if stat, err := os.Stat(inputPath); err != nil {
		return Result{}, fmt.Errorf("无法读取输入文件信息: %w", err)
	} else if !stat.Mode().IsRegular() {
		// FIFO 会在 open 阶段阻塞，目录读不出内容：输入必须是普通文件
		return Result{}, fmt.Errorf("输入必须是普通文件: %s", inputPath)
	}
	if err := imageio.RejectSameFile(inputPath, outputPath); err != nil {
		return Result{}, err
	}

	file, err := os.Open(inputPath)
	if err != nil {
		return Result{}, fmt.Errorf("无法打开输入文件: %w", err)
	}
	defer file.Close()
	initial, err := file.Stat()
	if err != nil {
		return Result{}, fmt.Errorf("无法读取输入文件信息: %w", err)
	}

	// 稳定快照：复制 + SHA-256 单遍完成，随后检测可观察变化。
	// 之后的格式检测与压缩全部基于快照，报告摘要与实际压缩内容
	// 严格一致；失败路径由 stablefile 清理快照。
	snapshot, err := stablefile.Snapshot(inputPath, file, initial)
	if err != nil {
		return Result{}, err
	}
	defer snapshot.Close()

	header, err := snapshot.Open()
	if err != nil {
		return Result{}, fmt.Errorf("无法打开输入快照: %w", err)
	}
	format, err := DetectFormat(header)
	header.Close()
	if err != nil {
		return Result{}, fmt.Errorf("无法检测图片格式: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	// 安全提交：输出先落目标目录临时文件，全部成功并校验后 rename
	outStat, err := commitOutput(outputPath, format, func(tmp *os.File) error {
		switch format {
		case "png":
			return compressPNGTo(ctx, snapshot.Path(), tmp, opts.Quality)
		case "jpeg":
			return compressJPEGTo(ctx, snapshot.Path(), tmp, opts.Quality)
		default:
			return fmt.Errorf("%w: %s", ErrUnsupportedFormat, format)
		}
	})
	if err != nil {
		return Result{}, err
	}

	// 输出摘要（文件已提交且不再变动）
	outputHash, err := filehash.SumFile(outputPath, []filehash.Algorithm{filehash.SHA256})
	if err != nil {
		return Result{}, fmt.Errorf("无法计算输出文件摘要: %w", err)
	}

	processor := ProcessorPNG
	if format == "jpeg" {
		processor = ProcessorJPEG
	}

	return Result{
		Format:       format,
		InputSize:    snapshot.Size(),
		OutputSize:   outStat.Size(),
		InputSHA256:  snapshot.SHA256(),
		OutputSHA256: outputHash.Digests[filehash.SHA256],
		Quality:      opts.Quality,
		Processor:    processor,
		ElapsedMs:    time.Since(start).Milliseconds(),
	}, nil
}

// commitOutput 安全提交：把 compress 的输出写入目标目录下的
// .itb-compress-* 临时文件，成功并校验（非空、格式正确）后原子 rename
// 到 outputPath。任何失败都会清理临时文件——已存在的目标保持原状，
// 目标路径不会留下 partial 内容。
func commitOutput(outputPath, format string, compress func(tmp *os.File) error) (os.FileInfo, error) {
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), ".itb-compress-*")
	if err != nil {
		return nil, fmt.Errorf("无法创建临时输出文件: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	if err := compress(tmp); err != nil {
		return nil, err
	}

	// 关闭并校验临时输出：非空且格式与输入一致（pngquant/oxipng 与
	// djpeg/cjpeg 的产物必须是合法的同格式流）
	info, err := validateTempOutput(tmp, format)
	if err != nil {
		return nil, err
	}
	if err := tmp.Chmod(0o644); err != nil {
		return nil, fmt.Errorf("无法设置输出文件权限: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("无法关闭输出文件: %w", err)
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		return nil, fmt.Errorf("无法提交输出文件: %w", err)
	}
	committed = true
	return info, nil
}

// validateTempOutput 校验临时输出：可重新打开、非空、格式与输入一致。
func validateTempOutput(tmp *os.File, format string) (os.FileInfo, error) {
	if err := tmp.Sync(); err != nil {
		return nil, fmt.Errorf("无法写入输出文件: %w", err)
	}
	info, err := tmp.Stat()
	if err != nil {
		return nil, fmt.Errorf("无法读取输出文件信息: %w", err)
	}
	if info.Size() == 0 {
		return nil, fmt.Errorf("压缩输出为空")
	}

	check, err := os.Open(tmp.Name())
	if err != nil {
		return nil, fmt.Errorf("无法校验输出文件: %w", err)
	}
	outFormat, detectErr := DetectFormat(check)
	check.Close()
	if detectErr != nil || outFormat != format {
		return nil, fmt.Errorf("压缩输出格式校验失败: got %v, want %s", outFormat, format)
	}
	return info, nil
}

func compressPNGTo(ctx context.Context, inputPath string, output *os.File, quality int) error {
	input, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("无法打开输入文件: %w", err)
	}
	defer input.Close()

	return CompressPNG(PNGOptions{
		Context:     ctx,
		Quality:     quality,
		OxiPngLevel: 4,
		Input:       input,
		Output:      output,
	})
}

func compressJPEGTo(ctx context.Context, inputPath string, output *os.File, quality int) error {
	return CompressJPEG(JPEGOptions{
		Context:     ctx,
		Quality:     quality,
		Progressive: true,
		Optimize:    true,
		InputPath:   inputPath,
		Output:      output,
	})
}
