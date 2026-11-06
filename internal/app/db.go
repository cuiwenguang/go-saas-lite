package app

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/migrations"
	"github.com/cuiwenguang/go-saas-lite/pkg/migrate"
	"github.com/cuiwenguang/go-saas-lite/pkg/obs"
	"github.com/cuiwenguang/go-saas-lite/pkg/redisx"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// tenantGorm / platformGorm 是模块内类型：容器外拿不到 *gorm.DB（readme 3.3）。
type tenantGorm struct{ db *gorm.DB }
type platformGorm struct{ db *gorm.DB }

func newPlatformGorm(lc fx.Lifecycle, cfg Config) (platformGorm, error) {
	db, err := tenancy.OpenPostgres(cfg.Database.PlatformDSN, tenancy.Silent())
	if err != nil {
		return platformGorm{}, err
	}

	pool, err := db.DB()
	if err != nil {
		return platformGorm{}, fmt.Errorf("get platform pool: %w", err)
	}
	pool.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	pool.SetMaxIdleConns(cfg.Database.MaxIdleConns)

	lc.Append(fx.Hook{
		// 启动时校验迁移 checksum，脚本漂移直接拒绝启动（readme 7）。
		OnStart: func(ctx context.Context) error {
			runner := migrate.New(db, migrations.FS)
			for _, group := range migrate.Groups() {
				if verifyErr := runner.Verify(ctx, group); verifyErr != nil {
					return fmt.Errorf("verify %s migrations: %w", group, verifyErr)
				}
			}
			return nil
		},
		OnStop: func(context.Context) error {
			return pool.Close()
		},
	})

	return platformGorm{db: db}, nil
}

func newTenantGorm(lc fx.Lifecycle, cfg Config) (tenantGorm, error) {
	db, err := tenancy.OpenPostgres(cfg.Database.TenantDSN, tenancy.WithTenantCallbacks(), tenancy.Silent())
	if err != nil {
		return tenantGorm{}, err
	}

	pool, err := db.DB()
	if err != nil {
		return tenantGorm{}, fmt.Errorf("get tenant pool: %w", err)
	}
	pool.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	pool.SetMaxIdleConns(cfg.Database.MaxIdleConns)

	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			return pool.Close()
		},
	})

	return tenantGorm{db: db}, nil
}

func newTenantDB(g tenantGorm) tenancy.DB {
	return tenancy.NewDB(g.db)
}

func newPlatformDB(g platformGorm) tenancy.PlatformDB {
	return tenancy.NewPlatformDB(g.db)
}

// newRedisClient 在未配置 Addr 时返回 nil，调用方需自行判空。
func newRedisClient(lc fx.Lifecycle, cfg Config) *redis.Client {
	if cfg.Redis.Addr == "" {
		return nil
	}

	client := redisx.New(redisx.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			return client.Close()
		},
	})

	return client
}

type dbChecker struct{ db *gorm.DB }

func (c dbChecker) Name() string { return "postgres" }

func (c dbChecker) Check(ctx context.Context) error {
	sqlDB, err := c.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

type redisChecker struct{ client *redis.Client }

func (c redisChecker) Name() string { return "redis" }

func (c redisChecker) Check(ctx context.Context) error {
	if c.client == nil {
		return nil
	}
	return redisx.Ping(ctx, c.client)
}

// registerHealth 把 DB 与 Redis 纳入 /readyz。
func registerHealth(health *obs.Health, platform platformGorm, client *redis.Client) {
	health.Register(dbChecker{db: platform.db}, redisChecker{client: client})
}
