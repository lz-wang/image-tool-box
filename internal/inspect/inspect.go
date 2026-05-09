package inspect

import (
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/deepteams/webp"
)

func File(path string, opts Options) (*Result, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件信息失败: %w", err)
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("输入路径是目录，不是图片文件: %s", path)
	}

	absPath, _ := filepath.Abs(path)

	header, err := readHeader(path, 512)
	if err != nil {
		return nil, err
	}

	result := &Result{
		SchemaVersion: SchemaVersion,
		File: FileInfo{
			Path:       path,
			AbsPath:    absPath,
			Name:       filepath.Base(path),
			Ext:        strings.ToLower(filepath.Ext(path)),
			SizeBytes:  stat.Size(),
			Mode:       stat.Mode().String(),
			ModifiedAt: stat.ModTime(),
			MIMEType:   http.DetectContentType(header),
			MagicHex:   firstHex(header, 4),
		},
		Warnings: []string{},
	}

	if !opts.NoHash {
		hashes, err := ComputeAllHashes(path)
		if err != nil {
			return nil, err
		}
		result.Hashes = hashes
	}

	imgInfo, decodeErr := decodeImageConfig(path)
	if decodeErr != nil {
		if opts.Strict {
			return nil, fmt.Errorf("解析图片元数据失败: %w", decodeErr)
		}

		result.Error = &InfoError{
			Code:    "decode_config_failed",
			Message: decodeErr.Error(),
		}
	} else {
		result.Image = imgInfo
	}

	if opts.Detail {
		detail := &DetailInfo{
			MagicBytes:  firstHex(header, 10),
			HeaderBytes: firstHex(header, 32),
			DetectedBy:  "image.DecodeConfig",
		}

		if result.Image != nil {
			detail.ExtensionMatchesFormat = extensionMatchesFormat(result.File.Ext, result.Image.Format)
		}

		result.Detail = detail
	}

	return result, nil
}

func decodeImageConfig(path string) (*ImageInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开图片失败: %w", err)
	}
	defer f.Close()

	cfg, format, err := image.DecodeConfig(f)
	if err != nil {
		return nil, err
	}

	return &ImageInfo{
		Format:         format,
		Width:          cfg.Width,
		Height:         cfg.Height,
		AspectRatio:    aspectRatio(cfg.Width, cfg.Height),
		Megapixels:     float64(cfg.Width*cfg.Height) / 1_000_000,
		ColorModel:     fmt.Sprintf("%T", cfg.ColorModel),
		HasAlpha:       hasAlpha(cfg.ColorModel),
		Animated:       false,
		DecodeConfigOK: true,
	}, nil
}

func readHeader(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	buf := make([]byte, n)
	readN, err := f.Read(buf)
	if err != nil && readN == 0 {
		return nil, fmt.Errorf("读取文件头失败: %w", err)
	}

	return buf[:readN], nil
}

func firstHex(data []byte, n int) string {
	if len(data) == 0 {
		return ""
	}
	if len(data) < n {
		n = len(data)
	}
	return hex.EncodeToString(data[:n])
}

func aspectRatio(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	g := gcd(width, height)
	return fmt.Sprintf("%d:%d", width/g, height/g)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func hasAlpha(model color.Model) bool {
	switch model {
	case color.AlphaModel,
		color.Alpha16Model,
		color.NRGBAModel,
		color.NRGBA64Model,
		color.RGBAModel,
		color.RGBA64Model:
		return true
	default:
		return false
	}
}

func extensionMatchesFormat(ext string, format string) bool {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	format = strings.ToLower(format)

	switch ext {
	case "jpg", "jpeg":
		return format == "jpeg"
	case "png":
		return format == "png"
	case "gif":
		return format == "gif"
	case "webp":
		return format == "webp"
	default:
		return false
	}
}
