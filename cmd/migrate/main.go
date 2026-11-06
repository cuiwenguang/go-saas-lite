package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/internal/platform/account"
	"github.com/cuiwenguang/go-saas-lite/internal/platform/tenants"
	"github.com/cuiwenguang/go-saas-lite/internal/security/identity"
	"github.com/cuiwenguang/go-saas-lite/migrations"
	"github.com/cuiwenguang/go-saas-lite/pkg/migrate"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

func main() {
	var (
		dsn        = flag.String("dsn", os.Getenv("DB_DSN"), "数据库 DSN")
		group      = flag.String("group", "all", "迁移组：platform | tenant | all")
		command    = flag.String("command", "up", "命令：up | status | verify | repair-identities")
		allTenants = flag.Bool("all-tenants", false, "Schema 模式：为每个租户 schema 执行租户迁移")
		syncRoles  = flag.Bool("sync-roles", false, "执行后同步内置角色权限")
		adminSeed  = flag.String("platform-admin", "", "初始化平台管理员，格式 email:password")
	)
	flag.Parse()

	if err := run(*dsn, *group, *command, *allTenants, *syncRoles, *adminSeed); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run(dsn, groupName, command string, allTenants, syncRoles bool, adminSeed string) error {
	if dsn == "" {
		return fmt.Errorf("dsn is required (use -dsn or DB_DSN)")
	}

	groups, err := resolveGroups(groupName)
	if err != nil {
		return err
	}

	db, err := tenancy.OpenPostgres(dsn, tenancy.Silent())
	if err != nil {
		return err
	}

	runner := migrate.New(db, migrations.FS)
	ctx := context.Background()

	if allTenants {
		return fmt.Errorf("schema-per-tenant is not enabled in this build; run -group tenant against the shared database")
	}

	if command == "repair-identities" {
		return repairIdentities(ctx, db, dsn)
	}

	for _, group := range groups {
		switch command {
		case "up":
			if err := runner.Up(ctx, group); err != nil {
				return err
			}
			fmt.Printf("applied %s migrations\n", group)
		case "verify":
			if err := runner.Verify(ctx, group); err != nil {
				return err
			}
			fmt.Printf("%s migrations verified\n", group)
		case "status":
			statuses, statusErr := runner.Status(ctx, group)
			if statusErr != nil {
				return statusErr
			}
			fmt.Printf("[%s]\n", group)
			for _, status := range statuses {
				mark := "pending"
				switch {
				case status.Applied && status.Drifted:
					mark = "drifted"
				case status.Applied:
					mark = "applied"
				}
				fmt.Printf("  v%-3d %-24s %s\n", status.Version, status.Filename, mark)
			}
		default:
			return fmt.Errorf("unknown command %q, want up|status|verify|repair-identities", command)
		}
	}

	if syncRoles {
		tenantGorm, openErr := tenancy.OpenPostgres(dsn, tenancy.WithTenantCallbacks(), tenancy.Silent())
		if openErr != nil {
			return openErr
		}

		synced, syncErr := tenants.SyncBuiltinRoles(ctx, tenancy.NewPlatformDB(db), tenancy.NewDB(tenantGorm))
		if syncErr != nil {
			return syncErr
		}
		fmt.Printf("synced builtin roles for %d tenants\n", synced)
	}

	if adminSeed != "" {
		parts := strings.SplitN(adminSeed, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("-platform-admin must be email:password")
		}
		if err := account.EnsureUser(ctx, tenancy.NewPlatformDB(db), parts[0], parts[1], account.RoleOwner); err != nil {
			return err
		}
		fmt.Printf("platform admin %s ensured\n", parts[0])
	}

	return nil
}

// repairIdentities 对账全局登录索引：补齐缺失行、清理指向不存在用户的脏行。
func repairIdentities(ctx context.Context, platformGorm *gorm.DB, dsn string) error {
	tenantGorm, err := tenancy.OpenPostgres(dsn, tenancy.WithTenantCallbacks(), tenancy.Silent())
	if err != nil {
		return err
	}

	service := identity.NewService(tenancy.NewPlatformDB(platformGorm), tenancy.NewDB(tenantGorm))
	report, err := service.Repair(ctx, 200)
	if err != nil {
		return err
	}

	fmt.Printf("identities repaired: tenants=%d scanned=%d added=%d removed=%d conflicts=%d\n",
		report.Tenants, report.Scanned, report.Added, report.Removed, report.Conflicts)

	return nil
}

func resolveGroups(name string) ([]migrate.Group, error) {
	if name == "all" {
		return migrate.Groups(), nil
	}

	group := migrate.Group(name)
	if !group.Valid() {
		return nil, fmt.Errorf("unknown group %q, want platform|tenant|all", name)
	}

	return []migrate.Group{group}, nil
}
