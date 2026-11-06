// Package testutil 提供双租户隔离测试工具：起库、跑最小迁移、建租户、断言。
//
// 数据库来源优先级：
//  1. 环境变量 TEST_DATABASE_DSN（CI 或有本地 PG 时使用）；
//  2. 进程内 PostgreSQL（embedded-postgres，无需 Docker）。
//
// 两者都不可用时测试跳过，并提示如何提供 DSN。
package testutil

import (
	"errors"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

const (
	embeddedDatabase = "saas_test"
	embeddedUser     = "saas"
	embeddedPassword = "saas"
)

var (
	embeddedOnce   sync.Once
	embeddedDSN    string
	embeddedErr    error
	embeddedServer *embeddedpostgres.EmbeddedPostgres
	stopOnce       sync.Once
)

// DSN 返回可用的测试数据库连接串。
func DSN(t *testing.T) string {
	t.Helper()

	if dsn := os.Getenv("TEST_DATABASE_DSN"); dsn != "" {
		return dsn
	}

	embeddedOnce.Do(func() {
		runtimePath, err := os.MkdirTemp("", "embedded-pg-*")
		if err != nil {
			embeddedErr = err
			return
		}

		port, err := freePort()
		if err != nil {
			embeddedErr = err
			return
		}

		server := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
			Port(uint32(port)).
			Database(embeddedDatabase).
			Username(embeddedUser).
			Password(embeddedPassword).
			RuntimePath(runtimePath).
			StartTimeout(90 * time.Second))

		if startErr := server.Start(); startErr != nil {
			embeddedErr = startErr
			return
		}

		embeddedServer = server
		embeddedDSN = "postgres://" + embeddedUser + ":" + embeddedPassword +
			"@127.0.0.1:" + strconv.Itoa(port) + "/" + embeddedDatabase + "?sslmode=disable"
	})

	if embeddedErr != nil {
		t.Skipf("postgres unavailable (%v); set TEST_DATABASE_DSN to reuse an existing server", embeddedErr)
	}

	return embeddedDSN
}

// Stop 关闭进程内 PostgreSQL；在 TestMain 中调用。
func Stop() {
	stopOnce.Do(func() {
		if embeddedServer == nil {
			return
		}
		_ = embeddedServer.Stop()
	})
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = listener.Close() }()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("testutil: cannot resolve free tcp port")
	}

	return addr.Port, nil
}
