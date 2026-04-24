//go:build mysql || all_stores

// Package mysql implements store.Store using MySQL.
// MySQL による store.Store の実装。
package mysql

import (
	"fmt"

	"github.com/murlog-org/murlog/store"
)

func init() {
	store.Register("mysql", func(dsn string) (store.Store, error) {
		return New(dsn)
	})
}

// New creates a new MySQL-backed store.
// MySQL バックエンドの Store を生成する。
func New(dsn string) (store.Store, error) {
	return nil, fmt.Errorf("mysql: not implemented")
}
