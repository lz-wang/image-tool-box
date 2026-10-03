package httpapi

import (
	"bytes"
	"context"
	"image/png"
	"net/http/httptest"
	"testing"
)

func TestBarcodeGenerateHTTP(t *testing.T) {
	h := mustNew(t, Config{NoAuth: true})
	r := newMultipartRequest(t, "POST", "/api/v1/barcode/generate", map[string]string{"symbology": "qr", "data": "hello"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("X-ITB-Operation") != "barcode.generate" || w.Header().Get("X-ITB-Barcode-Symbology") != "qr" || w.Header().Get("X-ITB-Output-Size") == "" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
	img, err := png.Decode(bytes.NewReader(w.Body.Bytes()))
	if err != nil || img.Bounds().Dx() != 290 {
		t.Fatal(err)
	}
}

func TestBarcodeHTTPAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		cfg        Config
		fields     map[string]string
		file       []byte
		status     int
	}{
		{"bad-symbology", "generate", Config{}, map[string]string{"symbology": "rmqr", "data": "hello"}, nil, 400},
		{"target-dimension", "generate", Config{MaxDimension: 100}, map[string]string{"symbology": "qr", "data": "hello"}, nil, 413},
		{"target-pixels", "generate", Config{MaxPixels: 100}, map[string]string{"symbology": "qr", "data": "hello"}, nil, 413},
		{"target-working", "generate", Config{MaxWorkingBytes: 100}, map[string]string{"symbology": "qr", "data": "hello"}, nil, 413},
		{"huge-plan", "generate", Config{}, map[string]string{"symbology": "qr", "data": "hello", "module-size": "1000000"}, nil, 413},
		{"zero-module", "generate", Config{}, map[string]string{"symbology": "qr", "data": "hello", "module-size": "0"}, nil, 400},
		{"unknown-field", "generate", Config{}, map[string]string{"symbology": "qr", "data": "hello", "force": "true"}, nil, 400},
		{"unexpected-file", "generate", Config{}, map[string]string{"symbology": "qr", "data": "hello"}, testPNG(t, 2, 2), 400},
		{"upload", "generate", Config{MaxUpload: 16}, map[string]string{"symbology": "qr", "data": "hello"}, nil, 413},
		{"source-size", "decode", Config{MaxDimension: 5}, nil, testPNG(t, 10, 10), 413},
		{"source-working", "decode", Config{MaxWorkingBytes: 100}, nil, testPNG(t, 10, 10), 413},
		{"missing-file", "decode", Config{}, nil, nil, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			cfg.NoAuth = true
			h := mustNew(t, cfg)
			var files []formFile
			if tc.file != nil {
				files = append(files, formFile{field: "input", filename: "image.png", content: tc.file})
			}
			r := newMultipartRequest(t, "POST", "/api/v1/barcode/"+tc.path, tc.fields, files...)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			decodeJSONError(t, w.Body.Bytes())
		})
	}
}

func TestBarcodeHTTPProtection(t *testing.T) {
	for _, path := range []string{"generate", "decode"} {
		t.Run(path, func(t *testing.T) {
			url := "/api/v1/barcode/" + path
			h := mustNew(t, Config{Token: "test-token"})
			w := httptest.NewRecorder()
			h.ServeHTTP(w, newMultipartRequest(t, "POST", url, nil))
			if w.Code != 401 {
				t.Fatal(w.Code)
			}
			w = httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
			if w.Code != 405 || w.Header().Get("Allow") != "POST" {
				t.Fatal(w.Code)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			r := newMultipartRequest(t, "POST", url, nil).WithContext(ctx)
			r.Header.Set("Authorization", "Bearer test-token")
			w = httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 504 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
