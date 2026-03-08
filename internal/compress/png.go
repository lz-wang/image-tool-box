package compress

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// PNGOptions PNG 压缩选项
type PNGOptions struct {
	Quality     string // 质量范围，如 "60-80"
	OxiPngLevel int    // oxipng 优化级别 0-6
	Input       io.Reader
	Output      io.Writer
}

// CompressPNG 执行 PNG 压缩管道
// 管道: pngquant --quality 60-80 --speed 1 --strip --output - - | oxipng -o 4 --strip all - --stdout
func CompressPNG(opts PNGOptions) error {
	// 第一阶段：pngquant 压缩
	pngquantOut, err := runPngQuant(opts)
	if err != nil {
		return fmt.Errorf("pngquant failed: %w", err)
	}

	// 第二阶段：oxipng 优化
	if err := runOxiPng(pngquantOut, opts); err != nil {
		return fmt.Errorf("oxipng failed: %w", err)
	}

	return nil
}

func runPngQuant(opts PNGOptions) (*bytes.Buffer, error) {
	binPath, err := EnsureBinary(PngQuant)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(
		binPath,
		"--quality", opts.Quality,
		"--speed", "1",
		"--strip",
		"--output", "-",
		"-",
	)

	cmd.Stdin = opts.Input
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return &out, nil
}

func runOxiPng(input *bytes.Buffer, opts PNGOptions) error {
	binPath, err := EnsureBinary(OxiPng)
	if err != nil {
		return err
	}

	cmd := exec.Command(
		binPath,
		fmt.Sprintf("-o%d", opts.OxiPngLevel),
		"--strip", "all",
		"-",
		"--stdout",
	)

	cmd.Stdin = input
	cmd.Stdout = opts.Output
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
