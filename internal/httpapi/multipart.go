package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"imagetoolbox/internal/imageio"
)

// uploadedFile 区分服务端存储路径与客户端提供的原始文件名：
// Path 是 CreateTemp 生成的随机路径，OriginalName 仅用于响应下载名等元数据。
type uploadedFile struct {
	Path         string
	OriginalName string
}

type form struct {
	values map[string]string
	files  map[string]uploadedFile
}

const (
	// defaultFieldLimit 限制普通标量字段大小。
	defaultFieldLimit int64 = 4 << 10
	// textFieldLimit 限制水印文字字段大小（更大的文本上限）。
	textFieldLimit int64 = 16 << 10
)

// fieldLimit 返回单个 multipart 标量字段的字节上限。
func fieldLimit(name string) int64 {
	if name == "text" {
		return textFieldLimit
	}
	return defaultFieldLimit
}

func parseMultipart(w http.ResponseWriter, r *http.Request, dir string, cfg Config) (form, error) {
	r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxUpload)
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		return form{}, fmt.Errorf("Content-Type must be multipart/form-data")
	}
	reader, err := r.MultipartReader()
	if err != nil {
		return form{}, err
	}
	f := form{values: map[string]string{}, files: map[string]uploadedFile{}}
	// 同名字段不论标量还是文件都视为重复，统一在 parser 阶段拒绝。
	seen := map[string]struct{}{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return form{}, err
		}
		name := part.FormName()
		if name == "" {
			return form{}, fmt.Errorf("multipart field name is required")
		}
		if _, exists := seen[name]; exists {
			return form{}, fmt.Errorf("duplicate parameter: %s", name)
		}
		seen[name] = struct{}{}
		if part.FileName() == "" {
			limit := fieldLimit(name)
			data, err := io.ReadAll(io.LimitReader(part, limit+1))
			if err != nil {
				return form{}, err
			}
			if int64(len(data)) > limit {
				return form{}, fmt.Errorf("%w: field %s exceeds %d bytes", ErrPayloadTooLarge, name, limit)
			}
			f.values[name] = string(data)
			continue
		}
		// 客户端 filename 只作为 OriginalName 元数据，绝不参与服务端
		// 存储路径：路径由 CreateTemp 生成，避免文件名碰撞与路径注入。
		original := sanitizeFilename(part.FileName())
		if original == "" {
			original = name + ".bin"
		}
		tmp, err := os.CreateTemp(dir, tempPrefix(name)+"-*"+filepath.Ext(original))
		if err != nil {
			return form{}, &imageio.OutputError{Err: err}
		}
		path := tmp.Name()
		_, copyErr := io.Copy(tmp, part)
		closeErr := tmp.Close()
		if copyErr != nil {
			var pathErr *os.PathError
			if errors.As(copyErr, &pathErr) {
				return form{}, &imageio.OutputError{Err: copyErr}
			}
			return form{}, copyErr
		}
		if closeErr != nil {
			return form{}, &imageio.OutputError{Err: closeErr}
		}
		f.files[name] = uploadedFile{Path: path, OriginalName: original}
	}
	return f, nil
}

// tempPrefix 把任意 multipart 字段名收敛为 CreateTemp 可用的安全前缀。
func tempPrefix(name string) string {
	safe := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			safe = append(safe, r)
		}
	}
	if len(safe) == 0 {
		return "upload"
	}
	return string(safe)
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	return strings.TrimLeft(filepath.Base(name), ".")
}

// input requires an uploaded input file and accepts only scalar option fields.
func (f form) input(scalarFields ...string) (uploadedFile, error) {
	return f.inputWithFiles(nil, scalarFields...)
}

// inputWithFiles explicitly declares auxiliary uploads, such as watermark image
// and font. Every other allowed field must be scalar, even when left empty.
func (f form) inputWithFiles(fileFields []string, scalarFields ...string) (uploadedFile, error) {
	if err := f.allowed(append([]string{"input"}, fileFields...), scalarFields...); err != nil {
		return uploadedFile{}, err
	}
	file := f.files["input"]
	if file.Path == "" {
		return uploadedFile{}, errMissingInput
	}
	return file, nil
}

// file 返回指定字段的存储路径；字段未上传时返回空字符串。
func (f form) file(name string) string {
	return f.files[name].Path
}

// allowed validates both names and transport types. A map entry is true for a
// file field, false for a scalar; absent entries are unknown parameters.
func (f form) allowed(fileFields []string, scalarFields ...string) error {
	fields := map[string]bool{}
	for _, name := range scalarFields {
		fields[name] = false
	}
	for _, name := range fileFields {
		fields[name] = true
	}
	for name := range f.values {
		isFile, ok := fields[name]
		if !ok {
			return fmt.Errorf("unknown parameter: %s", name)
		}
		if isFile {
			return fmt.Errorf("parameter %s must be a file", name)
		}
	}
	for name := range f.files {
		isFile, ok := fields[name]
		if !ok {
			return fmt.Errorf("unknown parameter: %s", name)
		}
		if !isFile {
			return fmt.Errorf("parameter %s must be a scalar", name)
		}
	}
	return nil
}
func (f form) int(name string) (int, error) {
	if f.values[name] == "" {
		return 0, nil
	}
	return strconv.Atoi(f.values[name])
}
func (f form) float(name string) (*float64, error) {
	if f.values[name] == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(f.values[name], 64)
	return &v, err
}
func (f form) bool(name string) (bool, error) {
	if f.values[name] == "" {
		return false, nil
	}
	return strconv.ParseBool(f.values[name])
}
