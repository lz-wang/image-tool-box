package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"imagetoolbox/internal/imageio"
)

func TestOutputErrorResponses(t *testing.T) {
	input := formFile{field: "input", filename: "photo.png", content: testPNG(t, 8, 8)}
	t.Run("grayscale commit failure is server error", func(t *testing.T) {
		cfg := Config{NoAuth: true}
		cfg.Normalize()
		h := imageHandler(cfg, "grayscale", func(ctx context.Context, f form, dir string, cfg Config) (string, string, int64, error) {
			if err := os.Mkdir(filepath.Join(dir, "result.png"), 0o700); err != nil {
				return "", "", 0, err
			}
			return grayscaleImage(ctx, f, dir, cfg)
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, newMultipartRequest(t, http.MethodPost, "/api/v1/grayscale", nil, input))
		assertInternalOutputError(t, w)
	})
	t.Run("upload storage failure is server error", func(t *testing.T) {
		cfg := Config{}
		cfg.Normalize()
		w := httptest.NewRecorder()
		// Upload transport is valid; the server's destination is unavailable.
		_, err := parseMultipart(w, newMultipartRequest(t, http.MethodPost, "/api/v1/grayscale", nil, input), filepath.Join(t.TempDir(), "unavailable"), cfg)
		var outputErr *imageio.OutputError
		if !errors.As(err, &outputErr) {
			t.Fatalf("missing output type: %v", err)
		}
		writeError(w, multipartErrorStatus(err), err)
		assertInternalOutputError(t, w)
	})
	t.Run("input media stays 415", func(t *testing.T) {
		w := httptest.NewRecorder()
		mustNew(t, Config{NoAuth: true}).ServeHTTP(w, newMultipartRequest(t, http.MethodPost, "/api/v1/grayscale", nil, formFile{field: "input", filename: "photo.gif", content: testGIF(t, 8, 8)}))
		if w.Code != 415 || decodeJSONError(t, w.Body.Bytes()) != "unsupported_format" {
			t.Fatal(w.Code, w.Body.String())
		}
	})
	t.Run("output failure has precedence over underlying format error", func(t *testing.T) {
		err := fmt.Errorf("wrapped: %w", &imageio.OutputError{Err: imageio.ErrUnsupportedFormat})
		if got := operationErrorStatus(err); got != 500 {
			t.Fatalf("status %d", got)
		}
		w := httptest.NewRecorder()
		writeError(w, operationErrorStatus(err), err)
		assertInternalOutputError(t, w)
	})
}

func assertInternalOutputError(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != 500 || decodeJSONError(t, w.Body.Bytes()) != "internal_error" {
		t.Fatal(w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "internal server error") || strings.Contains(w.Body.String(), "result.png") || strings.Contains(w.Body.String(), "unavailable") {
		t.Fatal("internal details exposed:", w.Body.String())
	}
}
