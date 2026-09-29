// Package store 是 v2 的本地持久化层：SQLite（modernc.org/sqlite，纯 Go，免 CGO）。
//
// 约定：
//   - WAL 模式 + busy_timeout，读写互不阻塞；
//   - 只开一个连接，写操作天然串行，避免 SQLITE_BUSY；
//   - 表结构变更一律新增 migrations/NNNN_xxx.sql，禁止修改已发布的迁移文件。
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Store 持有数据库连接。各领域的仓储方法（任务、媒体、预设等）按文件拆开挂在它上面。
type Store struct {
	db *sql.DB
}

// Open 打开（或创建）数据库并执行未应用的迁移。
func Open(ctx context.Context, dbPath string) (*Store, error) {
	dsn := "file:" + dbPath +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// DB 暴露底层连接，仅供同包外的仓储实现和测试使用。
func (s *Store) DB() *sql.DB { return s.db }

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	seen := map[int]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			return nil, fmt.Errorf("迁移文件名必须是 NNNN_描述.sql: %s", name)
		}
		v, err := strconv.Atoi(prefix)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("迁移文件版本号不合法: %s", name)
		}
		if prev, dup := seen[v]; dup {
			return nil, fmt.Errorf("迁移版本号重复: %s 和 %s", prev, name)
		}
		seen[v] = name
		body, err := migrationFS.ReadFile(path.Join("migrations", name))
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: name, sql: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("创建 schema_migrations 失败: %w", err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	current, err := s.SchemaVersion(ctx)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if err := s.applyMigration(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, m migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("执行迁移 %s 失败: %w", m.name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		m.version, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("记录迁移 %s 失败: %w", m.name, err)
	}
	return tx.Commit()
}

// SchemaVersion 返回已应用的最高迁移版本，全新数据库返回 0。
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, fmt.Errorf("读取迁移版本失败: %w", err)
	}
	return int(v.Int64), nil
}

// MarkInterrupted 在启动时调用：把上次没跑完（queued / running）的任务标成 interrupted，
// 同时递增 version，让前端能按版本号判断新旧。返回受影响的任务数。
func (s *Store) MarkInterrupted(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'interrupted',
		    version = version + 1,
		    finished_at = ?
		WHERE status IN ('queued', 'running')`, now.UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("标记中断任务失败: %w", err)
	}
	return res.RowsAffected()
}
