package imageio

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepteams/webp"
)

// ErrUnsupportedFormat 表示格式不在受支持的编码集合（JPEG/PNG/WEBP）内。
var ErrUnsupportedFormat = errors.New("unsupported image format")

type Format string

const (
	FormatJPEG Format = "jpeg"
	FormatPNG  Format = "png"
	FormatWEBP Format = "webp"
)

// SaveOptions 控制编码行为。是否铺底由目标格式唯一决定（JPEG 铺底、
// PNG/WebP 保留 Alpha），调用方不能干预。
type SaveOptions struct {
	Quality    int
	Lossless   bool
	Background color.NRGBA
}

// Info is lightweight image metadata decoded without loading pixel data.
type Info struct {
	// Format 是 image decoder 识别出的原始格式名（jpeg/png/gif/webp/...）。
	// Probe 只报告识别结果；格式能否被编码属于 Format/NormalizeFormat 的
	// 职责，两个概念不应绑定。
	Format string

	// PhysicalWidth / PhysicalHeight 是存储在文件头里的物理尺寸，
	// 未应用 EXIF Orientation。
	PhysicalWidth  int
	PhysicalHeight int

	// Width / Height 是应用 EXIF Orientation 后的逻辑尺寸，
	// 与 OpenStatic 解码后 image.Bounds() 一致。资源准入、
	// resize/watermark/crop 的计划推导必须使用逻辑尺寸，
	// 这样"Probe → Resolve → decode"三者才拥有同一个 invariant。
	Width  int
	Height int

	// Orientation 是 JPEG EXIF Orientation（1-8）；非 JPEG 或缺失时为 1。
	// 5/6/7/8 表示 90°/270° 旋转族，逻辑宽高与物理宽高互换。
	Orientation int
}

// Probe decodes image configuration for resource admission checks. It only
// reports what the image decoder recognizes; it does not normalize the format
// against the supported encode set. Width/Height carry the post-orientation
// (logical) dimensions, matching what OpenStatic decodes.
func Probe(path string) (Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, err
	}
	defer f.Close()
	config, format, err := image.DecodeConfig(f)
	if err != nil {
		if errors.Is(err, image.ErrFormat) {
			return Info{}, fmt.Errorf("%w: 无法识别的图片数据", ErrUnsupportedFormat)
		}
		return Info{}, err
	}

	info := Info{
		Format:         format,
		PhysicalWidth:  config.Width,
		PhysicalHeight: config.Height,
		Width:          config.Width,
		Height:         config.Height,
		Orientation:    1,
	}
	// orientation 只解析 JPEG EXIF（WebP 携带的 orientation 元数据不
	// 处理）；Probe 与 OpenStatic 共用 fileJPEGOrientation，保证两侧
	// 读到的值一致
	if format == "jpeg" {
		if o := fileJPEGOrientation(path); o != 0 {
			info.Orientation = o
		}
	}
	if swapsDimensions(info.Orientation) {
		info.Width, info.Height = info.PhysicalHeight, info.PhysicalWidth
	}
	return info, nil
}

// SuffixedPath returns a path in the input directory with suffix before its extension.
func SuffixedPath(inputPath, suffix string) string {
	ext := filepath.Ext(inputPath)
	base := strings.TrimSuffix(filepath.Base(inputPath), ext)
	return filepath.Join(filepath.Dir(inputPath), base+suffix+ext)
}

// SuffixedName returns an output filename with suffix and ext. An empty ext retains input extension.
func SuffixedName(inputName, suffix, ext string) string {
	if ext == "" {
		ext = filepath.Ext(inputName)
	}
	base := strings.TrimSuffix(filepath.Base(inputName), filepath.Ext(inputName))
	return base + suffix + ext
}

func NormalizeFormat(value string) (Format, error) {
	switch strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), ".")) {
	case "jpg", "jpeg":
		return FormatJPEG, nil
	case "png":
		return FormatPNG, nil
	case "webp":
		return FormatWEBP, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedFormat, value)
	}
}

func FormatFromPath(path string) (Format, error) {
	return NormalizeFormat(filepath.Ext(path))
}

func DetectFormat(path string) (Format, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, format, err := image.DecodeConfig(f)
	if err != nil {
		return "", err
	}

	return NormalizeFormat(format)
}

func Save(path string, img image.Image, opts SaveOptions) error {
	format, err := FormatFromPath(path)
	if err != nil {
		return err
	}
	return SaveWithFormat(path, img, format, opts)
}

func SaveWithFormat(path string, img image.Image, format Format, opts SaveOptions) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return &OutputError{Err: err}
	}

	out, err := os.Create(path)
	if err != nil {
		return &OutputError{Err: err}
	}
	defer func() {
		if closeErr := out.Close(); err == nil && closeErr != nil {
			err = &OutputError{Err: closeErr}
		}
	}()

	if err := Encode(out, img, format, opts); err != nil {
		return &OutputError{Err: err}
	}
	return nil
}

func Encode(w io.Writer, img image.Image, format Format, opts SaveOptions) error {
	quality := opts.Quality
	if quality <= 0 {
		quality = 100
	}
	background := opts.Background
	if background == (color.NRGBA{}) {
		background = color.NRGBA{255, 255, 255, 255}
	}

	switch format {
	case FormatJPEG:
		// JPEG 不支持 Alpha，固定铺底。
		return jpeg.Encode(w, Flatten(img, background), &jpeg.Options{Quality: quality})
	case FormatPNG:
		return png.Encode(w, img)
	case FormatWEBP:
		return encodeWEBP(w, img, opts.Lossless, quality)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedFormat, format)
	}
}

// encodeWEBP 基于 libwebp 标准 DefaultOptions 构建 encoder 配置：
// 多个字段使用 -1 哨兵值区分 Go 零值与 C 默认值（SNS/Filter/Alpha 等），
// 直接构造零值 struct 会得到非标准默认参数。
func encodeWEBP(w io.Writer, img image.Image, lossless bool, quality int) error {
	opts := webp.DefaultOptions()
	opts.Lossless = lossless
	opts.Quality = float32(quality)
	opts.Method = 4
	if lossless {
		// Exact 保留透明像素下的 RGB，保证 lossless 逐像素往返。
		opts.Exact = true
	}
	return webp.Encode(w, img, opts)
}

func SupportsWEBPEncoding() bool {
	return true
}

func Flatten(img image.Image, bg color.NRGBA) image.Image {
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, &image.Uniform{C: bg}, image.Point{}, draw.Src)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Over)
	return rgba
}

func ParseHexColor(s string) (color.NRGBA, error) {
	str := strings.TrimSpace(s)
	if str == "" {
		return color.NRGBA{}, fmt.Errorf("color must not be empty")
	}
	str = strings.TrimPrefix(str, "#")
	switch len(str) {
	case 3:
		str = fmt.Sprintf("%c%c%c%c%c%c", str[0], str[0], str[1], str[1], str[2], str[2])
	case 6, 8:
	default:
		return color.NRGBA{}, fmt.Errorf("invalid color format: %q", s)
	}

	var r, g, b, a uint8
	hexRGB := str
	if len(str) == 8 {
		hexRGB = str[:6]
	}
	if _, err := fmt.Sscanf(hexRGB, "%02x%02x%02x", &r, &g, &b); err != nil {
		return color.NRGBA{}, err
	}
	if len(str) == 8 {
		if _, err := fmt.Sscanf(str[6:], "%02x", &a); err != nil {
			return color.NRGBA{}, err
		}
	} else {
		a = 255
	}

	return color.NRGBA{R: r, G: g, B: b, A: a}, nil
}
