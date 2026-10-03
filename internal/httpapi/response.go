package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"imagetoolbox/internal/barcode"
	"imagetoolbox/internal/compress"
	"imagetoolbox/internal/imageio"
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, err error) {
	code, message := errorResponse(status, err)
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// errorResponse 把状态码映射为稳定的 error code 与对外消息；
// 内部错误细节不透出给客户端。
func errorResponse(status int, err error) (string, string) {
	switch status {
	case http.StatusBadRequest:
		if errors.Is(err, errMissingInput) {
			return "missing_input", "input is required"
		}
		return "invalid_argument", err.Error()
	case http.StatusUnauthorized:
		return "unauthorized", "valid bearer token is required"
	case http.StatusRequestEntityTooLarge:
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return "payload_too_large", "request exceeds the configured upload limit"
		}
		if errors.Is(err, ErrPayloadTooLarge) {
			return "payload_too_large", err.Error()
		}
		return "image_too_large", "image exceeds configured limits"
	case http.StatusUnsupportedMediaType:
		return "unsupported_format", "unsupported image format"
	case http.StatusNotFound:
		return "not_found", "route not found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed", "method is not allowed for this route"
	case http.StatusTooManyRequests:
		return "busy", "too many image operations in progress"
	case http.StatusGatewayTimeout:
		return "timeout", "image operation timed out"
	default:
		return "internal_error", "internal server error"
	}
}

// operationErrorStatus 统一把领域/准入错误映射为 HTTP 状态码，
// 全部基于 typed errors 判别，不做字符串匹配。
func operationErrorStatus(err error) int {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(err, ErrImageTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, barcode.ErrDecoderFailure), errors.Is(err, barcode.ErrDecoderUnavailable):
		return http.StatusInternalServerError
	case errors.Is(err, imageio.ErrUnsupportedFormat), errors.Is(err, compress.ErrUnsupportedFormat):
		return http.StatusUnsupportedMediaType
	default:
		return http.StatusBadRequest
	}
}

func multipartErrorStatus(err error) int {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return http.StatusRequestEntityTooLarge
	}
	if errors.Is(err, ErrPayloadTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func serveFile(w http.ResponseWriter, r *http.Request, path, name string, inputSize int64, operationName string) {
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		buffer := make([]byte, 512)
		n, readErr := f.Read(buffer)
		if readErr != nil && readErr != io.EOF {
			writeError(w, http.StatusInternalServerError, readErr)
			return
		}
		contentType = http.DetectContentType(buffer[:n])
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", contentDisposition(name))
	w.Header().Set("X-ITB-Input-Size", strconv.FormatInt(inputSize, 10))
	w.Header().Set("X-ITB-Output-Size", strconv.FormatInt(stat.Size(), 10))
	w.Header().Set("X-ITB-Operation", operationName)
	http.ServeContent(w, r, name, stat.ModTime(), f)
}

func contentDisposition(name string) string {
	for _, r := range name {
		if r > 0x7f {
			return fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", "download"+filepath.Ext(name), url.PathEscape(name))
		}
	}
	return fmt.Sprintf("attachment; filename=%q", name)
}
