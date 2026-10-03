// Package httpapi provides the HTTP adapter for Image Tool Box.
package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const (
	DefaultMaxUpload       int64 = 64 << 20
	DefaultMaxPixels       int64 = 50_000_000
	DefaultMaxDimension          = 16_384
	DefaultMaxConcurrent         = 2
	DefaultMaxWorkingBytes int64 = 512 << 20
	DefaultTimeout               = 2 * time.Minute
)

// Config configures the trusted remote HTTP API.
type Config struct {
	Token           string
	NoAuth          bool
	Logger          *slog.Logger
	MaxUpload       int64
	MaxPixels       int64
	MaxDimension    int
	MaxConcurrent   int
	MaxWorkingBytes int64
	Timeout         time.Duration
}

// New creates the versioned Image Tool Box HTTP API. It returns an error when
// the configuration is unusable instead of panicking on invalid limits.
func New(cfg Config) (http.Handler, error) {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return newHandler(cfg), nil
}

// newHandler 组装路由与中间件：accessLog 在最外层记录含 500 的最终
// 状态，recoverMiddleware 把 handler panic 收敛为稳定 JSON 500。
func newHandler(cfg Config) http.Handler {
	sem := make(chan struct{}, cfg.MaxConcurrent)
	mux := http.NewServeMux()
	route(mux, http.MethodGet, "/api/v1/health", http.HandlerFunc(health))
	route(mux, http.MethodPost, "/api/v1/compress", protected(cfg, sem, imageHandler(cfg, "compress", compressImage)))
	route(mux, http.MethodPost, "/api/v1/resize", protected(cfg, sem, imageHandler(cfg, "resize", resizeImage)))
	route(mux, http.MethodPost, "/api/v1/crop", protected(cfg, sem, imageHandler(cfg, "crop", cropImage)))
	route(mux, http.MethodPost, "/api/v1/rotate", protected(cfg, sem, imageHandler(cfg, "rotate", rotateImage)))
	route(mux, http.MethodPost, "/api/v1/convert", protected(cfg, sem, imageHandler(cfg, "convert", convertImage)))
	route(mux, http.MethodPost, "/api/v1/watermark", protected(cfg, sem, imageHandler(cfg, "watermark", watermarkImage)))
	route(mux, http.MethodPost, "/api/v1/inspect", protected(cfg, sem, inspectHandler(cfg)))
	route(mux, http.MethodPost, "/api/v1/barcode/generate", protected(cfg, sem, barcodeHandler(cfg, true)))
	route(mux, http.MethodPost, "/api/v1/barcode/decode", protected(cfg, sem, barcodeHandler(cfg, false)))
	mux.HandleFunc("/", notFound)
	return accessLog(cfg, recoverMiddleware(cfg.Logger, mux))
}

// route 注册限定方法的路径：方法不匹配返回结构化 405 与 Allow 头，
// 使路由层错误与操作错误共用同一 JSON contract。
func route(mux *http.ServeMux, method, pattern string, h http.Handler) {
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("%s is not allowed for this route", r.Method))
			return
		}
		h.ServeHTTP(w, r)
	})
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, fmt.Errorf("route not found"))
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Normalize fills zero-valued fields with service defaults.
func (c *Config) Normalize() {
	if c.MaxUpload == 0 {
		c.MaxUpload = DefaultMaxUpload
	}
	if c.MaxPixels == 0 {
		c.MaxPixels = DefaultMaxPixels
	}
	if c.MaxDimension == 0 {
		c.MaxDimension = DefaultMaxDimension
	}
	if c.MaxConcurrent == 0 {
		c.MaxConcurrent = DefaultMaxConcurrent
	}
	if c.MaxWorkingBytes == 0 {
		c.MaxWorkingBytes = DefaultMaxWorkingBytes
	}
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

// Validate reports whether the configuration holds usable service limits.
// It runs after Normalize, so zero values have already become defaults and
// only genuinely invalid (negative or otherwise unusable) values remain.
func (c Config) Validate() error {
	if c.MaxUpload <= 0 {
		return fmt.Errorf("max upload must be greater than 0")
	}
	if c.MaxPixels <= 0 {
		return fmt.Errorf("max pixels must be greater than 0")
	}
	if c.MaxDimension <= 0 {
		return fmt.Errorf("max dimension must be greater than 0")
	}
	if c.MaxConcurrent <= 0 {
		return fmt.Errorf("max concurrent must be greater than 0")
	}
	if c.MaxWorkingBytes <= 0 {
		return fmt.Errorf("max working bytes must be greater than 0")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be greater than 0")
	}
	return nil
}
