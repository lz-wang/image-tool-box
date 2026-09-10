package inspect

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// svgScanLimit 限制 SVG 识别的扫描量：SVG 的根元素必须出现在文件
// 头部（此前只允许 BOM、XML 声明、DOCTYPE 与注释），超过限制即判定
// 非 SVG，避免对大二进制文件做无谓的流式解析。结构校验
//（validateSVG）只对已识别为 SVG 的文本文件执行，不受此限制。
const svgScanLimit = 64 << 10

// newSVGDecoder 返回处理过 UTF-8 BOM 的 XML decoder。
func newSVGDecoder(r io.Reader) *xml.Decoder {
	buffered := bufio.NewReader(r)

	// encoding/xml 不处理 UTF-8 BOM，需要显式剥除
	if head, err := buffered.Peek(3); err == nil && bytes.Equal(head, []byte("\xef\xbb\xbf")) {
		_, _ = buffered.Discard(3)
	}
	return xml.NewDecoder(buffered)
}

// svgRootLooksLikeSVG 在 root 前置内容约束下确认文档第一个 element
// 是 <svg>（任意 namespace，含无 namespace）。返回 (isSVG, reachedRoot)：
// reachedRoot 表示在扫描上限内见到了第一个 element。
func svgRootLooksLikeSVG(decoder *xml.Decoder) (isSVG, reachedRoot bool) {
	for {
		token, err := decoder.Token()
		if err != nil {
			// 语法错误 / EOF / 超出扫描上限：都不是合法 SVG 根
			return false, false
		}
		switch t := token.(type) {
		case xml.ProcInst, xml.Comment, xml.Directive:
			// 声明 / DOCTYPE / 注释：继续
		case xml.CharData:
			// 根元素前只允许空白文本
			if strings.TrimSpace(string(t)) != "" {
				return false, false
			}
		case xml.StartElement:
			return t.Name.Local == "svg", true
		case xml.EndElement:
			return false, false
		}
	}
}

// sniffSVG 流式识别 SVG：找到文档第一个 element 并要求它是 <svg>
//（任意 namespace，含无 namespace）。允许根元素前出现：
//
//   - UTF-8 BOM
//   - XML 声明（<?xml ...?>）
//   - DOCTYPE（<!DOCTYPE ...>）
//   - 注释（<!-- ... -->）
//   - 空白
//
// HTML 改名为 .svg（根元素是 <html>）、非 XML 内容、被截断的 XML
// 一律返回 false。识别失败与"不是 SVG"不做区分——调用方只关心
// 内容能否被认定为 SVG。文档结构是否完整由 validateSVG 独立判定
//（recognition → structure validation 两阶段分离）。
func sniffSVG(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	decoder := newSVGDecoder(io.LimitReader(f, svgScanLimit))
	isSVG, reachedRoot := svgRootLooksLikeSVG(decoder)
	return reachedRoot && isSVG
}

// validateSVG 结构校验：整个文档必须是语法完整的 XML（解析到 EOF），
// 根元素必须是 <svg>，根元素之外不得有其余内容。只应 sniffSVG 命中后
// 调用；任何截断（如 "<svg><g>"）、非法嵌套或根元素后追加内容都返回
// 错误——"内容识别成功但结构损坏"必须显式暴露，而不是静默通过。
func validateSVG(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	decoder := newSVGDecoder(f)
	seenRoot := false
	rootClosed := false
	depth := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch t := token.(type) {
		case xml.StartElement:
			if rootClosed {
				// encoding/xml 是宽松 tokenizer，不强制单根文档：
				// 根元素闭合后再出现元素必须显式拒绝
				return errors.New("content after the root element")
			}
			if !seenRoot {
				seenRoot = true
				if t.Name.Local != "svg" {
					return fmt.Errorf("root element is %q, want \"svg\"", t.Name.Local)
				}
			}
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				rootClosed = true
			}
		case xml.CharData:
			// 根元素闭合后只允许尾部空白
			if rootClosed && strings.TrimSpace(string(t)) != "" {
				return errors.New("content after the root element")
			}
		}
	}
	if !seenRoot {
		return errors.New("document has no root element")
	}
	if depth != 0 {
		return errors.New("unbalanced element structure")
	}
	return nil
}
