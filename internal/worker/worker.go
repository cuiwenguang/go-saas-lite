// Package worker 承载后台任务：归档租户清理与用量落库（readme 1.4 / 5）。
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

const maintenanceLockKey = "worker:lock:maintenance"

// Options 是 worker 运行参数。
type Options struct {
	Interval    time.Duration
	CoolingDays int
	// Cleaners 是业务侧注册的数据清理钩子（readme 1.4）。
	Cleaners []Cleaner
}

// Cleaner 清理某租户的业务数据。
type Cleaner interface {
	Purge(ctx context.Context, tenantID string) error
}

// Worker 周期性执行维护任务。
type Worker struct {
	platform tenancy.PlatformDB
	tenant   tenancy.DB
	quota    *quota.Service
	client   *redis.Client
	log      *slog.Logger
	opts     Options
	now      func() time.Time

	lockValue string
	wg        sync.WaitGroup
}

// New 创建 worker 并挂到 fx 生命周期。
func New(
	lc fx.Lifecycle,
	opts Options,
	platform tenancy.PlatformDB,
	tenantDB tenancy.DB,
	client *redis.Client,
	log *slog.Logger,
) *Worker {
	w := NewRunner(opts, platform, tenantDB, client, log)

	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			w.wg.Add(1)
			go func() {
				defer w.wg.Done()
				w.loop(ctx)
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			w.wg.Wait()
			return nil
		},
	})

	return w
}

// NewRunner 创建不带生命周期钩子的 worker，测试与一次性任务复用。
func NewRunner(
	opts Options,
	platform tenancy.PlatformDB,
	tenantDB tenancy.DB,
	client *redis.Client,
	log *slog.Logger,
) *Worker {
	w := &Worker{
		platform:  platform,
		tenant:    tenantDB,
		client:    client,
		log:       log,
		opts:      opts,
		now:       time.Now,
		lockValue: uuid.Must(uuid.NewV7()).String(),
	}
	if client != nil {
		w.quota = quota.NewService(platform, client)
	}

	return w
}

func (w *Worker) loop(ctx context.Context) {
	interval := w.opts.Interval
	if interval <= 0 {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil {
				w.log.ErrorContext(ctx, "worker tick failed", "err", err)
			}
		}
	}
}

// RunOnce 执行一轮维护，供测试与 ticker 复用。
func (w *Worker) RunOnce(ctx context.Context) error {
	if !w.acquireLock(ctx) {
		return nil
	}
	defer w.releaseLock(ctx)

	if w.quota != nil {
		flushed, err := w.quota.Flush(ctx)
		if err != nil {
			w.log.ErrorContext(ctx, "flush usage failed", "err", err)
		} else if flushed > 0 {
			w.log.InfoContext(ctx, "usage flushed", "rows", flushed)
		}
	}

	purged, err := w.PurgeArchived(ctx)
	if err != nil {
		return err
	}
	if purged > 0 {
		w.log.InfoContext(ctx, "archived tenants purged", "count", purged)
	}

	return nil
}

// PurgeArchived 清理超过冷却期的归档租户，返回处理数量。
func (w *Worker) PurgeArchived(ctx context.Context) (int, error) {
	cutoff := w.now().AddDate(0, 0, -w.opts.CoolingDays)

	var tenants []tenantRecord
	err := w.platform.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("status = ? AND updated_at < ?", "archived", cutoff).
			Limit(50).
			Find(&tenants)
	})
	if err != nil {
		return 0, fmt.Errorf("worker: list archived tenants: %w", err)
	}

	purged := 0
	var firstErr error
	for _, tenant := range tenants {
		if purgeErr := w.purgeTenant(ctx, tenant.ID); purgeErr != nil {
			w.log.ErrorContext(ctx, "purge tenant failed", "tenant_id", tenant.ID, "err", purgeErr)
			if firstErr == nil {
				firstErr = purgeErr
			}
			continue
		}
		purged++
	}

	return purged, firstErr
}

// purgeTenant 删除框架自带的租户数据，并标记 tenants.deleted_at。
// 业务表的清理由业务实现（readme 1.4），在业务侧注册后由这里回调。
func (w *Worker) purgeTenant(ctx context.Context, tenantID string) error {
	scopeCtx := tenancy.WithScope(ctx, tenancy.Scope{
		TenantID: tenantID,
		Actor:    tenancy.ActorTenant,
	})

	err := w.tenant.Tx(scopeCtx, func(tx tenancy.Tx) error {
		for _, model := range []any{
			&userRoleRecord{},
			&roleRecord{},
			&configRecord{},
			&userRecord{},
		} {
			if deleteErr := tx.DeleteAll(model); deleteErr != nil {
				return deleteErr
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("purge tenant data: %w", err)
	}

	err = w.platform.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", tenantID).Update(&tenantRecord{}, map[string]any{
			"provision_step": "purged",
			"deleted_at":     w.now(),
		})
	})
	if err != nil {
		return fmt.Errorf("mark tenant purged: %w", err)
	}

	for _, cleaner := range w.opts.Cleaners {
		if cleanErr := cleaner.Purge(ctx, tenantID); cleanErr != nil {
			w.log.ErrorContext(ctx, "business cleaner failed", "tenant_id", tenantID, "err", cleanErr)
		}
	}

	return nil
}

func (w *Worker) acquireLock(ctx context.Context) bool {
	if w.client == nil {
		return true
	}

	ttl := w.opts.Interval * 2
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}

	ok, err := w.client.SetNX(ctx, maintenanceLockKey, w.lockValue, ttl).Result()
	if err != nil {
		w.log.WarnContext(ctx, "acquire maintenance lock failed", "err", err)
		return false
	}

	return ok
}

func (w *Worker) releaseLock(ctx context.Context) {
	if w.client == nil {
		return
	}

	script := redis.NewScript(`if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) end return 0`)
	_ = script.Run(ctx, w.client, []string{maintenanceLockKey}, w.lockValue).Err()
}

// 下列模型只用于清理，字段按需保留。
type tenantRecord struct {
	ID string `gorm:"column:id;primaryKey"`
}

func (tenantRecord) TableName() string { return "tenants" }

type userRecord struct {
	ID       string `gorm:"column:id;primaryKey"`
	TenantID string `gorm:"column:tenant_id"`
}

func (userRecord) TableName() string { return "users" }

type roleRecord struct {
	ID       string `gorm:"column:id;primaryKey"`
	TenantID string `gorm:"column:tenant_id"`
}

func (roleRecord) TableName() string { return "roles" }

type userRoleRecord struct {
	UserID   string `gorm:"column:user_id;primaryKey"`
	RoleID   string `gorm:"column:role_id"`
	TenantID string `gorm:"column:tenant_id"`
}

func (userRoleRecord) TableName() string { return "user_roles" }

type configRecord struct {
	TenantID  string `gorm:"column:tenant_id"`
	Namespace string `gorm:"column:namespace;primaryKey"`
	Key       string `gorm:"column:key;primaryKey"`
}

func (configRecord) TableName() string { return "tenant_configs" }
