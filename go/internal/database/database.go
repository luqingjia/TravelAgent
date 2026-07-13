package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func Open(ctx context.Context, dsn string) (*sqlx.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("POSTGRESQL_DSN is empty")
	}
	raw, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	raw.SetMaxOpenConns(10)
	raw.SetMaxIdleConns(5)
	raw.SetConnMaxLifetime(30 * time.Minute)

	db := sqlx.NewDb(raw, "pgx")
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
