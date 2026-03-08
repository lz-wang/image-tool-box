package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"img-compress/internal/compress"
)

//go:embed bins/**
var binaries embed.FS

var (
	// 版本信息，编译时注入
	version = "dev"
)

func main() {
	// 初始化嵌入的二进制文件
	compress.InitBinaries(binaries)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "img-compress",
	Short: "高效的图片压缩工具",
	Long: `一个基于 pngquant、oxipng 和 libjpeg-turbo 的图片压缩 CLI 工具。

支持 PNG 和 JPEG 格式的高效压缩，所有依赖二进制已内嵌，无需外部依赖。`,
}

var pngCmd = &cobra.Command{
	Use:   "png",
	Short: "压缩 PNG 图片",
	Long: `使用 pngquant + oxipng 双重管道压缩 PNG 图片。

压缩流程：
1. pngquant 进行有损压缩（减少颜色数量）
2. oxipng 进行无损优化（优化 PNG 结构）`,
	Example: `  # 基本用法
  img-compress png -i photo.png

  # 指定输出文件
  img-compress png -i photo.png -o compressed.png

  # 自定义质量范围
  img-compress png -i photo.png -q 70-90`,
	RunE: runPNG,
}

var jpegCmd = &cobra.Command{
	Use:     "jpeg",
	Aliases: []string{"jpg"},
	Short:   "压缩 JPEG 图片",
	Long: `使用 libjpeg-turbo (djpeg + cjpeg) 管道压缩 JPEG 图片。

压缩流程：
1. djpeg 将 JPEG 解码为 PPM 格式
2. cjpeg 使用指定质量重新编码`,
	Example: `  # 基本用法
  img-compress jpeg -i photo.jpg

  # 指定输出文件和质量
  img-compress jpeg -i photo.jpg -o compressed.jpg -q 85`,
	RunE: runJPEG,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("img-compress version %s\n", version)
	},
}

// PNG 命令参数
var (
	pngInput       string
	pngOutput      string
	pngQuality     string
	pngOxiPngLevel int
)

// JPEG 命令参数
var (
	jpegInput       string
	jpegOutput      string
	jpegQuality     int
	jpegProgressive bool
)

func init() {
	rootCmd.AddCommand(pngCmd)
	rootCmd.AddCommand(jpegCmd)
	rootCmd.AddCommand(versionCmd)

	// PNG 命令参数
	pngCmd.Flags().StringVarP(&pngInput, "input", "i", "", "输入 PNG 文件路径")
	pngCmd.Flags().StringVarP(&pngOutput, "output", "o", "", "输出 PNG 文件路径")
	pngCmd.Flags().StringVarP(&pngQuality, "quality", "q", "60-80", "压缩质量范围 (min-max)")
	pngCmd.Flags().IntVar(&pngOxiPngLevel, "oxipng-level", 4, "oxipng 优化级别 (0-6)")

	// JPEG 命令参数
	jpegCmd.Flags().StringVarP(&jpegInput, "input", "i", "", "输入 JPEG 文件路径")
	jpegCmd.Flags().StringVarP(&jpegOutput, "output", "o", "", "输出 JPEG 文件路径")
	jpegCmd.Flags().IntVarP(&jpegQuality, "quality", "q", 80, "压缩质量 (1-100)")
	jpegCmd.Flags().BoolVar(&jpegProgressive, "progressive", true, "使用渐进式编码")
}

func runPNG(cmd *cobra.Command, args []string) error {
	// 确定输入源
	var input *os.File
	if pngInput != "" {
		f, err := os.Open(pngInput)
		if err != nil {
			return fmt.Errorf("无法打开输入文件: %w", err)
		}
		defer f.Close()
		input = f
	} else {
		// 从 stdin 读取
		input = os.Stdin
	}

	// 确定输出目标
	var output *os.File
	var outputPath string
	var tmpFile *os.File

	if pngOutput != "" {
		f, err := os.Create(pngOutput)
		if err != nil {
			return fmt.Errorf("无法创建输出文件: %w", err)
		}
		defer f.Close()
		output = f
		outputPath = pngOutput
	} else if pngInput != "" {
		// 覆盖原文件（先写入临时文件）
		var err error
		tmpFile, err = os.CreateTemp("", "img-compress-*.png")
		if err != nil {
			return err
		}
		output = tmpFile
		outputPath = pngInput
	} else {
		// 输出到 stdout
		output = os.Stdout
	}

	opts := compress.PNGOptions{
		Quality:     pngQuality,
		OxiPngLevel: pngOxiPngLevel,
		Input:       input,
		Output:      output,
	}

	if err := compress.CompressPNG(opts); err != nil {
		return err
	}

	// 如果是覆盖原文件
	if tmpFile != nil {
		tmpFile.Close()
		if err := os.Rename(tmpFile.Name(), pngInput); err != nil {
			return fmt.Errorf("无法覆盖原文件: %w", err)
		}
	}

	if outputPath != "" {
		fmt.Printf("压缩完成: %s\n", outputPath)
	}

	return nil
}

func runJPEG(cmd *cobra.Command, args []string) error {
	// JPEG 压缩需要指定输入文件路径
	if jpegInput == "" {
		return fmt.Errorf("必须指定输入文件路径 (-i)")
	}

	// 检查输入文件是否存在
	if _, err := os.Stat(jpegInput); err != nil {
		return fmt.Errorf("无法访问输入文件: %w", err)
	}

	// 确定输出目标
	var output *os.File
	var outputPath string
	var tmpFile *os.File

	if jpegOutput != "" {
		f, err := os.Create(jpegOutput)
		if err != nil {
			return fmt.Errorf("无法创建输出文件: %w", err)
		}
		defer f.Close()
		output = f
		outputPath = jpegOutput
	} else {
		// 覆盖原文件（先写入临时文件）
		var err error
		tmpFile, err = os.CreateTemp("", "img-compress-*.jpg")
		if err != nil {
			return err
		}
		output = tmpFile
		outputPath = jpegInput
	}

	opts := compress.JPEGOptions{
		Quality:     jpegQuality,
		Progressive: jpegProgressive,
		Optimize:    true,
		InputPath:   jpegInput,
		Output:      output,
	}

	if err := compress.CompressJPEG(opts); err != nil {
		return err
	}

	// 如果是覆盖原文件
	if tmpFile != nil {
		tmpFile.Close()
		if err := os.Rename(tmpFile.Name(), jpegInput); err != nil {
			return fmt.Errorf("无法覆盖原文件: %w", err)
		}
	}

	if outputPath != "" {
		fmt.Printf("压缩完成: %s\n", outputPath)
	}

	return nil
}
