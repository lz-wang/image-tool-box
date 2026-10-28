package httpapi

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"imagetoolbox/internal/grayscale"
)

func TestGrayscaleHTTP(t *testing.T) {
	input := formFile{field: "input", filename: "photo.png", content: testPNG(t, 32, 16)}
	for _, tc := range []struct {
		name   string
		fields map[string]string
		cfg    Config
		want   int
	}{
		{"defaults", nil, Config{NoAuth: true}, 200},
		{"FS", map[string]string{"levels": "16", "dither": "fs"}, Config{NoAuth: true}, 200},
		{"ordered", map[string]string{"levels": "4", "dither": "ordered"}, Config{NoAuth: true}, 200},
		{"jpeg", map[string]string{"to": "jpg"}, Config{NoAuth: true}, 200},
		{"webp", map[string]string{"to": "webp", "lossless": "true"}, Config{NoAuth: true}, 200},
		{"levels", map[string]string{"levels": "3"}, Config{NoAuth: true}, 400},
		{"number", map[string]string{"levels": "x"}, Config{NoAuth: true}, 400},
		{"quality", map[string]string{"quality": "101"}, Config{NoAuth: true}, 400},
		{"background", map[string]string{"background": "#00000000"}, Config{NoAuth: true}, 400},
		{"dither", map[string]string{"dither": "fs"}, Config{NoAuth: true}, 400},
		{"lossless jpeg", map[string]string{"to": "jpeg", "lossless": "true"}, Config{NoAuth: true}, 400},
		{"unknown", map[string]string{"output": "x.png"}, Config{NoAuth: true}, 400},
		{"unauthorized", nil, Config{Token: "secret"}, 401},
		{"dimension", nil, Config{NoAuth: true, MaxDimension: 16}, 413},
		{"pixels", nil, Config{NoAuth: true, MaxPixels: 511}, 413},
		{"memory", map[string]string{"levels": "16", "dither": "fs"}, Config{NoAuth: true, MaxWorkingBytes: 6144}, 413},
		{"memory boundary", nil, Config{NoAuth: true, MaxWorkingBytes: 6144}, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			mustNew(t, tc.cfg).ServeHTTP(w, newMultipartRequest(t, http.MethodPost, "/api/v1/grayscale", tc.fields, input))
			if w.Code != tc.want {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.want == 413 && decodeJSONError(t, w.Body.Bytes()) != "image_too_large" {
				t.Fatal(w.Body.String())
			}
			if tc.want != 200 {
				return
			}
			if w.Header().Get("X-ITB-Operation") != "grayscale" || w.Header().Get("X-ITB-Input-Size") == "" || w.Header().Get("X-ITB-Output-Size") == "" {
				t.Fatal(w.Header())
			}
			if !strings.Contains(w.Header().Get("Content-Disposition"), "photo_grayscale.") {
				t.Fatal(w.Header())
			}
			if tc.fields["to"] != "" {
				return
			}
			img, err := png.Decode(bytes.NewReader(w.Body.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := img.(*image.Gray); !ok {
				t.Fatalf("not Gray8: %T", img)
			}
			dir := t.TempDir()
			src := filepath.Join(dir, "input.png")
			dst := filepath.Join(dir, "out.png")
			if err := os.WriteFile(src, input.content, 0o600); err != nil {
				t.Fatal(err)
			}
			opts := grayscale.Options{}
			if tc.name == "FS" {
				opts.Levels = 16
				opts.Dither = grayscale.DitherFS
			}
			if tc.name == "ordered" {
				opts.Levels = 4
				opts.Dither = grayscale.DitherOrdered
			}
			if err := grayscale.ConvertFile(src, dst, opts); err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(want, w.Body.Bytes()) {
				t.Fatal("HTTP/domain outputs differ")
			}
		})
	}
}

func TestGrayscaleAdmissionBeforeDecode(t *testing.T) {
	// Valid IHDR, no pixel payload: admission must win over the decode failure.
	for _, tc := range []struct {
		cfg  Config
		w, h int
	}{
		{Config{NoAuth: true, MaxDimension: 16}, 32, 16},
		{Config{NoAuth: true, MaxWorkingBytes: 100}, 32, 16},
	} {
		t.Run("header", func(t *testing.T) {
			w := httptest.NewRecorder()
			mustNew(t, tc.cfg).ServeHTTP(w, newMultipartRequest(t, http.MethodPost, "/api/v1/grayscale", nil, formFile{field: "input", filename: "huge.png", content: pngHeader(t, tc.w, tc.h)}))
			if w.Code != 413 || decodeJSONError(t, w.Body.Bytes()) != "image_too_large" {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestGrayscaleConcurrentOptions(t *testing.T) {
	h := mustNew(t, Config{Token: "secret", MaxConcurrent: 8})
	input := formFile{field: "input", filename: "same.png", content: testPNG(t, 8, 8)}
	type result struct {
		w     *httptest.ResponseRecorder
		white bool
	}
	results := make(chan result, 8)
	for i := range 8 {
		white := i%2 == 0
		bg := "#000"
		if white {
			bg = "#FFF"
		}
		// testPNG is transparent: background choices must remain isolated.
		req := newMultipartRequest(t, http.MethodPost, "/api/v1/grayscale", map[string]string{"background": bg}, input)
		req.Header.Set("Authorization", "Bearer secret")
		go func() { w := httptest.NewRecorder(); h.ServeHTTP(w, req); results <- result{w, white} }()
	}
	for range 8 {
		r := <-results
		if r.w.Code != 200 {
			t.Fatal(r.w.Code, r.w.Body.String())
		}
		img := decodePNG(t, r.w.Body.Bytes())
		want := uint32(0)
		if r.white {
			want = 65535
		}
		rr, g, b, a := img.At(0, 0).RGBA()
		if rr != want || g != want || b != want || a != 65535 {
			t.Fatalf("background leaked: %d %d %d %d", rr, g, b, a)
		}
	}
}
