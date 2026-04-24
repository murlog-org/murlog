//go:build all_stores

// Package all imports all store drivers for an all-in-one build.
// 全ストアドライバを import する全部入りビルド用パッケージ。
package all

import (
	_ "github.com/murlog-org/murlog/store/mysql"
	_ "github.com/murlog-org/murlog/store/postgres"
	_ "github.com/murlog-org/murlog/store/sqlite"
)
