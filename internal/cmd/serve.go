package cmd

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
	"imagetoolbox/internal/httpapi"
)

func newServeCommand() *cli.Command {
	return &cli.Command{
		Name:     "serve",
		Usage:    "Run the HTTP API server",
		Category: categoryService,
		Description: `Run the Image Tool Box HTTP API server.

The API exposes the domain image operations under /api/v1
(compress, resize, crop, rotate, convert, watermark, inspect)
plus /api/v1/health. It is a trusted remote service, not a
remote shell: there is no WebUI, no workflow orchestration,
and no S3 management.

DEFAULTS:
  Binds to 127.0.0.1:8080 (loopback only).

CONSTRAINTS:
  Authentication uses the ITB_API_TOKEN bearer token and is
  required unless --no-auth is set.
  --no-auth is allowed only on loopback addresses.
  Remote deployment requires ITB_API_TOKEN and a reverse
  proxy in front of the server.

EXAMPLES:
  itb serve
	  itb serve --addr 127.0.0.1:9000`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "addr",
				Value: "127.0.0.1:8080",
				Usage: "Listen `ADDRESS` (loopback only by default)",
			},
			&cli.StringFlag{Name: "max-upload", Value: "64MiB", Usage: "Maximum multipart request `SIZE` (KiB/MiB/GiB)"},
			&cli.Int64Flag{Name: "max-pixels", Value: httpapi.DefaultMaxPixels, Usage: "Maximum `PIXELS` per image", Validator: positiveInt64Validator("max-pixels")},
			&cli.IntFlag{Name: "max-dimension", Value: httpapi.DefaultMaxDimension, Usage: "Maximum single-side dimension in `PIXELS`", Validator: positiveIntValidator("max-dimension")},
			&cli.IntFlag{Name: "max-concurrent", Value: httpapi.DefaultMaxConcurrent, Usage: "Maximum concurrent image operations", Validator: positiveIntValidator("max-concurrent")},
			&cli.StringFlag{Name: "max-working-bytes", Value: "512MiB", Usage: "Working-set memory limit `SIZE` per operation (watermark, arbitrary-angle rotate, etc.)"},
			&cli.DurationFlag{Name: "timeout", Value: httpapi.DefaultTimeout, Usage: "Timeout per image operation", Validator: positiveDurationValidator("timeout")},
			&cli.BoolFlag{Name: "no-auth", Usage: "Disable authentication; loopback development only"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runServe(ctx, cmd)
		},
	}
}

func runServe(ctx context.Context, cmd *cli.Command) error {
	addr := cmd.String("addr")
	noAuth := cmd.Bool("no-auth")
	if noAuth && !isLoopbackAddress(addr) {
		return invalidArgument("--no-auth 只能用于 loopback 地址")
	}
	token := os.Getenv("ITB_API_TOKEN")
	if !noAuth && token == "" {
		return invalidArgument("ITB_API_TOKEN is required unless --no-auth is set")
	}
	maxUpload, err := parseByteSize(cmd.String("max-upload"))
	if err != nil {
		return err
	}
	maxWorkingBytes, err := parseByteSize(cmd.String("max-working-bytes"))
	if err != nil {
		return err
	}
	handler, err := httpapi.New(httpapi.Config{Token: token, NoAuth: noAuth, MaxUpload: maxUpload, MaxPixels: cmd.Int64("max-pixels"), MaxDimension: cmd.Int("max-dimension"), MaxConcurrent: cmd.Int("max-concurrent"), MaxWorkingBytes: maxWorkingBytes, Timeout: cmd.Duration("timeout")})
	if err != nil {
		return fmt.Errorf("invalid HTTP API configuration: %w", err)
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	url := "http://" + addr
	fmt.Printf("Image Tool Box HTTP API 已启动: %s/api/v1/health\n", url)
	fmt.Println("按 Ctrl+C 停止")

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("关闭 HTTP API 失败: %w", err)
		}
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("启动 HTTP API 失败: %w", err)
		}
	}
	return nil
}

func isLoopbackAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func parseByteSize(value string) (int64, error) {
	v := strings.ToUpper(strings.TrimSpace(value))
	units := map[string]int64{"KIB": 1 << 10, "MIB": 1 << 20, "GIB": 1 << 30}
	for suffix, multiplier := range units {
		if strings.HasSuffix(v, suffix) {
			n, err := strconv.ParseInt(strings.TrimSpace(strings.TrimSuffix(v, suffix)), 10, 64)
			if err != nil || n <= 0 {
				return 0, fmt.Errorf("invalid byte size: %s", value)
			}
			if n > math.MaxInt64/multiplier {
				return 0, fmt.Errorf("byte size exceeds int64 range: %s", value)
			}
			return n * multiplier, nil
		}
	}
	return 0, fmt.Errorf("byte size must use KiB, MiB, or GiB: %s", value)
}
