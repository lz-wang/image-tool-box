package s3

import (
	"fmt"
	"net/url"
	"time"
)

// S3 网络行为的默认值：与 AWS SDK Go v2 标准 retryer 的默认最大
// 尝试次数（3）及 v0.9.x 以来的传输层超时基线一致。
const (
	DefaultMaxAttempts           = 3
	DefaultConnectTimeout        = 30 * time.Second
	DefaultResponseHeaderTimeout = 30 * time.Second
)

// Config S3 客户端配置
type Config struct {
	Endpoint        string // S3 端点 URL
	AccessKeyID     string // Access Key ID
	SecretAccessKey string // Secret Access Key
	SessionToken    string // 临时凭证 Session Token（长期凭证留空）
	Region          string // 区域
	Bucket          string // 存储桶名称
	ForcePathStyle  bool   // 是否强制路径样式（MinIO 需要）

	// MaxAttempts 是单个 S3 API 操作的最大尝试次数（含首次），
	// 由 AWS SDK 标准 retryer 执行，默认 3。
	MaxAttempts int

	// ConnectTimeout 限制 TCP 连接建立时长，默认 30s。
	ConnectTimeout time.Duration

	// ResponseHeaderTimeout 限制"请求发出后等待响应头"的时长，
	// 不限制 body 传输时长，默认 30s。
	ResponseHeaderTimeout time.Duration
}

// Normalize 归一化配置：补全 region 与网络行为默认值，并按端点自动
// 启用路径样式。
//
// 只有零值视为"未设置"并补默认；负值不是缺省信号，由 Validate 显式
// 拒绝——静默回退默认会让 domain 直调与 CLI adapter 走出不同契约。
//
// 环境变量不在领域层读取：ITB_S3_* 由 CLI 层（urfave/cli 的
// Sources）解析注入，优先级为 CLI flag > 环境变量 > 默认值。
func (c *Config) Normalize() {
	if c.Region == "" {
		c.Region = "us-east-1"
	}
	if c.MaxAttempts == 0 {
		c.MaxAttempts = DefaultMaxAttempts
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = DefaultConnectTimeout
	}
	if c.ResponseHeaderTimeout == 0 {
		c.ResponseHeaderTimeout = DefaultResponseHeaderTimeout
	}

	// 自动检测 MinIO 等自建端点，默认启用路径样式。
	// 用 url.Parse 的 Hostname/Port 精确判断，而不是子串匹配：
	// "https://example.com/?redirect=localhost" 这类 URL 不应误命中。
	if c.Endpoint != "" && !c.ForcePathStyle {
		if u, err := url.Parse(c.Endpoint); err == nil {
			switch host := u.Hostname(); host {
			case "localhost", "127.0.0.1", "::1":
				c.ForcePathStyle = true
			default:
				// 9000 是 MinIO 默认端口（如 http://minio.internal:9000）
				if u.Port() == "9000" {
					c.ForcePathStyle = true
				}
			}
		}
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if err := c.ValidateNetwork(); err != nil {
		return err
	}
	if c.Endpoint == "" {
		return ErrMissingEndpoint
	}
	if c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return ErrMissingCredentials
	}
	if c.Bucket == "" {
		return ErrMissingBucket
	}
	return nil
}

// ValidateNetwork 校验网络行为参数：负值是配置错误（Normalize 只把
// 零值视为"未设置"）。
func (c *Config) ValidateNetwork() error {
	if c.MaxAttempts < 0 {
		return fmt.Errorf("%w: max-attempts must not be negative, got %d", ErrInvalidConfig, c.MaxAttempts)
	}
	if c.ConnectTimeout < 0 {
		return fmt.Errorf("%w: connect-timeout must not be negative, got %s", ErrInvalidConfig, c.ConnectTimeout)
	}
	if c.ResponseHeaderTimeout < 0 {
		return fmt.Errorf("%w: response-header-timeout must not be negative, got %s", ErrInvalidConfig, c.ResponseHeaderTimeout)
	}
	return nil
}

// ValidateWithoutBucket 验证配置（不验证 bucket，用于 list buckets 等操作）
func (c *Config) ValidateWithoutBucket() error {
	if err := c.ValidateNetwork(); err != nil {
		return err
	}
	if c.Endpoint == "" {
		return ErrMissingEndpoint
	}
	if c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return ErrMissingCredentials
	}
	return nil
}

// String 返回配置的安全字符串表示（隐藏敏感信息）
func (c *Config) String() string {
	secret := ""
	if c.SecretAccessKey != "" {
		if len(c.SecretAccessKey) > 4 {
			secret = c.SecretAccessKey[:4] + "****"
		} else {
			secret = "****"
		}
	}
	return fmt.Sprintf("Config{Endpoint: %s, AccessKeyID: %s, SecretAccessKey: %s, Region: %s, Bucket: %s, ForcePathStyle: %v}",
		c.Endpoint, c.AccessKeyID, secret, c.Region, c.Bucket, c.ForcePathStyle)
}
