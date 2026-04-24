//go:build postgres || all_stores

// Package postgres implements store.Store using PostgreSQL.
// PostgreSQL による store.Store の実装。
package postgres

import (
	"fmt"

	"github.com/murlog-org/murlog/store"
)

func init() {
	store.Register("postgres", func(dsn string) (store.Store, error) {
		return New(dsn)
	})
}

// New creates a new PostgreSQL-backed store.
// PostgreSQL バックエンドの Store を生成する。
func New(dsn string) (store.Store, error) {
	return nil, fmt.Errorf("postgres: not implemented")
}
