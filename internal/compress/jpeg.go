package compress

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// JPEGOptions JPEG 压缩选项
type JPEGOptions struct {
	Quality     int  // 压缩质量 1-100
	Progressive bool // 是否使用渐进式编码
	Optimize    bool // 是否优化霍夫曼表
	InputPath   string // 输入文件路径（djpeg 需要直接读取文件）
	Output      io.Writer
}

// CompressJPEG 执行 JPEG 压缩管道
// 管道: djpeg input.jpg | cjpeg -quality 80 -optimize -progressive
func CompressJPEG(opts JPEGOptions) error {
	djpegPath, err := EnsureBinary(DJpeg)
	if err != nil {
		return err
	}

	cjpegPath, err := EnsureBinary(CJpeg)
	if err != nil {
		return err
	}

	// 构建 cjpeg 参数
	cjpegArgs := []string{
		"-quality", fmt.Sprintf("%d", opts.Quality),
	}
	if opts.Optimize {
		cjpegArgs = append(cjpegArgs, "-optimize")
	}
	if opts.Progressive {
		cjpegArgs = append(cjpegArgs, "-progressive")
	}

	// djpeg 读取文件，输出到 stdout
	djpegCmd := exec.Command(djpegPath, opts.InputPath)

	// cjpeg 从 stdin 读取，输出到 stdout
	cjpegCmd := exec.Command(cjpegPath, cjpegArgs...)

	// 连接管道：djpeg stdout -> cjpeg stdin
	pipe, err := djpegCmd.StdoutPipe()
	if err != nil {
		return err
	}
	cjpegCmd.Stdin = pipe
	cjpegCmd.Stdout = opts.Output
	cjpegCmd.Stderr = os.Stderr

	// 启动 djpeg
	if err := djpegCmd.Start(); err != nil {
		return err
	}

	// 运行 cjpeg 并等待完成
	if err := cjpegCmd.Run(); err != nil {
		djpegCmd.Wait()
		return err
	}

	// 等待 djpeg 完成
	return djpegCmd.Wait()
}
