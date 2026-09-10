package inspect

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestSniffSVGVariants 覆盖合法前置元素与必须拒绝的内容。
func TestSniffSVGVariants(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"裸 svg", `<svg xmlns="http://www.w3.org/2000/svg"></svg>`, true},
		{"带 XML 声明", `<?xml version="1.0" encoding="UTF-8"?><svg/>`, true},
		{"声明+注释", `<?xml version="1.0"?><!-- generated --><svg/>`, true},
		{"DOCTYPE", `<?xml version="1.0"?><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg/>`, true},
		{"多个注释", `<!-- a --><!-- b --><svg width="1"/>`, true},
		{"无 namespace", `<svg></svg>`, true},
		{"属性跨行", "<svg\n  width=\"10\"\n  height=\"10\"\n/>", true},
		{"HTML 改名 .svg", `<!DOCTYPE html><html><body></body></html>`, false},
		{"HTML 无 doctype", `<html><body>hi</body></html>`, false},
		{"根元素前有文本", `oops <svg/>`, false},
		{"截断 XML", `<?xml version="1.0"?><svg`, false},
		{"空文件", ``, false},
		{"二进制垃圾", "\x00\x01\x02\xff\xfe", false},
		{"svg 拼写在注释里", `<!-- <svg/> --><html/>`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTextFile(t, "probe.svg", tt.content)
			if got := sniffSVG(path); got != tt.want {
				t.Errorf("sniffSVG = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSniffSVGBOM UTF-8 BOM 前缀合法。
func TestSniffSVGBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bom.svg")
	if err := os.WriteFile(path, append([]byte("\xef\xbb\xbf"), []byte(`<?xml version="1.0"?><svg/>`)...), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !sniffSVG(path) {
		t.Fatal("BOM-prefixed SVG must be recognized")
	}
}

// TestSniffSVGLargeBinary 前置大二进制内容不得触发无界解析。
func TestSniffSVGLargeBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.bin")
	data := make([]byte, 128<<10) // 128KB 零字节
	for i := range data {
		data[i] = byte(i % 251)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if sniffSVG(path) {
		t.Fatal("large binary must not be recognized as svg")
	}
}

// TestInspectSVGFile 端到端：SVG 识别为合法图片内容但不做 raster 解码。
func TestInspectSVGFile(t *testing.T) {
	svg := `<?xml version="1.0"?><!-- vector --><svg xmlns="http://www.w3.org/2000/svg" width="120" height="60"><rect width="120" height="60"/></svg>`
	path := writeTextFile(t, "vector.svg", svg)

	t.Run("默认检查", func(t *testing.T) {
		result, err := File(path, Options{NoHash: true})
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		if !result.Content.Recognized || result.Content.Format != "svg" {
			t.Fatalf("content = %+v, want recognized svg", result.Content)
		}
		if result.Content.DecodeSupported {
			t.Error("svg decode_supported must be false")
		}
		// 不支持 raster 解码不是"图片损坏"：无 error 对象
		if result.Error != nil {
			t.Errorf("svg must not produce error object: %+v", result.Error)
		}
		// image 对象缺省：SVG 尺寸属于可选信息
		if result.Image != nil {
			t.Errorf("svg must not produce image info: %+v", result.Image)
		}
	})

	t.Run("strict 不因 SVG 失败", func(t *testing.T) {
		if _, err := File(path, Options{NoHash: true, Strict: true}); err != nil {
			t.Fatalf("strict File on valid svg: %v", err)
		}
	})

	t.Run("full-decode 记录 warning 而非报错", func(t *testing.T) {
		result, err := File(path, Options{NoHash: true, FullDecode: true})
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		if len(result.Warnings) == 0 {
			t.Error("expected a warning noting full decode is unsupported")
		}
	})

	t.Run("HTML 改名 .svg 不得被识别", func(t *testing.T) {
		htmlPath := writeTextFile(t, "page.svg", `<!DOCTYPE html><html><body>x</body></html>`)
		result, err := File(htmlPath, Options{NoHash: true})
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		if result.Content.Recognized {
			t.Fatalf("html content must not be recognized as svg: %+v", result.Content)
		}
	})
}

// TestInspectBrokenSVGStructure 内容识别成功但文档结构损坏（截断、
// 非法嵌套、根元素后追加内容）必须显式暴露：默认输出 error 对象，
// --strict 直接失败。
func TestInspectBrokenSVGStructure(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"根元素未闭合", `<svg><g>`},
		{"标签未闭合", `<svg><broken`},
		{"根元素后追加内容", `<svg/><oops/>`},
		{"根元素后追加文本", `<svg/>trailing`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTextFile(t, "broken.svg", tt.content)

			result, err := File(path, Options{NoHash: true})
			if err != nil {
				t.Fatalf("File: %v", err)
			}
			// 内容识别仍成立（recognition 与结构校验分离）
			if !result.Content.Recognized || result.Content.Format != "svg" {
				t.Fatalf("content = %+v, want recognized svg", result.Content)
			}
			if result.Error == nil || result.Error.Code != "structure_invalid" {
				t.Fatalf("error = %+v, want structure_invalid", result.Error)
			}

			if _, err := File(path, Options{NoHash: true, Strict: true}); err == nil {
				t.Error("strict mode must fail for broken svg")
			}
		})
	}
}

// TestInspectValidSVGHasNoStructureError 完整合法 SVG（自闭合、嵌套
// 完整、根后空白）不产生结构错误。
func TestInspectValidSVGHasNoStructureError(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"自闭合", `<svg/>`},
		{"嵌套完整", `<svg><g><rect/></g></svg>`},
		{"根后空白", "<svg/>\n"},
		{"声明+注释+根", `<?xml version="1.0"?><!-- x --><svg xmlns="http://www.w3.org/2000/svg"/>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTextFile(t, "ok.svg", tt.content)
			result, err := File(path, Options{NoHash: true})
			if err != nil {
				t.Fatalf("File: %v", err)
			}
			if !result.Content.Recognized {
				t.Fatalf("content = %+v, want recognized", result.Content)
			}
			if result.Error != nil {
				t.Errorf("error = %+v, want none", result.Error)
			}
		})
	}
}

// TestValidateSVGDetectsByDetail --detail 的 detected_by 反映真实
// 检测来源：SVG 为 svg.parse，magic 格式为 magic，兜底为
// image.DecodeConfig。
func TestValidateSVGDetectsByDetail(t *testing.T) {
	svgPath := writeTextFile(t, "vector.svg", `<svg><rect/></svg>`)
	result, err := File(svgPath, Options{NoHash: true, Detail: true})
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if result.Detail == nil || result.Detail.DetectedBy != "svg.parse" {
		t.Fatalf("detail = %+v, want svg.parse", result.Detail)
	}

	pngPath := filepath.Join(t.TempDir(), "img.png")
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	f, err := os.Create(pngPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	result, err = File(pngPath, Options{NoHash: true, Detail: true})
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if result.Detail == nil || result.Detail.DetectedBy != "magic" {
		t.Fatalf("detail = %+v, want magic", result.Detail)
	}
}
