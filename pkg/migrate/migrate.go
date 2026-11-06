// Package migrate 负责平台组与租户组的迁移执行、状态查询与 checksum 校验。
//
// 职责边界：
//   - 执行交给 goose；
//   - checksum 由本包维护（goose 的版本表不含 checksum），落 `schema_migrations_checksum`；
//   - 两个组各用一张版本表，避免单库下版本号互相污染。
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

// Group 是迁移组。
type Group string

const (
	// Platform 是平台组（tenants、plans 等公共表）。
	Platform Group = "platform"
	// Tenant 是租户组（业务表）。
	Tenant Group = "tenant"
)

// Valid 判断组名是否合法。
func (g Group) Valid() bool {
	return g == Platform || g == Tenant
}

// Groups 返回全部迁移组。
func Groups() []Group {
	return []Group{Platform, Tenant}
}

var versionTables = map[Group]string{
	Platform: "schema_migrations_platform",
	Tenant:   "schema_migrations_tenant",
}

const checksumTable = "schema_migrations_checksum"

// goose 使用全局状态，进程内串行执行。
var gooseMu sync.Mutex

// Migration 是一个脚本文件的元信息。
type Migration struct {
	Version  int64
	Filename string
	Checksum string
}

// Status 是一条迁移的执行状态。
type Status struct {
	Migration
	Applied bool
	Drifted bool
}

// Runner 执行指定文件系统上的迁移。
type Runner struct {
	gorm *gorm.DB
	fsys fs.FS
}

// New 创建执行器；fsys 需包含 <group>/*.sql。
func New(db *gorm.DB, fsys fs.FS) *Runner {
	return &Runner{gorm: db, fsys: fsys}
}

// List 解析某组脚本，按版本排序。
func (r *Runner) List(group Group) ([]Migration, error) {
	if !group.Valid() {
		return nil, fmt.Errorf("migrate: unknown group %q", group)
	}

	entries, err := fs.ReadDir(r.fsys, string(group))
	if err != nil {
		return nil, fmt.Errorf("migrate: read %s scripts: %w", group, err)
	}

	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, ok := parseVersion(entry.Name())
		if !ok {
			return nil, fmt.Errorf("migrate: %s/%s must start with numeric version", group, entry.Name())
		}

		content, readErr := fs.ReadFile(r.fsys, path.Join(string(group), entry.Name()))
		if readErr != nil {
			return nil, fmt.Errorf("migrate: read %s/%s: %w", group, entry.Name(), readErr)
		}

		sum := sha256.Sum256(content)
		migrations = append(migrations, Migration{
			Version:  version,
			Filename: entry.Name(),
			Checksum: hex.EncodeToString(sum[:]),
		})
	}

	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })

	return migrations, nil
}

// advisoryLockID 是迁移的跨进程锁，避免多实例同时建表。
const advisoryLockID int64 = 8127312

// lock 获取 PostgreSQL advisory lock，返回释放函数。
func (r *Runner) lock(ctx context.Context) (func(), error) {
	sqlDB, err := r.gorm.DB()
	if err != nil {
		return nil, fmt.Errorf("migrate: get sql.DB: %w", err)
	}

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: acquire connection: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", advisoryLockID); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("migrate: acquire lock: %w", err)
	}

	return func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryLockID)
		_ = conn.Close()
	}, nil
}

// Up 校验后执行某组迁移，并记录 checksum。
func (r *Runner) Up(ctx context.Context, group Group) error {
	if !group.Valid() {
		return fmt.Errorf("migrate: unknown group %q", group)
	}

	if err := r.Verify(ctx, group); err != nil {
		return err
	}

	unlock, err := r.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	sqlDB, err := r.gorm.DB()
	if err != nil {
		return fmt.Errorf("migrate: get sql.DB: %w", err)
	}

	gooseMu.Lock()
	defer gooseMu.Unlock()

	goose.SetBaseFS(r.fsys)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrate: set dialect: %w", err)
	}
	goose.SetTableName(versionTables[group])

	if err := goose.UpContext(ctx, sqlDB, string(group)); err != nil {
		return fmt.Errorf("migrate: apply %s migrations: %w", group, err)
	}

	return r.recordChecksums(ctx, group)
}

// Verify 校验已执行脚本的 checksum 是否与文件一致。
// 尚未记录 checksum 的历史版本不报错，由下一次 Up 补齐基线。
func (r *Runner) Verify(ctx context.Context, group Group) error {
	applied, err := r.appliedVersions(ctx, group)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		return nil
	}

	migrations, err := r.List(group)
	if err != nil {
		return err
	}

	byVersion := make(map[int64]Migration, len(migrations))
	for _, migration := range migrations {
		byVersion[migration.Version] = migration
	}

	stored, err := r.storedChecksums(ctx, group)
	if err != nil {
		return err
	}

	for version := range applied {
		migration, ok := byVersion[version]
		if !ok {
			return fmt.Errorf("migrate: %s migration v%d is applied but the script file is missing", group, version)
		}

		sum, ok := stored[version]
		if !ok {
			continue
		}
		if sum != migration.Checksum {
			return fmt.Errorf("migrate: %s migration v%d (%s) checksum mismatch: applied=%s current=%s",
				group, version, migration.Filename, sum, migration.Checksum)
		}
	}

	return nil
}

// Status 返回某组脚本的执行状态。
func (r *Runner) Status(ctx context.Context, group Group) ([]Status, error) {
	migrations, err := r.List(group)
	if err != nil {
		return nil, err
	}

	applied, err := r.appliedVersions(ctx, group)
	if err != nil {
		return nil, err
	}

	stored, err := r.storedChecksums(ctx, group)
	if err != nil {
		return nil, err
	}

	statuses := make([]Status, 0, len(migrations))
	for _, migration := range migrations {
		_, isApplied := applied[migration.Version]
		sum, hasSum := stored[migration.Version]

		statuses = append(statuses, Status{
			Migration: migration,
			Applied:   isApplied,
			Drifted:   hasSum && sum != migration.Checksum,
		})
	}

	return statuses, nil
}

func (r *Runner) recordChecksums(ctx context.Context, group Group) error {
	if err := r.ensureChecksumTable(ctx); err != nil {
		return err
	}

	migrations, err := r.List(group)
	if err != nil {
		return err
	}

	applied, err := r.appliedVersions(ctx, group)
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if _, ok := applied[migration.Version]; !ok {
			continue
		}

		if err := r.gorm.WithContext(ctx).Exec(
			`INSERT INTO `+checksumTable+` (group_name, version, filename, checksum)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT (group_name, version) DO NOTHING`,
			string(group), migration.Version, migration.Filename, migration.Checksum,
		).Error; err != nil {
			return fmt.Errorf("migrate: record checksum for %s v%d: %w", group, migration.Version, err)
		}
	}

	return nil
}

func (r *Runner) ensureChecksumTable(ctx context.Context) error {
	return r.gorm.WithContext(ctx).Exec(`
CREATE TABLE IF NOT EXISTS ` + checksumTable + ` (
    group_name varchar(32)  NOT NULL,
    version    bigint       NOT NULL,
    filename   varchar(255) NOT NULL,
    checksum   varchar(64)  NOT NULL,
    applied_at timestamptz  NOT NULL DEFAULT now(),
    PRIMARY KEY (group_name, version)
)`).Error
}

func (r *Runner) appliedVersions(ctx context.Context, group Group) (map[int64]struct{}, error) {
	table := versionTables[group]

	exists, err := r.tableExists(ctx, table)
	if err != nil {
		return nil, err
	}
	if !exists {
		return map[int64]struct{}{}, nil
	}

	var versions []int64
	if err := r.gorm.WithContext(ctx).
		Raw(`SELECT version_id FROM ` + table + ` WHERE is_applied`).
		Scan(&versions).Error; err != nil {
		return nil, fmt.Errorf("migrate: read %s: %w", table, err)
	}

	applied := make(map[int64]struct{}, len(versions))
	for _, version := range versions {
		// goose 会写入一条 version 0 的基线记录，不对应任何脚本。
		if version <= 0 {
			continue
		}
		applied[version] = struct{}{}
	}

	return applied, nil
}

func (r *Runner) storedChecksums(ctx context.Context, group Group) (map[int64]string, error) {
	exists, err := r.tableExists(ctx, checksumTable)
	if err != nil {
		return nil, err
	}
	if !exists {
		return map[int64]string{}, nil
	}

	var rows []struct {
		Version  int64
		Checksum string
	}
	if err := r.gorm.WithContext(ctx).
		Raw(`SELECT version, checksum FROM `+checksumTable+` WHERE group_name = ?`, string(group)).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("migrate: read %s: %w", checksumTable, err)
	}

	stored := make(map[int64]string, len(rows))
	for _, row := range rows {
		stored[row.Version] = row.Checksum
	}

	return stored, nil
}

func (r *Runner) tableExists(ctx context.Context, table string) (bool, error) {
	var exists bool
	err := r.gorm.WithContext(ctx).
		Raw(`SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_name = ?
		)`, table).
		Scan(&exists).Error
	if err != nil {
		return false, fmt.Errorf("migrate: check table %s: %w", table, err)
	}

	return exists, nil
}

func parseVersion(filename string) (int64, bool) {
	base := strings.TrimSuffix(filename, ".sql")
	index := strings.Index(base, "_")
	if index <= 0 {
		return 0, false
	}

	version, err := strconv.ParseInt(base[:index], 10, 64)
	if err != nil {
		return 0, false
	}

	return version, true
}

// ErrNothingToDo 表示没有待执行的迁移。
var ErrNothingToDo = errors.New("migrate: nothing to do")
