package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5/pgconn"
)

type sqlExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// MigrateCommand накатывает migrations/*.sql по порядку имён.
//
// Миграции идемпотентны (IF NOT EXISTS, ON CONFLICT), поэтому журнала версий
// нет: каждый запуск применяет все файлы заново. Работает с любой базой из
// DATABASE_URL — psql и docker exec не нужны.
type MigrateCommand struct {
	logger *slog.Logger
	db     sqlExecutor
	dir    string
}

func NewMigrateCommand(logger *slog.Logger, db sqlExecutor, dir string) *MigrateCommand {
	return &MigrateCommand{logger: logger, db: db, dir: dir}
}

func (c *MigrateCommand) Run(ctx context.Context) error {
	files, err := filepath.Glob(filepath.Join(c.dir, "*.sql"))
	if err != nil {
		return fmt.Errorf("list migrations in %s: %w", c.dir, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no migrations found in %s", c.dir)
	}
	sort.Strings(files)

	for _, file := range files {
		body, readErr := os.ReadFile(file)
		if readErr != nil {
			return fmt.Errorf("read migration %s: %w", file, readErr)
		}
		// Без аргументов pgx шлёт запрос простым протоколом: в одном Exec
		// можно выполнить весь файл с несколькими statement-ами.
		if _, execErr := c.db.Exec(ctx, string(body)); execErr != nil {
			return fmt.Errorf("apply migration %s: %w", file, execErr)
		}
		c.logger.Info("migration applied", slog.String("file", file))
	}

	return nil
}
