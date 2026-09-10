package inspect

import (
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// File 检查图片文件，输出分三个阶段：
//
//  1. content recognition —— 从文件 magic / SVG 流式解析识别格式
//  2. structure/config validation —— image.DecodeConfig 校验结构（SVG 跳过）
//  3. optional full decode —— --full-decode 完整解码（SVG 不支持）
//
// 识别不再依赖 DecodeConfig 是否成功："内容能识别但结构损坏"与
// "内容完全无法识别"是不同的结论，SVG 这类不支持 raster 解码的格式
// 也不会被误报为损坏。
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

	// 阶段 1：内容识别（magic 嗅探 + SVG 流式解析）
	result.Content = recognizeContent(header, path)

	// 阶段 2：结构/配置校验。已识别为不支持 raster 解码的格式（SVG）
	// 跳过 DecodeConfig，改以 XML 结构校验代替——识别成功但文档损坏
	//（如截断的 <svg><g>）必须显式暴露，strict 模式直接失败；未识别
	// 内容仍尝试 DecodeConfig，其失败保留 v2 的 error 结论。
	if result.Content.Recognized && !result.Content.DecodeSupported {
		if result.Content.Format == "svg" {
			if err := validateSVG(path); err != nil {
				if opts.Strict {
					return nil, fmt.Errorf("SVG 结构校验失败: %w", err)
				}
				result.Error = &InfoError{
					Code:    "structure_invalid",
					Message: err.Error(),
				}
			}
		}
	} else {
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
			// WebP 动画信息来自 VP8X 头嗅探，无需完整解码即可断言
			if imgInfo.Format == "webp" {
				animated, known := webpAnimation(header)
				imgInfo.Animated = animated
				imgInfo.AnimationKnown = known
			}
		}
	}

	// 阶段 3：可选完整解码：捕获"header 正常但文件后半部分损坏"，
	// GIF 额外解析帧数与动画状态。已识别但不支持完整解码的格式
	//（SVG）记录 warning，不报错误。
	if opts.FullDecode {
		if !result.Content.FullDecodeSupported {
			if result.Content.Recognized {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("full decode not supported for %s", result.Content.Format))
			}
		} else if result.Image != nil {
			if err := applyFullDecode(path, result.Image); err != nil {
				if opts.Strict {
					return nil, fmt.Errorf("完整解码失败: %w", err)
				}
				result.Warnings = append(result.Warnings, fmt.Sprintf("full decode failed: %v", err))
			}
		}
	}

	if !opts.NoHash {
		hashes, err := computeHashes(path, opts.Hashes)
		if err != nil {
			return nil, err
		}
		result.Hashes = hashes
	}

	if opts.Detail {
		detail := &DetailInfo{
			MagicBytes:  firstHex(header, 10),
			HeaderBytes: firstHex(header, 32),
		}

		// DetectedBy 报告真实检测来源：magic 嗅探、SVG 流式解析，
		// 或兜底的 image.DecodeConfig（内容未识别但 DecodeConfig 成功）
		switch {
		case result.Content.Recognized && result.Content.Format == "svg":
			detail.DetectedBy = "svg.parse"
		case result.Content.Recognized:
			detail.DetectedBy = "magic"
		case result.Image != nil:
			detail.DetectedBy = "image.DecodeConfig"
		}
		detail.ExtensionMatchesFormat = result.Content.Recognized && result.Content.ExtensionMatches

		result.Detail = detail
	}

	return result, nil
}

// recognizeContent 执行阶段 1：先 magic 嗅探光栅格式，未命中时尝试
// SVG 流式识别。识别结论全部来自 formatRegistry。
func recognizeContent(header []byte, path string) ContentInfo {
	content := ContentInfo{}

	if spec := magicSniff(header); spec != nil {
		content.Format = spec.Name
		content.CanonicalExtension = spec.CanonicalExtension
		content.MIMEType = spec.MIMEType
		content.Recognized = true
		content.DecodeSupported = spec.DecodeSupported
		content.FullDecodeSupported = spec.FullDecodeSupported
		content.ExtensionMatches = spec.ExtensionMatches(filepath.Ext(path))
		return content
	}

	if sniffSVG(path) {
		spec, _ := LookupFormat("svg")
		content.Format = spec.Name
		content.CanonicalExtension = spec.CanonicalExtension
		content.MIMEType = spec.MIMEType
		content.Recognized = true
		content.DecodeSupported = spec.DecodeSupported
		content.FullDecodeSupported = spec.FullDecodeSupported
		content.ExtensionMatches = spec.ExtensionMatches(filepath.Ext(path))
	}
	return content
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

	info := &ImageInfo{
		Format:         format,
		Width:          cfg.Width,
		Height:         cfg.Height,
		AspectRatio:    aspectRatio(cfg.Width, cfg.Height),
		Megapixels:     float64(cfg.Width*cfg.Height) / 1_000_000,
		ColorModel:     fmt.Sprintf("%T", cfg.ColorModel),
		HasAlpha:       hasAlpha(cfg.ColorModel),
		DecodeConfigOK: true,
		// 静态光栅格式 header 阶段即可断言非动画；GIF 需要完整解码
		// 数帧，WebP 由 VP8X 嗅探判断
		AnimationKnown: staticAnimationFormats[format],
	}
	return info, nil
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
