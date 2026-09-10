package s3

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestConfigValidateNetworkNegatives 网络行为参数为负是配置错误：
// Normalize 只把零值视为"未设置"，负值必须显式拒绝，绝不静默回退
// 默认——否则 domain 直调与 CLI adapter 走出不同契约。
func TestConfigValidateNetworkNegatives(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"负 max-attempts", Config{MaxAttempts: -1}},
		{"负 connect-timeout", Config{ConnectTimeout: -time.Second}},
		{"负 response-header-timeout", Config{ResponseHeaderTimeout: -time.Second}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			cfg.Normalize()
			err := cfg.Validate()
			if err == nil || !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Validate() = %v, want ErrInvalidConfig", err)
			}
			if err := cfg.ValidateNetwork(); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("ValidateNetwork() = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

// TestDownloadOptionsValidate digest 格式、负 expect-size 与非法
// if-exists 必须在选项阶段拒绝。
func TestDownloadOptionsValidate(t *testing.T) {
	negative := int64(-1)
	tests := []struct {
		name    string
		opts    DownloadOptions
		wantErr error
	}{
		{"短 digest", DownloadOptions{VerifySHA256: "0000"}, ErrInvalidSHA256},
		{"非十六进制 digest", DownloadOptions{VerifySHA256: "zzzz"}, ErrInvalidSHA256},
		{"负 expect-size", DownloadOptions{ExpectSize: &negative}, ErrInvalidOptions},
		{"非法 if-exists", DownloadOptions{IfExists: "skip"}, ErrInvalidIfExists},
		{"全部合法", DownloadOptions{VerifySHA256: helloSHA256, ExpectSize: int64Ptr(0), IfExists: IfExistsVerify}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestDownloadOptionsNormalize 空字符串 if-exists 归一化为 replace。
func TestDownloadOptionsNormalize(t *testing.T) {
	var opts DownloadOptions
	opts.Normalize()
	if opts.IfExists != IfExistsReplace {
		t.Fatalf("IfExists = %q, want replace", opts.IfExists)
	}
}

// TestUploadOptionsValidate skip 策略互斥与非法 if-exists 在选项阶段
// 拒绝（与 Upload 入口同一防线）。
func TestUploadOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    UploadOptions
		wantErr error
	}{
		{"skip-existing + skip-unchanged", UploadOptions{SkipExisting: true, SkipUnchanged: true}, ErrSkipStrategyConflict},
		{"skip-unchanged + skip-matching", UploadOptions{SkipUnchanged: true, SkipMatching: true}, ErrSkipStrategyConflict},
		{"三个 skip 策略", UploadOptions{SkipExisting: true, SkipUnchanged: true, SkipMatching: true}, ErrSkipStrategyConflict},
		{"非法 if-exists", UploadOptions{IfExists: "overwrite"}, ErrInvalidIfExists},
		{"全部合法", UploadOptions{SkipMatching: true, IfExists: IfExistsVerify}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestUploadOptionsNormalize 空字符串 if-exists 归一化为 replace。
func TestUploadOptionsNormalize(t *testing.T) {
	var opts UploadOptions
	opts.Normalize()
	if opts.IfExists != IfExistsReplace {
		t.Fatalf("IfExists = %q, want replace", opts.IfExists)
	}
}

// TestErrInvalidOptionsMessage sentinel 消息不携带参数值（稳定文案由
// 错误分类层包装）。
func TestErrInvalidOptionsMessage(t *testing.T) {
	if strings.Contains(ErrInvalidOptions.Error(), "%") {
		t.Errorf("sentinel message must be static, got %q", ErrInvalidOptions.Error())
	}
}
