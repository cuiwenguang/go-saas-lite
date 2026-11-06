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
	Tenancy  TenancyConfig
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
	if c.Worker.CoolingDays <= 0 {
		return fmt.Errorf("worker.cooling_days must be positive")
	}
	switch strings.ToLower(c.Trace.Exporter) {
	case "", "none", "stdout":
	default:
		return fmt.Errorf("unsupported trace.exporter %q", c.Trace.Exporter)
	}
	return nil
}
