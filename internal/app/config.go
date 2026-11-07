package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config 是进程级配置（yaml + env，重启生效）。
// 平台运行时配置与租户配置属于后续里程碑的数据库层，不在这里。
type Config struct {
	App      AppConfig
	Server   ServerConfig
	Log      LogConfig
	Trace    TraceConfig
	Metrics  MetricsConfig
	Auth     AuthConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Worker   WorkerConfig
	Audit    AuditConfig
	Tenancy  TenancyConfig
	MCP      MCPConfig
	Storage  StorageConfig
	SMS      SMSConfig
	Portal   PortalConfig
	Queue    QueueConfig
	Docs     DocsConfig
}

// DocsConfig 控制 OpenAPI 文档与 Swagger UI。
type DocsConfig struct {
	Enabled bool `koanf:"enabled"`
}

// QueueConfig 是 asynq 异步任务参数。
type QueueConfig struct {
	Enabled     bool `koanf:"enabled"`
	Concurrency int  `koanf:"concurrency"`
	// Heartbeat 开启一个每分钟的示例定时任务，仅用于验证调度器。
	Heartbeat bool `koanf:"heartbeat"`
}

// PortalConfig 是营销门户与自助注册参数。
type PortalConfig struct {
	Enabled           bool   `koanf:"enabled"`
	ConsoleURL        string `koanf:"console_url"`
	AutoApprove       bool   `koanf:"auto_approve"`
	SignupRatePerHour int    `koanf:"signup_rate_per_hour"`
}

// SMSConfig 是短信通道参数。
type SMSConfig struct {
	Provider string `koanf:"provider"` // dev
}

// StorageConfig 是对象存储参数。
type StorageConfig struct {
	Driver      string             `koanf:"driver"`        // local | cos | oss
	MaxUploadMB int                `koanf:"max_upload_mb"` // 单文件上限
	Local       LocalStorageConfig `koanf:"local"`
	COS         COSStorageConfig   `koanf:"cos"`
	OSS         OSSStorageConfig   `koanf:"oss"`
}

// LocalStorageConfig 是本地磁盘驱动参数。
type LocalStorageConfig struct {
	Root       string `koanf:"root"`
	BaseURL    string `koanf:"base_url"`
	SigningKey string `koanf:"signing_key"`
}

// COSStorageConfig 是腾讯云 COS 参数。
type COSStorageConfig struct {
	Bucket    string `koanf:"bucket"`
	Region    string `koanf:"region"`
	SecretID  string `koanf:"secret_id"`
	SecretKey string `koanf:"secret_key"`
	Endpoint  string `koanf:"endpoint"`
}

// OSSStorageConfig 是阿里云 OSS 参数。
type OSSStorageConfig struct {
	Bucket          string `koanf:"bucket"`
	Endpoint        string `koanf:"endpoint"`
	AccessKeyID     string `koanf:"access_key_id"`
	AccessKeySecret string `koanf:"access_key_secret"`
}

func (c StorageConfig) validate() error {
	switch strings.ToLower(strings.TrimSpace(c.Driver)) {
	case "", "local":
		if strings.TrimSpace(c.Local.Root) == "" {
			return fmt.Errorf("storage.local.root is required")
		}
	case "cos":
		if c.COS.Bucket == "" || c.COS.SecretID == "" || c.COS.SecretKey == "" {
			return fmt.Errorf("storage.cos bucket, secret_id and secret_key are required")
		}
		if c.COS.Region == "" && c.COS.Endpoint == "" {
			return fmt.Errorf("storage.cos region or endpoint is required")
		}
	case "oss":
		if c.OSS.Bucket == "" || c.OSS.Endpoint == "" || c.OSS.AccessKeyID == "" || c.OSS.AccessKeySecret == "" {
			return fmt.Errorf("storage.oss bucket, endpoint, access_key_id and access_key_secret are required")
		}
	default:
		return fmt.Errorf("unsupported storage.driver %q", c.Driver)
	}

	return nil
}

// MCPConfig 是 MCP 网关参数。
type MCPConfig struct {
	// CredentialKey 用于上游凭据的 AES-GCM 静态加密；为空时按明文存储（仅开发）。
	CredentialKey string `koanf:"credential_key"`
}

// AuditConfig 是审计日志参数。
type AuditConfig struct {
	// RetentionDays 是审计日志保留天数；worker 每天清理超过该天数的记录。
	RetentionDays int `koanf:"retention_days"`
}

// TenancyConfig 是租户域名解析参数。
type TenancyConfig struct {
	BaseDomain string `koanf:"base_domain"`
}

// WorkerConfig 是后台任务参数。
type WorkerConfig struct {
	IntervalSeconds int `koanf:"interval_seconds"`
	CoolingDays     int `koanf:"cooling_days"`
}

// DatabaseConfig 是双连接池参数；v1 两者可以指向同一个库。
type DatabaseConfig struct {
	PlatformDSN  string `koanf:"platform_dsn"`
	TenantDSN    string `koanf:"tenant_dsn"`
	MaxOpenConns int    `koanf:"max_open_conns"`
	MaxIdleConns int    `koanf:"max_idle_conns"`
}

// RedisConfig 是缓存与广播参数；Addr 为空时不启用。
type RedisConfig struct {
	Addr     string `koanf:"addr"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"`
}

// AuthConfig 是 JWT 签发参数。
type AuthConfig struct {
	IssuerName       string `koanf:"issuer_name"`
	Secret           string `koanf:"secret"`
	AccessTTLSeconds int    `koanf:"access_ttl_seconds"`
}

// AppConfig 是应用标识。
type AppConfig struct {
	Name string `koanf:"name"`
	Env  string `koanf:"env"`
}

// ServerConfig 是 HTTP 服务参数；时长统一用秒，避免解析歧义。
type ServerConfig struct {
	Addr                 string   `koanf:"addr"`
	ReadHeaderTimeoutSec int      `koanf:"read_header_timeout_sec"`
	ReadTimeoutSec       int      `koanf:"read_timeout_sec"`
	WriteTimeoutSec      int      `koanf:"write_timeout_sec"`
	IdleTimeoutSec       int      `koanf:"idle_timeout_sec"`
	ShutdownTimeoutSec   int      `koanf:"shutdown_timeout_sec"`
	AllowedOrigins       []string `koanf:"allowed_origins"`
	// MaxBodyMB 是普通 JSON 请求体上限；文件上传与 MCP 各自限制。
	MaxBodyMB int `koanf:"max_body_mb"`
}

// LogConfig 是日志参数。
type LogConfig struct {
	Level  string `koanf:"level"`
	Format string `koanf:"format"`
}

// TraceConfig 是链路追踪参数；Exporter 支持 none / stdout。
type TraceConfig struct {
	Enabled  bool   `koanf:"enabled"`
	Exporter string `koanf:"exporter"`
}

// MetricsConfig 控制 /metrics 是否暴露。
type MetricsConfig struct {
	Enabled bool `koanf:"enabled"`
}

// DefaultConfig 返回可运行的内置默认值，作为配置合并的基线。
func DefaultConfig() Config {
	return Config{
		App: AppConfig{
			Name: "go-saas-lite",
			Env:  "dev",
		},
		Server: ServerConfig{
			Addr:                 ":8080",
			ReadHeaderTimeoutSec: 5,
			ReadTimeoutSec:       10,
			WriteTimeoutSec:      30,
			IdleTimeoutSec:       60,
			ShutdownTimeoutSec:   15,
			MaxBodyMB:            2,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
		Trace: TraceConfig{
			Enabled:  true,
			Exporter: "none",
		},
		Metrics: MetricsConfig{
			Enabled: true,
		},
		Auth: AuthConfig{
			IssuerName:       "go-saas-lite",
			AccessTTLSeconds: 900,
		},
		Database: DatabaseConfig{
			MaxOpenConns: 20,
			MaxIdleConns: 5,
		},
		Worker: WorkerConfig{
			IntervalSeconds: 60,
			CoolingDays:     7,
		},
		Audit: AuditConfig{
			RetentionDays: 90,
		},
		Storage: StorageConfig{
			Driver:      "local",
			MaxUploadMB: 20,
			Local: LocalStorageConfig{
				Root:    "./data/files",
				BaseURL: "http://localhost:8080/api/files/download",
			},
		},
		SMS: SMSConfig{Provider: "dev"},
		Portal: PortalConfig{
			Enabled:           true,
			ConsoleURL:        "http://localhost:5173",
			AutoApprove:       true,
			SignupRatePerHour: 5,
		},
		Queue: QueueConfig{Enabled: true, Concurrency: 10},
		Docs:  DocsConfig{Enabled: true},
	}
}

// LoadConfig 读取 yaml 并用 SAAS_ 前缀环境变量覆盖。
// 环境变量名把 "__" 还原成 "."，例如 SAAS_SERVER__ADDR。
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	k := koanf.New(".")

	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
				return Config{}, fmt.Errorf("load config file %s: %w", path, err)
			}
		}
	}

	if err := k.Load(env.Provider("SAAS_", ".", func(key string) string {
		key = strings.TrimPrefix(key, "SAAS_")
		return strings.ToLower(strings.ReplaceAll(key, "__", "."))
	}), nil); err != nil {
		return Config{}, fmt.Errorf("load config env: %w", err)
	}

	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// applyDefaults 只在 dev 下提供固定密钥，便于本地签发与联调。
func (c *Config) applyDefaults() {
	if c.Auth.Secret == "" && c.App.Env == "dev" {
		c.Auth.Secret = "dev-insecure-jwt-secret"
	}
	if c.Storage.Local.SigningKey == "" {
		c.Storage.Local.SigningKey = c.Auth.Secret
	}
}

func (c Config) validate() error {
	if c.Server.Addr == "" {
		return fmt.Errorf("server.addr is required")
	}
	if c.Server.ShutdownTimeoutSec <= 0 {
		return fmt.Errorf("server.shutdown_timeout_sec must be positive")
	}
	if c.Auth.Secret == "" {
		return fmt.Errorf("auth.secret is required outside dev")
	}
	if c.Auth.AccessTTLSeconds <= 0 {
		return fmt.Errorf("auth.access_ttl_seconds must be positive")
	}
	if c.Database.PlatformDSN == "" {
		return fmt.Errorf("database.platform_dsn is required")
	}
	if c.Database.TenantDSN == "" {
		return fmt.Errorf("database.tenant_dsn is required")
	}
	if c.Worker.IntervalSeconds <= 0 {
		return fmt.Errorf("worker.interval_seconds must be positive")
	}
	if c.Audit.RetentionDays <= 0 {
		return fmt.Errorf("audit.retention_days must be positive")
	}
	if c.Worker.CoolingDays <= 0 {
		return fmt.Errorf("worker.cooling_days must be positive")
	}
	if err := c.Storage.validate(); err != nil {
		return err
	}
	switch strings.ToLower(c.Trace.Exporter) {
	case "", "none", "stdout":
	default:
		return fmt.Errorf("unsupported trace.exporter %q", c.Trace.Exporter)
	}
	return nil
}
