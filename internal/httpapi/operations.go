package httpapi

import (
	"context"
	"fmt"
	"image"
	"net/http"
	"os"
	"path/filepath"

	"imagetoolbox/internal/compress"
	"imagetoolbox/internal/convert"
	"imagetoolbox/internal/crop"
	"imagetoolbox/internal/imageio"
	"imagetoolbox/internal/inspect"
	"imagetoolbox/internal/resize"
	"imagetoolbox/internal/rotate"
	"imagetoolbox/internal/watermark"
)

type operation func(context.Context, form, string, Config) (string, string, int64, error)

// imageHandler 是图片变换类端点的统一请求生命周期：
// 临时目录隔离 -> multipart 解析 -> 输入准入 -> 超时检查 -> 领域操作 ->
// 超时检查 -> 流式响应。
func imageHandler(cfg Config, operationName string, op operation) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dir, err := os.MkdirTemp("", "itb-api-*")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		defer os.RemoveAll(dir)
		f, err := parseMultipart(w, r, dir, cfg)
		if err != nil {
			writeError(w, multipartErrorStatus(err), err)
			return
		}
		if err := admitImage(f.files["input"].Path, cfg); err != nil {
			writeError(w, operationErrorStatus(err), err)
			return
		}
		if err := r.Context().Err(); err != nil {
			writeError(w, http.StatusGatewayTimeout, err)
			return
		}
		path, name, inputSize, err := op(r.Context(), f, dir, cfg)
		if err != nil {
			writeError(w, operationErrorStatus(err), err)
			return
		}
		if err := r.Context().Err(); err != nil {
			writeError(w, http.StatusGatewayTimeout, err)
			return
		}
		serveFile(w, r, path, name, inputSize, operationName)
	}
}

func admitImage(path string, cfg Config) error {
	if path == "" {
		return nil
	}
	info, err := imageio.Probe(path)
	if err != nil {
		return err
	}
	return validateImageSize(info.Width, info.Height, cfg)
}

// validateImageSize 是统一的图片尺寸准入检查：既用于上传的输入/辅助
// 图片，也用于操作计划推导出的目标输出尺寸。像素数使用 int64 计算，
// 避免 int 溢出。
func validateImageSize(width, height int, cfg Config) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("invalid image dimensions: %dx%d", width, height)
	}
	if width > cfg.MaxDimension || height > cfg.MaxDimension {
		return fmt.Errorf("%w: %dx%d exceeds max dimension %d", ErrImageTooLarge, width, height, cfg.MaxDimension)
	}
	if pixels := int64(width) * int64(height); pixels > cfg.MaxPixels {
		return fmt.Errorf("%w: %dx%d exceeds max pixels %d", ErrImageTooLarge, width, height, cfg.MaxPixels)
	}
	return nil
}

func compressImage(ctx context.Context, f form, dir string, _ Config) (string, string, int64, error) {
	input, err := f.input("quality")
	if err != nil {
		return "", "", 0, err
	}
	quality, err := f.int("quality")
	if err != nil {
		return "", "", 0, err
	}
	output := resultPath(dir, input.Path)
	result, err := compress.CompressFile(ctx, input.Path, output, compress.FileOptions{Quality: quality})
	return output, input.OriginalName, result.InputSize, err
}
func resizeImage(_ context.Context, f form, dir string, cfg Config) (string, string, int64, error) {
	input, err := f.input("width", "height", "percent", "mode", "anchor", "filter")
	if err != nil {
		return "", "", 0, err
	}
	width, err := f.int("width")
	if err != nil {
		return "", "", 0, err
	}
	height, err := f.int("height")
	if err != nil {
		return "", "", 0, err
	}
	opts := resize.Options{Width: width, Height: height, Percent: f.values["percent"], Mode: resize.Mode(f.values["mode"]), Anchor: f.values["anchor"], Filter: f.values["filter"]}
	// 先用领域 Resolve 推导真实输出尺寸（含 percent/fit/fill 语义），
	// 再对计划输出做资源准入，杜绝通过参数放大绕过限制。
	info, err := imageio.Probe(input.Path)
	if err != nil {
		return "", "", 0, err
	}
	plan, err := resize.Resolve(image.Rect(0, 0, info.Width, info.Height), opts)
	if err != nil {
		return "", "", 0, err
	}
	if err := validateImageSize(plan.Width, plan.Height, cfg); err != nil {
		return "", "", 0, fmt.Errorf("resize target: %w", err)
	}
	name := imageio.SuffixedName(input.OriginalName, "_resized", "")
	output := resultPath(dir, input.Path)
	err = resize.ResizeFile(input.Path, output, opts)
	return output, name, fileSize(input.Path), err
}
func cropImage(_ context.Context, f form, dir string, _ Config) (string, string, int64, error) {
	input, err := f.input("anchor", "width", "height")
	if err != nil {
		return "", "", 0, err
	}
	name := imageio.SuffixedName(input.OriginalName, "_cropped", "")
	output := resultPath(dir, input.Path)
	_, err = crop.CropFile(input.Path, output, crop.Options{Anchor: crop.Anchor(f.values["anchor"]), Width: f.values["width"], Height: f.values["height"]})
	return output, name, fileSize(input.Path), err
}
func rotateImage(_ context.Context, f form, dir string, cfg Config) (string, string, int64, error) {
	input, err := f.input("angle")
	if err != nil {
		return "", "", 0, err
	}
	angle, err := f.float("angle")
	if err != nil {
		return "", "", 0, err
	}
	if angle == nil {
		return "", "", 0, fmt.Errorf("angle is required")
	}
	opts := rotate.Options{Angle: *angle}
	// 先用领域 Resolve 推导旋转后的输出尺寸（任意角度可能扩大画布），
	// 再对计划输出做资源准入，杜绝先分配画布再发现超限。
	info, err := imageio.Probe(input.Path)
	if err != nil {
		return "", "", 0, err
	}
	plan, err := rotate.Resolve(image.Rect(0, 0, info.Width, info.Height), opts)
	if err != nil {
		return "", "", 0, err
	}
	if err := validateImageSize(plan.Width, plan.Height, cfg); err != nil {
		return "", "", 0, fmt.Errorf("rotate target: %w", err)
	}
	// working-set 准入：任意角度会同时驻留输入 NRGBA 副本与输出画布，
	// 与 watermark 一致地在真实分配之前按保守上界拒绝。
	if plan.WorkingBytes > cfg.MaxWorkingBytes {
		return "", "", 0, fmt.Errorf("%w: rotate working set exceeds %d bytes", ErrImageTooLarge, cfg.MaxWorkingBytes)
	}
	name := imageio.SuffixedName(input.OriginalName, "_rotated", "")
	output := resultPath(dir, input.Path)
	err = rotate.RotateFile(input.Path, output, opts)
	return output, name, fileSize(input.Path), err
}
func convertImage(_ context.Context, f form, dir string, _ Config) (string, string, int64, error) {
	input, err := f.input("to", "quality", "lossless", "background")
	if err != nil {
		return "", "", 0, err
	}
	quality, err := f.int("quality")
	if err != nil {
		return "", "", 0, err
	}
	lossless, err := f.bool("lossless")
	if err != nil {
		return "", "", 0, err
	}
	format, err := imageio.NormalizeFormat(f.values["to"])
	if err != nil {
		return "", "", 0, err
	}
	ext := "." + string(format)
	name := imageio.SuffixedName(input.OriginalName, "_converted", ext)
	output := filepath.Join(dir, "result"+ext)
	err = convert.ConvertFile(input.Path, output, convert.Options{Quality: quality, Lossless: lossless, Background: f.values["background"]})
	return output, name, fileSize(input.Path), err
}
func watermarkImage(_ context.Context, f form, dir string, cfg Config) (string, string, int64, error) {
	input, err := f.inputWithFiles([]string{"image", "font"}, "text", "mode", "color", "space", "angle", "opacity", "font-size", "position", "margin", "scale")
	if err != nil {
		return "", "", 0, err
	}
	space, err := f.int("space")
	if err != nil {
		return "", "", 0, err
	}
	angle, err := f.int("angle")
	if err != nil {
		return "", "", 0, err
	}
	fontSize, err := f.int("font-size")
	if err != nil {
		return "", "", 0, err
	}
	opacity, err := f.float("opacity")
	if err != nil {
		return "", "", 0, err
	}
	margin, err := f.float("margin")
	if err != nil {
		return "", "", 0, err
	}
	scale, err := f.float("scale")
	if err != nil {
		return "", "", 0, err
	}
	opts := watermark.Options{Text: f.values["text"], ImagePath: f.file("image"), Mode: watermark.Mode(f.values["mode"]), Position: watermark.Position(f.values["position"]), Color: f.values["color"], FontPath: f.file("font"), Opacity: opacity, FontSize: intPtr(f.values["font-size"], fontSize), Space: intPtr(f.values["space"], space), Angle: intPtr(f.values["angle"], angle), Margin: margin, Scale: scale}
	// 领域 Normalize/Validate 提前拦截非法参数，语义与 CLI 完全一致。
	opts.Normalize()
	if err := opts.Validate(); err != nil {
		return "", "", 0, err
	}
	baseInfo, err := imageio.Probe(input.Path)
	if err != nil {
		return "", "", 0, err
	}
	baseBounds := image.Rect(0, 0, baseInfo.Width, baseInfo.Height)
	var logoBounds image.Rectangle
	// 辅助图片（图片水印 logo）与输入图片受同样的尺寸限制；
	// 缩放后的 logo 目标尺寸同样要过 validateImageSize，防止 scale
	// 把小 logo 放大成超出限制的中间图。
	if logoPath := f.file("image"); logoPath != "" {
		logoInfo, err := imageio.Probe(logoPath)
		if err != nil {
			return "", "", 0, fmt.Errorf("watermark image: %w", err)
		}
		if err := validateImageSize(logoInfo.Width, logoInfo.Height, cfg); err != nil {
			return "", "", 0, fmt.Errorf("watermark image: %w", err)
		}
		logoBounds = image.Rect(0, 0, logoInfo.Width, logoInfo.Height)
		plan, err := watermark.ResolveImagePlan(baseBounds, logoBounds, opts)
		if err != nil {
			return "", "", 0, fmt.Errorf("watermark image: %w", err)
		}
		if err := validateImageSize(plan.TargetWidth, plan.TargetHeight, cfg); err != nil {
			return "", "", 0, fmt.Errorf("watermark image: %w", err)
		}
	}
	// working-set 准入：文字 mark 画布、repeat 平铺/旋转画布与缩放 logo
	// 的保守内存上界不能超过服务限制，在真实分配前拒绝。
	set, err := watermark.ResolveWorkingSet(baseBounds, logoBounds, opts)
	if err != nil {
		return "", "", 0, err
	}
	if set.Bytes() > cfg.MaxWorkingBytes {
		return "", "", 0, fmt.Errorf("%w: watermark working set exceeds %d bytes", ErrImageTooLarge, cfg.MaxWorkingBytes)
	}
	name := imageio.SuffixedName(input.OriginalName, "_watermarked", "")
	output := resultPath(dir, input.Path)
	err = watermark.AddFile(input.Path, output, opts)
	return output, name, fileSize(input.Path), err
}

// resultPath 生成操作输出路径：固定 result 前缀 + 输入扩展名。上传文件
// 一律由 CreateTemp 生成带随机后缀的路径，因此 output 永远不会与任何
// 上传路径相同，输入输出互不覆盖。
func resultPath(dir, inputPath string) string {
	return filepath.Join(dir, "result"+filepath.Ext(inputPath))
}
func intPtr(raw string, value int) *int {
	if raw == "" {
		return nil
	}
	return &value
}
func fileSize(path string) int64 {
	stat, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return stat.Size()
}

// inspectHandler 单独成流：inspect 的 Probe 是可选的尺寸检查，
// 失败不提前终止，strict 语义由 inspect.File 决定。
func inspectHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dir, err := os.MkdirTemp("", "itb-api-*")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		defer os.RemoveAll(dir)
		f, err := parseMultipart(w, r, dir, cfg)
		if err != nil {
			writeError(w, multipartErrorStatus(err), err)
			return
		}
		input, err := f.input("detail", "no-detail", "no-hash", "strict", "full-decode")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		// inspect 的 Probe 是可选的尺寸检查：识别成功才执行限制；
		// 识别失败不提前终止，交给 inspect.File 按 strict 语义决定
		// 是返回 metadata+error 还是直接报错。
		if info, err := imageio.Probe(input.Path); err == nil {
			if err := validateImageSize(info.Width, info.Height, cfg); err != nil {
				writeError(w, operationErrorStatus(err), err)
				return
			}
		}
		if err := r.Context().Err(); err != nil {
			writeError(w, http.StatusGatewayTimeout, err)
			return
		}
		detail, err := f.bool("detail")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		noDetail, err := f.bool("no-detail")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		noHash, err := f.bool("no-hash")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		strict, err := f.bool("strict")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		fullDecode, err := f.bool("full-decode")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if !detail {
			detail = true
		}
		if noDetail {
			detail = false
		}
		result, err := inspect.File(input.Path, inspect.Options{Detail: detail, NoHash: noHash, Strict: strict, FullDecode: fullDecode})
		if err != nil {
			// strict=true 时解析失败在此报错；其余失败（路径/读取等）
			// 同样归为客户端输入问题。
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := r.Context().Err(); err != nil {
			writeError(w, http.StatusGatewayTimeout, err)
			return
		}
		// 响应不暴露服务端存储路径，文件名使用客户端原始名。
		result.File.Path = input.OriginalName
		result.File.Name = input.OriginalName
		result.File.AbsPath = ""
		writeJSON(w, http.StatusOK, result)
	}
}
