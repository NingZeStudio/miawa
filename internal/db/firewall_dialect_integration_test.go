package db

// 本文件是真实 MySQL/PostgreSQL 上的防火墙数据适配集成测试，
// 与 postgres_migrate_test.go 相同，需本机数据库并设置
// LEMWOOD_MIGRATION_INTEGRATION=1 才会运行（默认跳过）。
//
// 本地环境约定（见 AGENTS.md）：PostgreSQL 127.0.0.1:55432（信任本机回环），
// MySQL 127.0.0.1:33306（用户 lemwood / testpass，mysql_native_password）。
import (
	"os"
	"path/filepath"
	"testing"

	"lemwood_mirror/internal/config"
)

// 方言集成环境（与既有集成测试一致）。
func dialectIntegrationConfigs() map[string]*config.Config {
	user := getenv("USER")
	return map[string]*config.Config{
		"sqlite": {DatabaseMode: "sqlite"},
		"mysql": {
			DatabaseMode: "mysql",
			MySQLHost:    "127.0.0.1", MySQLPort: 33306, MySQLUser: "lemwood", MySQLPassword: "testpass", MySQLDatabase: "lemwood_fw_test",
		},
		"postgres": {
			DatabaseMode:    "pgsql",
			PostgresHost:    "127.0.0.1", PostgresPort: 55432, PostgresUser: user, PostgresDatabase: "lemwood_fw_test", PostgresSSLMode: "disable",
		},
	}
}

func closeTestDB() {
	if DB != nil {
		_ = DB.Close()
		DB = nil
	}
}

// 防火墙黑名单（含 CIDR 网段条目）在三种方言下的读写适配：
// ip_blacklist.ip 为 TEXT，CIDR 值按字符串精确存取；网段展开匹配在内存层（firewall 包）完成。
func TestBlacklistCIDRAcrossDialectsIntegration(t *testing.T) {
	if testing.Short() || getenv("LEMWOOD_MIGRATION_INTEGRATION") != "1" {
		t.Skip("set LEMWOOD_MIGRATION_INTEGRATION=1 to run local MySQL/PostgreSQL integration")
	}

	for name, cfg := range dialectIntegrationConfigs() {
		t.Run(name, func(t *testing.T) {
			closeTestDB()
			if err := InitDB(t.TempDir(), cfg); err != nil {
				t.Fatalf("InitDB(%s) error = %v", name, err)
			}
			defer closeTestDB()

			// 幂等：清掉历史数据
			if _, err := DB.Exec("DELETE FROM ip_blacklist"); err != nil {
				t.Fatalf("清空 ip_blacklist 失败: %v", err)
			}

			if err := AddIPToBlacklistWithSource("203.0.113.0/24", "网段封禁", "local", "manual"); err != nil {
				t.Fatalf("AddIPToBlacklistWithSource(CIDR) error = %v", err)
			}
			if err := AddIPToBlacklistWithSource("198.51.100.7", "精确 IP", "local", "traffic"); err != nil {
				t.Fatalf("AddIPToBlacklistWithSource(exact) error = %v", err)
			}

			// 重复写入走 upsert 分支，不应报错或产生重复行
			if err := AddIPToBlacklistWithSource("203.0.113.0/24", "网段封禁-更新", "local", "manual"); err != nil {
				t.Fatalf("AddIPToBlacklistWithSource(CIDR upsert) error = %v", err)
			}

			if !IsIPBlacklisted("203.0.113.0/24") {
				t.Fatal("CIDR 条目应按字符串精确命中")
			}
			if IsIPBlacklisted("203.0.113.55") {
				t.Fatal("IP 展开匹配不属于 DB 层职责（由 firewall 内存网段匹配承担）")
			}

			list, err := GetIPBlacklist()
			if err != nil {
				t.Fatalf("GetIPBlacklist() error = %v", err)
			}
			if len(list) != 2 {
				t.Fatalf("GetIPBlacklist() len = %d, want 2: %v", len(list), list)
			}
			for _, item := range list {
				if item["ip"] == "203.0.113.0/24" {
					if item["reason"] != "网段封禁-更新" || item["source"] != "local" || item["ban_type"] != "manual" {
						t.Fatalf("CIDR 条目 upsert 后字段不符: %v", item)
					}
				}
			}

			if err := RemoveIPFromBlacklist("203.0.113.0/24"); err != nil {
				t.Fatalf("RemoveIPFromBlacklist(CIDR) error = %v", err)
			}
			if IsIPBlacklisted("203.0.113.0/24") {
				t.Fatal("删除 CIDR 条目后不应再命中")
			}
		})
	}
}

// 黑名单服务端分页/过滤在三种方言下的适配：
// 关键词按字面匹配（_ 不是通配符，escapeLike + ESCAPE 子句），source 过滤语义一致。
func TestBlacklistPagedAcrossDialectsIntegration(t *testing.T) {
	if testing.Short() || getenv("LEMWOOD_MIGRATION_INTEGRATION") != "1" {
		t.Skip("set LEMWOOD_MIGRATION_INTEGRATION=1 to run local MySQL/PostgreSQL integration")
	}

	for name, cfg := range dialectIntegrationConfigs() {
		t.Run(name, func(t *testing.T) {
			closeTestDB()
			if err := InitDB(t.TempDir(), cfg); err != nil {
				t.Fatalf("InitDB(%s) error = %v", name, err)
			}
			defer closeTestDB()

			if _, err := DB.Exec("DELETE FROM ip_blacklist"); err != nil {
				t.Fatalf("清空 ip_blacklist 失败: %v", err)
			}
			entries := []struct{ ip, source, reason string }{
				{"192.0.2.101", "manual", "a_c"},     // 字面含下划线
				{"192.0.2.102", "manual", "abc"},     // 若 _ 被当通配符会被 "a_c" 误命中
				{"198.51.100.7", "external", "外部同步说明"},
				{"203.0.113.99", "local", "流量超限自动封禁"},
			}
			for _, e := range entries {
				if err := AddIPToBlacklistWithSource(e.ip, e.reason, e.source, "manual"); err != nil {
					t.Fatalf("seed %s: %v", e.ip, err)
				}
			}

			// 分页与总数
			items, total, err := GetIPBlacklistPaged(0, 2, "", "")
			if err != nil {
				t.Fatalf("GetIPBlacklistPaged error = %v", err)
			}
			if total != 4 || len(items) != 2 {
				t.Fatalf("分页结果不符: total=%d items=%d", total, len(items))
			}

			// source 过滤
			_, total, err = GetIPBlacklistPaged(0, 50, "local", "")
			if err != nil {
				t.Fatalf("source 过滤 error = %v", err)
			}
			if total != 1 {
				t.Fatalf("source=local 应 1 条, got %d", total)
			}

			// 字面关键词：_ 不是通配符，"a_c" 只命中自身，不能命中 "abc"
			_, total, err = GetIPBlacklistPaged(0, 50, "", "a_c")
			if err != nil {
				t.Fatalf("关键词过滤 error = %v", err)
			}
			if total != 1 {
				t.Fatalf("字面 a_c 应只命中 1 条（_ 不是通配符）: total=%d", total)
			}
			_, total, err = GetIPBlacklistPaged(0, 50, "", "192.0.2.10")
			if err != nil {
				t.Fatalf("IP 前缀过滤 error = %v", err)
			}
			if total != 2 {
				t.Fatalf("IP 前缀应命中 2 条, got %d", total)
			}

			// 统计
			counts, err := GetIPBlacklistSourceCounts()
			if err != nil {
				t.Fatalf("GetIPBlacklistSourceCounts error = %v", err)
			}
			if counts["all"] != 4 || counts["manual"] != 2 || counts["external"] != 1 || counts["local"] != 1 {
				t.Fatalf("统计不符: %v", counts)
			}
		})
	}
}

// SQLite → MySQL 一次性迁移（mysql_migration: true）应携带 CIDR 黑名单条目。
func TestSQLiteToMySQLMigrationCarriesCIDRBlacklistIntegration(t *testing.T) {
	if testing.Short() || getenv("LEMWOOD_MIGRATION_INTEGRATION") != "1" {
		t.Skip("set LEMWOOD_MIGRATION_INTEGRATION=1 to run local MySQL/PostgreSQL integration")
	}

	storage := t.TempDir()
	closeTestDB()
	if err := InitDB(storage, &config.Config{DatabaseMode: "sqlite"}); err != nil {
		t.Fatalf("InitDB(sqlite) error = %v", err)
	}
	if err := AddIPToBlacklistWithSource("203.0.113.0/24", "网段封禁", "local", "traffic"); err != nil {
		t.Fatalf("seed CIDR blacklist error = %v", err)
	}
	closeTestDB()

	cfg := dialectIntegrationConfigs()["mysql"]
	cfg.MySQLMigration = true
	if err := InitDB(storage, cfg); err != nil {
		t.Fatalf("InitDB(mysql with migration) error = %v", err)
	}
	defer closeTestDB()

	if !IsIPBlacklisted("203.0.113.0/24") {
		t.Fatal("CIDR 黑名单条目应随 SQLite→MySQL 迁移保留")
	}
	if _, err := os.Stat(filepath.Join(storage, "stats.db")); !os.IsNotExist(err) {
		t.Fatal("迁移完成后 stats.db 应被重命名为 .bak")
	}
}
