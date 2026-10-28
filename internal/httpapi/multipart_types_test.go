package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestMultipartOptionFilesRejected(t *testing.T) {
	h := mustNew(t, Config{NoAuth: true})
	input := formFile{field: "input", filename: "photo.png", content: testPNG(t, 16, 16)}
	// Every endpoint declares its file fields; every other option is scalar.
	for _, tc := range []struct {
		endpoint string
		fields   []string
		noInput  bool
	}{
		{"compress", []string{"quality"}, false},
		{"resize", []string{"width", "height", "percent", "mode", "anchor", "filter"}, false},
		{"crop", []string{"anchor", "width", "height"}, false},
		{"rotate", []string{"angle"}, false},
		{"convert", []string{"to", "quality", "lossless", "background"}, false},
		{"grayscale", []string{"levels", "dither", "background", "to", "quality", "lossless"}, false},
		{"watermark", []string{"text", "mode", "color", "space", "angle", "opacity", "font-size", "position", "margin", "scale"}, false},
		{"inspect", []string{"detail", "no-detail", "no-hash", "strict", "full-decode"}, false},
		{"barcode/generate", []string{"symbology", "data", "error-correction", "module-size", "border", "no-draw-text", "module-width", "module-height"}, true},
		{"barcode/decode", []string{"symbology"}, false},
	} {
		for _, field := range tc.fields {
			t.Run(tc.endpoint+"/"+field, func(t *testing.T) {
				files := []formFile{}
				if !tc.noInput {
					files = append(files, input)
				}
				files = append(files, formFile{field: field, filename: "option.txt", content: []byte("1")})
				w := httptest.NewRecorder()
				h.ServeHTTP(w, newMultipartRequest(t, http.MethodPost, "/api/v1/"+tc.endpoint, nil, files...))
				assertMultipartTypeError(t, w, "parameter "+field+" must be a scalar")
			})
		}
	}
}

func TestMultipartScalarInputsRejected(t *testing.T) {
	h := mustNew(t, Config{NoAuth: true})
	for _, endpoint := range []string{"compress", "resize", "crop", "rotate", "convert", "grayscale", "watermark", "inspect", "barcode/decode"} {
		t.Run(endpoint, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, newMultipartRequest(t, http.MethodPost, "/api/v1/"+endpoint, map[string]string{"input": "photo.png"}))
			assertMultipartTypeError(t, w, "parameter input must be a file")
		})
	}
}

func TestMultipartWatermarkAuxiliaryFiles(t *testing.T) {
	h := mustNew(t, Config{NoAuth: true})
	input := formFile{field: "input", filename: "photo.png", content: testPNG(t, 32, 16)}
	for _, field := range []string{"image", "font"} {
		t.Run(field+" rejects scalar", func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, newMultipartRequest(t, http.MethodPost, "/api/v1/watermark", map[string]string{"text": "hello", field: "local-path"}, input))
			assertMultipartTypeError(t, w, "parameter "+field+" must be a file")
		})
	}
	for _, tc := range []struct {
		name   string
		fields map[string]string
		extra  formFile
	}{
		{"image", map[string]string{"scale": "0.5"}, formFile{field: "image", filename: "logo.png", content: testPNG(t, 8, 8)}},
		{"font", map[string]string{"text": "hello", "font-size": "10"}, formFile{field: "font", filename: "body.ttf", content: goregular.TTF}},
	} {
		t.Run(tc.name+" accepts file", func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, newMultipartRequest(t, http.MethodPost, "/api/v1/watermark", tc.fields, input, tc.extra))
			if w.Code != http.StatusOK {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if img := decodePNG(t, w.Body.Bytes()); img.Bounds().Dx() != 32 || img.Bounds().Dy() != 16 {
				t.Fatal(img.Bounds())
			}
		})
	}
}

func TestMultipartTypeValidationPreservesParserErrors(t *testing.T) {
	input := rawPart{name: "input", isFile: true, filename: "photo.png", content: testPNG(t, 8, 8)}
	for _, tc := range []struct {
		name          string
		parts         []rawPart
		cfg           Config
		status        int
		code, message string
	}{
		{"mixed duplicate", []rawPart{input, {name: "levels", value: "16"}, {name: "levels", isFile: true, filename: "option.txt", content: []byte("16")}}, Config{NoAuth: true}, 400, "invalid_argument", "duplicate parameter: levels"},
		{"unknown scalar", []rawPart{input, {name: "unknown", value: "1"}}, Config{NoAuth: true}, 400, "invalid_argument", "unknown parameter: unknown"},
		{"unknown file", []rawPart{input, {name: "unknown", isFile: true, filename: "option.txt", content: []byte("1")}}, Config{NoAuth: true}, 400, "invalid_argument", "unknown parameter: unknown"},
		{"request too large", []rawPart{input}, Config{NoAuth: true, MaxUpload: 128}, 413, "payload_too_large", ""},
		{"scalar too large", []rawPart{input, {name: "levels", value: strings.Repeat("1", 4097)}}, Config{NoAuth: true}, 413, "payload_too_large", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			mustNew(t, tc.cfg).ServeHTTP(w, newRawMultipartRequest(t, "/api/v1/grayscale", tc.parts...))
			if w.Code != tc.status || decodeJSONError(t, w.Body.Bytes()) != tc.code || !strings.Contains(w.Body.String(), tc.message) {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func assertMultipartTypeError(t *testing.T, w *httptest.ResponseRecorder, message string) {
	t.Helper()
	if w.Code != http.StatusBadRequest || decodeJSONError(t, w.Body.Bytes()) != "invalid_argument" || !strings.Contains(w.Body.String(), message) {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}
