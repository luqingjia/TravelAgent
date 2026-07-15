// Package database 负责建立和配置进程级 PostgreSQL 连接池。
//
// 这里只处理连接生命周期，不执行迁移，也不包含任何知识文档 SQL；具体查询属于业务适配器。
package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // 注册 database/sql 使用的 "pgx" 驱动名。
	"github.com/jmoiron/sqlx"

	"github.com/luqingjia/TravelAgent/internal/platform/config"
)

// Open 创建 sqlx 数据库句柄、设置连接池参数并立即 Ping PostgreSQL。
//
// sql.Open 本身只创建连接池对象，不代表数据库真的可用；因此启动阶段必须 PingContext，
// 让错误在开始监听 HTTP 之前暴露。错误信息不会拼接 DSN，避免把其中的密码写入日志。
func Open(ctx context.Context, configuration config.Database) (*sqlx.DB, error) {
	if strings.TrimSpace(configuration.DSN) == "" {
		return nil, fmt.Errorf("POSTGRESQL_DSN is required")
	}
	if configuration.MaxOpenConnections <= 0 {
		return nil, fmt.Errorf("POSTGRESQL_MAX_OPEN_CONNS must be positive")
	}
	if configuration.MaxIdleConnections < 0 ||
		configuration.MaxIdleConnections > configuration.MaxOpenConnections {
		return nil, fmt.Errorf("POSTGRESQL_MAX_IDLE_CONNS must be between 0 and max open connections")
	}
	if configuration.ConnMaxLifetime <= 0 {
		return nil, fmt.Errorf("POSTGRESQL_CONN_MAX_LIFETIME must be positive")
	}
	if configuration.ConnMaxIdleTime <= 0 {
		return nil, fmt.Errorf("POSTGRESQL_CONN_MAX_IDLE_TIME must be positive")
	}

	// database/sql 的连接池是并发安全的，整个进程只需要创建这一份，再由 app 组合根向下传递。
	rawDatabase, err := sql.Open("pgx", configuration.DSN)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL driver: %w", err)
	}

	// SetMaxOpenConns 限制同时占用的连接数；MaxIdle 保留热连接；两个 duration 控制连接定期回收。
	rawDatabase.SetMaxOpenConns(configuration.MaxOpenConnections)
	rawDatabase.SetMaxIdleConns(configuration.MaxIdleConnections)
	rawDatabase.SetConnMaxLifetime(configuration.ConnMaxLifetime)
	rawDatabase.SetConnMaxIdleTime(configuration.ConnMaxIdleTime)

	// sqlx.NewDb 只是在标准连接池外增加 Get/Select 等便利方法，不会另建一套连接。
	database := sqlx.NewDb(rawDatabase, "pgx")
	if err := database.PingContext(ctx); err != nil {
		// Ping 失败后必须关闭刚创建的池；否则反复启动测试或热重启会泄漏后台资源。
		_ = database.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	return database, nil
}
