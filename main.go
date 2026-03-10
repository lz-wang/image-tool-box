package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "image/jpeg"
	_ "image/png"

	"github.com/spf13/cobra"
	"imagetoolbox/internal/compress"
	"imagetoolbox/internal/s3"
	"imagetoolbox/internal/watermark"
)

//go:embed bins/**
var binaries embed.FS

var (
	version = "dev"
)

func main() {
	compress.InitBinaries(binaries)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "imagetoolbox",
	Short: "高效的图片压缩工具",
	Long: `一个基于 pngquant、oxipng 和 libjpeg-turbo 的图片压缩 CLI 工具。

支持 PNG 和 JPEG 格式的高效压缩，所有依赖二进制已内嵌，无需外部依赖。`,
}

var compressCmd = &cobra.Command{
	Use:   "compress",
	Short: "自动检测并压缩图片",
	Long: `自动检测输入图片的格式（PNG/JPEG），然后执行对应的压缩操作。

无需指定图片类型，程序会通过读取文件头自动判断。`,
	Example: `  imagetoolbox compress -i photo.png
  imagetoolbox compress -i photo.jpg -o compressed.jpg -q 90`,
	RunE: runCompress,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("imagetoolbox version %s\n", version)
	},
}

var watermarkCmd = &cobra.Command{
	Use:   "watermark",
	Short: "为图片添加水印",
	Long: `为图片添加文字水印，支持两种模式：

1. position（默认）: 单点位置水印，在指定位置添加水印
   - 自动根据背景亮度选择黑/白文字
   - 支持自动描边提高可读性

2. repeat: 重复平铺水印，文字以平铺方式覆盖整张图片
   - 支持旋转角度和间距调整
   - 需要指定字体文件路径`,
	Example: `  # 位置水印（默认右下角，智能颜色）
  imagetoolbox watermark -i photo.jpg -t "Author"

  # 指定位置和透明度
  imagetoolbox watermark -i photo.png -t "Copyright" --position center --opacity 0.8

  # 重复平铺水印
  imagetoolbox watermark -i photo.png -t "WATERMARK" --mode repeat --font /path/to/font.ttf

  # 指定输出路径
  imagetoolbox watermark -i photo.jpg -t "Author" -o output.jpg`,
	RunE: runWatermark,
}

// S3 命令
var s3Cmd = &cobra.Command{
	Use:   "s3",
	Short: "S3 兼容存储操作",
	Long: `S3 兼容存储操作，支持 AWS S3、MinIO、阿里云 OSS、腾讯云 COS 等。

环境变量支持:
  AWS_ACCESS_KEY_ID       Access Key
  AWS_SECRET_ACCESS_KEY   Secret Key
  AWS_REGION              区域
  S3_ENDPOINT             自定义端点`,
}

var s3UploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "上传文件到存储桶",
	Long:  `上传本地文件到 S3 兼容存储桶。`,
	Example: `  # 上传文件
  imagetoolbox s3 upload -i photo.jpg -b my-bucket -e http://localhost:9000

  # 指定对象键名
  imagetoolbox s3 upload -i photo.jpg -b my-bucket -k images/photo.jpg

  # 指定 Content-Type
  imagetoolbox s3 upload -i data.json -b my-bucket --content-type application/json`,
	RunE: runS3Upload,
}

var s3DownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "从存储桶下载文件",
	Long:  `从 S3 兼容存储桶下载文件到本地。`,
	Example: `  # 下载文件
  imagetoolbox s3 download -b my-bucket -k photo.jpg -o ./photo.jpg

  # 使用默认文件名
  imagetoolbox s3 download -b my-bucket -k images/photo.jpg`,
	RunE: runS3Download,
}

var s3DeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "从存储桶删除对象",
	Long:  `从 S3 兼容存储桶删除指定对象。`,
	Example: `  # 删除对象（需要确认）
  imagetoolbox s3 delete -b my-bucket -k photo.jpg

  # 强制删除（不需要确认）
  imagetoolbox s3 delete -b my-bucket -k photo.jpg -f`,
	RunE: runS3Delete,
}

var s3ListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出存储桶中的对象",
	Long:  `列出 S3 兼容存储桶中的对象。`,
	Example: `  # 列出所有对象
  imagetoolbox s3 list -b my-bucket

  # 按前缀过滤
  imagetoolbox s3 list -b my-bucket -p images/

  # JSON 格式输出
  imagetoolbox s3 list -b my-bucket --format json`,
	RunE: runS3List,
}

var (
	inputFile  string
	outputFile string
	quality    int
)

// watermark 命令参数
var (
	wmInputFile  string
	wmOutputFile string
	wmText       string
	wmMode       string
	wmColor      string
	wmSpace      int
	wmAngle      int
	wmOpacity    float64
	wmFontPath   string
	wmFontSize   int
	wmPosition   string
	wmMargin     float64
)

// s3 公共参数
var (
	s3Endpoint       string
	s3AccessKey      string
	s3SecretKey      string
	s3Region         string
	s3Bucket         string
	s3ForcePathStyle bool
)

// s3 upload 参数
var (
	s3UploadInput       string
	s3UploadKey         string
	s3UploadContentType string
)

// s3 download 参数
var (
	s3DownloadKey    string
	s3DownloadOutput string
)

// s3 delete 参数
var (
	s3DeleteKey    string
	s3DeleteForce  bool
)

// s3 list 参数
var (
	s3ListPrefix  string
	s3ListMaxKeys int
	s3ListFormat  string
)

func init() {
	rootCmd.AddCommand(compressCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(watermarkCmd)

	compressCmd.Flags().StringVarP(&inputFile, "input", "i", "", "输入图片文件路径")
	compressCmd.Flags().StringVarP(&outputFile, "output", "o", "", "输出图片文件路径")
	compressCmd.Flags().IntVarP(&quality, "quality", "q", 80, "压缩质量 (1-100)")

	// watermark 命令参数
	watermarkCmd.Flags().StringVarP(&wmInputFile, "input", "i", "", "输入图片文件路径")
	watermarkCmd.Flags().StringVarP(&wmOutputFile, "output", "o", "", "输出图片文件路径（默认在原文件名后加 _watermarked）")
	watermarkCmd.Flags().StringVarP(&wmText, "text", "t", "", "水印文字")
	watermarkCmd.Flags().StringVarP(&wmMode, "mode", "m", "position", "水印模式: position（位置）/ repeat（重复平铺）")
	watermarkCmd.Flags().StringVar(&wmColor, "color", "", "水印颜色（repeat模式，空表示自动选择）")
	watermarkCmd.Flags().IntVar(&wmSpace, "space", 0, "平铺间距（0表示自动计算）")
	watermarkCmd.Flags().IntVar(&wmAngle, "angle", 30, "旋转角度（repeat模式）")
	watermarkCmd.Flags().Float64Var(&wmOpacity, "opacity", 0.5, "透明度 (0~1)")
	watermarkCmd.Flags().StringVar(&wmFontPath, "font", "", "字体文件路径")
	watermarkCmd.Flags().IntVar(&wmFontSize, "font-size", 0, "字体大小（0表示自动计算）")
	watermarkCmd.Flags().StringVar(&wmPosition, "position", "bottom-right", "水印位置: bottom-right/bottom-left/top-right/top-left/center")
	watermarkCmd.Flags().Float64Var(&wmMargin, "margin", 0.04, "边距比例（position模式）")

	watermarkCmd.MarkFlagRequired("input")
	watermarkCmd.MarkFlagRequired("text")

	// S3 命令注册
	rootCmd.AddCommand(s3Cmd)
	s3Cmd.AddCommand(s3UploadCmd)
	s3Cmd.AddCommand(s3DownloadCmd)
	s3Cmd.AddCommand(s3DeleteCmd)
	s3Cmd.AddCommand(s3ListCmd)

	// S3 公共参数（所有子命令共享）
	for _, cmd := range []*cobra.Command{s3UploadCmd, s3DownloadCmd, s3DeleteCmd, s3ListCmd} {
		cmd.Flags().StringVarP(&s3Endpoint, "endpoint", "e", "", "S3 端点 URL")
		cmd.Flags().StringVarP(&s3AccessKey, "access-key", "a", "", "Access Key ID（默认从环境变量读取）")
		cmd.Flags().StringVarP(&s3SecretKey, "secret-key", "s", "", "Secret Access Key（默认从环境变量读取）")
		cmd.Flags().StringVarP(&s3Region, "region", "r", "us-east-1", "区域")
		cmd.Flags().StringVarP(&s3Bucket, "bucket", "b", "", "存储桶名称")
		cmd.Flags().BoolVar(&s3ForcePathStyle, "force-path-style", false, "强制路径样式 URL（MinIO 需要）")
		cmd.MarkFlagRequired("bucket")
	}

	// S3 upload 参数
	s3UploadCmd.Flags().StringVarP(&s3UploadInput, "input", "i", "", "本地文件路径")
	s3UploadCmd.Flags().StringVarP(&s3UploadKey, "key", "k", "", "对象键名（默认使用文件名）")
	s3UploadCmd.Flags().StringVar(&s3UploadContentType, "content-type", "", "内容类型（自动检测）")
	s3UploadCmd.MarkFlagRequired("input")

	// S3 download 参数
	s3DownloadCmd.Flags().StringVarP(&s3DownloadKey, "key", "k", "", "对象键名")
	s3DownloadCmd.Flags().StringVarP(&s3DownloadOutput, "output", "o", "", "本地输出路径（默认使用对象键名）")
	s3DownloadCmd.MarkFlagRequired("key")

	// S3 delete 参数
	s3DeleteCmd.Flags().StringVarP(&s3DeleteKey, "key", "k", "", "对象键名")
	s3DeleteCmd.Flags().BoolVarP(&s3DeleteForce, "force", "f", false, "强制删除，不确认")
	s3DeleteCmd.MarkFlagRequired("key")

	// S3 list 参数
	s3ListCmd.Flags().StringVarP(&s3ListPrefix, "prefix", "p", "", "对象键前缀")
	s3ListCmd.Flags().IntVar(&s3ListMaxKeys, "max-keys", 1000, "最大返回数量")
	s3ListCmd.Flags().StringVar(&s3ListFormat, "format", "table", "输出格式: table/json/plain")
}

func runCompress(cmd *cobra.Command, args []string) error {
	if inputFile == "" {
		return fmt.Errorf("必须指定输入文件路径 (-i)")
	}

	f, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("无法打开输入文件: %w", err)
	}

	format, err := compress.DetectFormat(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("无法检测图片格式: %w", err)
	}

	fmt.Printf("检测到格式: %s\n", format)

	switch format {
	case "png":
		return compressPNGFile(inputFile, outputFile, quality)
	case "jpeg":
		return compressJPEGFile(inputFile, outputFile, quality)
	default:
		return fmt.Errorf("不支持的图片格式: %s", format)
	}
}

func compressPNGFile(inPath, outPath string, q int) error {
	input, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer input.Close()

	var output *os.File
	var outputPath string
	var tmpFile *os.File

	if outPath != "" {
		output, err = os.Create(outPath)
		if err != nil {
			return err
		}
		defer output.Close()
		outputPath = outPath
	} else {
		tmpFile, err = os.CreateTemp("", "imagetoolbox-*.png")
		if err != nil {
			return err
		}
		output = tmpFile
		outputPath = inPath
	}

	opts := compress.PNGOptions{
		Quality:     q,
		OxiPngLevel: 4,
		Input:       input,
		Output:      output,
	}

	if err := compress.CompressPNG(opts); err != nil {
		return err
	}

	if tmpFile != nil {
		tmpFile.Close()
		os.Rename(tmpFile.Name(), inPath)
	}

	fmt.Printf("压缩完成: %s\n", outputPath)
	return nil
}

func compressJPEGFile(inPath, outPath string, q int) error {
	var output *os.File
	var outputPath string
	var tmpFile *os.File
	var err error

	if outPath != "" {
		output, err = os.Create(outPath)
		if err != nil {
			return err
		}
		defer output.Close()
		outputPath = outPath
	} else {
		tmpFile, err = os.CreateTemp("", "imagetoolbox-*.jpg")
		if err != nil {
			return err
		}
		output = tmpFile
		outputPath = inPath
	}

	opts := compress.JPEGOptions{
		Quality:     q,
		Progressive: true,
		Optimize:    true,
		InputPath:   inPath,
		Output:      output,
	}

	if err := compress.CompressJPEG(opts); err != nil {
		return err
	}

	if tmpFile != nil {
		tmpFile.Close()
		os.Rename(tmpFile.Name(), inPath)
	}

	fmt.Printf("压缩完成: %s\n", outputPath)
	return nil
}

func runWatermark(cmd *cobra.Command, args []string) error {
	if wmInputFile == "" {
		return fmt.Errorf("必须指定输入文件路径 (-i)")
	}
	if wmText == "" {
		return fmt.Errorf("必须指定水印文字 (-t)")
	}

	// 生成默认输出路径
	outputPath := wmOutputFile
	if outputPath == "" {
		ext := filepath.Ext(wmInputFile)
		base := strings.TrimSuffix(filepath.Base(wmInputFile), ext)
		dir := filepath.Dir(wmInputFile)
		outputPath = filepath.Join(dir, base+"_watermarked"+ext)
	}

	var err error
	switch wmMode {
	case "repeat":
		opts := &watermark.RepeatOptions{
			Color:          &wmColor,
			Space:          &wmSpace,
			Angle:          &wmAngle,
			Opacity:        &wmOpacity,
			FontPath:       wmFontPath,
			FontSize:       &wmFontSize,
			FontHeightCrop: nil,
		}
		_, err = watermark.AddRepeatWatermark(wmInputFile, outputPath, wmText, opts)

	case "position":
		pos := watermark.Position(wmPosition)
		opts := &watermark.PositionOptions{
			Opacity:     &wmOpacity,
			Position:    pos,
			FontPath:    wmFontPath,
			FontSize:    &wmFontSize,
			Color:       &wmColor,
			MarginRatio: &wmMargin,
		}
		_, err = watermark.AddPositionWatermark(wmInputFile, outputPath, wmText, opts)

	default:
		return fmt.Errorf("不支持的水印模式: %s（支持: position, repeat）", wmMode)
	}

	if err != nil {
		return fmt.Errorf("添加水印失败: %w", err)
	}

	fmt.Printf("水印添加完成: %s\n", outputPath)
	return nil
}

// S3 辅助函数：创建客户端
func newS3Client(ctx context.Context) (*s3.Client, error) {
	cfg := &s3.Config{
		Endpoint:        s3Endpoint,
		AccessKeyID:     s3AccessKey,
		SecretAccessKey: s3SecretKey,
		Region:          s3Region,
		Bucket:          s3Bucket,
		ForcePathStyle:  s3ForcePathStyle,
	}
	cfg.LoadFromEnv()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return s3.NewClient(ctx, cfg)
}

func runS3Upload(cmd *cobra.Command, args []string) error {
	if s3UploadInput == "" {
		return fmt.Errorf("必须指定输入文件路径 (-i)")
	}

	client, err := newS3Client(cmd.Context())
	if err != nil {
		return err
	}

	// 默认使用文件名作为对象键
	key := s3UploadKey
	if key == "" {
		key = filepath.Base(s3UploadInput)
	}

	opts := &s3.UploadOptions{
		ContentType: s3UploadContentType,
	}

	return s3.Upload(cmd.Context(), client, s3UploadInput, key, opts)
}

func runS3Download(cmd *cobra.Command, args []string) error {
	if s3DownloadKey == "" {
		return fmt.Errorf("必须指定对象键名 (-k)")
	}

	client, err := newS3Client(cmd.Context())
	if err != nil {
		return err
	}

	// 默认使用对象键名作为本地文件名
	output := s3DownloadOutput
	if output == "" {
		output = filepath.Base(s3DownloadKey)
	}

	return s3.Download(cmd.Context(), client, s3DownloadKey, output, nil)
}

func runS3Delete(cmd *cobra.Command, args []string) error {
	if s3DeleteKey == "" {
		return fmt.Errorf("必须指定对象键名 (-k)")
	}

	// 确认删除
	if !s3DeleteForce {
		fmt.Printf("确定要删除 s3://%s/%s 吗？(y/N): ", s3Bucket, s3DeleteKey)
		var confirm string
		fmt.Scanln(&confirm)
		if strings.ToLower(confirm) != "y" {
			fmt.Println("已取消")
			return nil
		}
	}

	client, err := newS3Client(cmd.Context())
	if err != nil {
		return err
	}

	return s3.Delete(cmd.Context(), client, s3DeleteKey, nil)
}

func runS3List(cmd *cobra.Command, args []string) error {
	client, err := newS3Client(cmd.Context())
	if err != nil {
		return err
	}

	opts := &s3.ListOptions{
		Prefix:  s3ListPrefix,
		MaxKeys: int32(s3ListMaxKeys),
	}

	objects, err := s3.List(cmd.Context(), client, opts)
	if err != nil {
		return err
	}

	fmt.Print(s3.FormatOutput(objects, s3ListFormat))
	return nil
}
