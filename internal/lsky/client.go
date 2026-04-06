package lsky

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config LskyPro 配置
type Config struct {
	BaseURL string
	Token   string
}

// LoadFromEnv 从环境变量加载配置
func (c *Config) LoadFromEnv() {
	if c.BaseURL == "" {
		c.BaseURL = os.Getenv("LSKY_URL")
	}
	if c.Token == "" {
		c.Token = os.Getenv("LSKY_TOKEN")
	}
}

// Validate 校验配置
func (c *Config) Validate() error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("缺少 LskyPro 地址，请通过 --url 或 LSKY_URL 提供")
	}
	if strings.TrimSpace(c.Token) == "" {
		return fmt.Errorf("缺少 LskyPro Token，请通过 --token 或 LSKY_TOKEN 提供")
	}
	return nil
}

// Client LskyPro API 客户端
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient 创建客户端
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不能为空")
	}
	cfg.LoadFromEnv()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Client{
		baseURL: normalizeBaseURL(cfg.BaseURL),
		token:   strings.TrimSpace(cfg.Token),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

func normalizeBaseURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if strings.HasSuffix(base, "/api/v1") {
		return base
	}
	if strings.HasSuffix(base, "/api") {
		return base + "/v1"
	}
	return base + "/api/v1"
}
