# Устройство `domain` и `infrastructure/repository`

## 1. Паттерн: Сущности и Value Objects со Smart Constructors (`internal/domain`)

В слое `domain` запрещены зависимости на внешние пакеты (JSON/SQL/gRPC-теги также не ставятся на доменные модели — за сериализацию отвечает `presenter` или `repository`).

### Правила проектирования домена:
1. **Сущность (Entity)**: чистая Go-структура, использующая строгие доменные типы вместо голых `string`/`int`.
2. **Value Object**: типизированный алиас примитива (`type Source string`, `type IPVersion int`) с набором допустимых констант.
3. **Smart Constructor**: функция `New<Type>(raw) (<Type>, error)`, которая гарантирует, что невалидное значение невозможно сконструировать.

### Пример сущности и Value Objects (`internal/domain/trusted_net`)

```go
// internal/domain/trusted_net/bot_net.go
package trustednet

import "time"

type TrustedBotNet struct {
	CIDR      string
	Source    Source
	Type      IPVersion
	CreatedAt time.Time
}
```

```go
// internal/domain/trusted_net/source.go
package trustednet

import (
	"fmt"
	"strings"
)

type Source string

const (
	Yandex        Source = "yandex"
	GoogleCommon  Source = "google"
	GoogleSpecial Source = "google_special_crawlers"
)

func NewSource(s string) (Source, error) {
	switch s {
	case string(Yandex):
		return Yandex, nil
	case string(GoogleCommon):
		return GoogleCommon, nil
	case string(GoogleSpecial):
		return GoogleSpecial, nil
	default:
		return Source(""), fmt.Errorf(
			"unexpected source `%s`, expected: `%s`",
			s,
			strings.Join([]string{string(Yandex), string(GoogleCommon), string(GoogleSpecial)}, "`,`"),
		)
	}
}
```

```go
// internal/domain/trusted_net/ip_version.go
package trustednet

import (
	"fmt"
	"strconv"
	"strings"
)

type IPVersion int

const (
	V4 IPVersion = 4
	V6 IPVersion = 6
)

func NewIPVersion(ipV int) (IPVersion, error) {
	switch ipV {
	case int(V4):
		return V4, nil
	case int(V6):
		return V6, nil
	default:
		return IPVersion(0), fmt.Errorf(
			"unexpected ip version `%d`, expected: `%s`",
			ipV,
			strings.Join([]string{strconv.Itoa(int(V4)), strconv.Itoa(int(V6))}, "`,`"),
		)
	}
}
```

---

## 2. Паттерн: SQL-репозиторий поверх `database/sql` (`internal/infrastructure/repository`)

Репозиторий инкапсулирует работу с реляционной базой данных без ORM.

### Ключевые приемы:
1. **Никаких интерфейсов в пакете репозитория**: репозиторий возвращает конкретный указатель `*TrustedNetRepository`. Узкие интерфейсы объявляются там, где репозиторий используется (`usecase`, `commands`).
2. **Динамическая сборка параметризованных запросов**: для `IN ($1, $2)` и батч-конструкций `VALUES ($2), ($3)` используется счетчик позиции аргумента `argPos`, что полностью защищает от SQL-инъекций.
3. **Восстановление доменных инвариантов при чтении**: значения, прочитанные через `rows.Scan` в сырые переменные (`srcRaw string`, `ipRaw int`), обязательно проходят через `trustednet.NewSource(srcRaw)` и `trustednet.NewIPVersion(ipRaw)`.
4. **Безопасное закрытие `sql.Rows`**: используется именованный возврат `(result []..., err error)` и проверка `rows.Close()` внутри `defer`, чтобы не потерять ошибку закрытия курсора, а также обязательная проверка `rows.Err()` после цикла `rows.Next()`.

### Пример `internal/infrastructure/repository/trusted_net.go`

```go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	trustednet "example.com/project/internal/domain/trusted_net"
)

type TrustedNetRepository struct {
	db *sql.DB
}

func NewTrustedNetRepository(db *sql.DB) *TrustedNetRepository {
	return &TrustedNetRepository{db: db}
}

func (r *TrustedNetRepository) Upsert(ctx context.Context, botNet trustednet.TrustedBotNet) error {
	const query = `
		WITH updated AS (
			UPDATE trusted_bot_nets
			SET ip_version = $3
			WHERE cidr = $1::cidr AND source = $2
			RETURNING id
		)
		INSERT INTO trusted_bot_nets (cidr, source, ip_version)
		SELECT $1::cidr, $2, $3
		WHERE NOT EXISTS (SELECT 1 FROM updated)
	`

	if _, err := r.db.ExecContext(ctx, query, botNet.CIDR, botNet.Source, int(botNet.Type)); err != nil {
		return fmt.Errorf("upsert trusted bot net: %w", err)
	}

	return nil
}

func (r *TrustedNetRepository) DeleteStale(
	ctx context.Context,
	source trustednet.Source,
	nets []trustednet.TrustedBotNet,
) error {
	if len(nets) == 0 {
		return errors.New("nets slice must not be empty")
	}

	args := make([]any, 0, 1+len(nets))
	args = append(args, source)
	valuesRows := make([]string, 0, len(nets))
	argPos := 2
	for _, net := range nets {
		valuesRows = append(valuesRows, fmt.Sprintf("($%d)", argPos))
		args = append(args, net.CIDR)
		argPos++
	}

	query := fmt.Sprintf(`
		DELETE FROM trusted_bot_nets t
		WHERE t.source = $1
		AND NOT EXISTS (
			SELECT 1
			FROM (VALUES %s) AS actual(cidr)
			WHERE t.cidr = actual.cidr::cidr
		)
	`, strings.Join(valuesRows, ","))
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("delete stale trusted bot nets: %w", err)
	}

	return nil
}

func (r *TrustedNetRepository) List(
	ctx context.Context,
	sources []trustednet.Source,
	ipV *trustednet.IPVersion,
) (result []trustednet.TrustedBotNet, err error) {
	query := "SELECT cidr, source, ip_version, created_at FROM trusted_bot_nets"
	var where []string
	var args []any
	argPos := 1

	if len(sources) > 0 {
		inSources := make([]string, 0, len(sources))
		for _, source := range sources {
			args = append(args, source)
			inSources = append(inSources, fmt.Sprintf("$%d", argPos))
			argPos++
		}
		where = append(where, fmt.Sprintf("source IN (%s)", strings.Join(inSources, ", ")))
	}
	if ipV != nil {
		where = append(where, fmt.Sprintf("ip_version = $%d", argPos))
		args = append(args, ipV)
	}
	if len(where) > 0 {
		query = fmt.Sprintf("%s WHERE %s", query, strings.Join(where, " AND "))
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query trusted bot nets: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil && err == nil {
			err = fmt.Errorf("close trusted bot nets rows: %w", cErr)
		}
	}()

	result = make([]trustednet.TrustedBotNet, 0)
	for rows.Next() {
		var (
			cidr      string
			srcRaw    string
			ipRaw     int
			createdAt time.Time
		)

		if err := rows.Scan(&cidr, &srcRaw, &ipRaw, &createdAt); err != nil {
			return nil, fmt.Errorf("scan trusted bot net row: %w", err)
		}
		src, err := trustednet.NewSource(srcRaw)
		if err != nil {
			return nil, fmt.Errorf("parse source from db value `%s`: %w", srcRaw, err)
		}
		ipVersion, err := trustednet.NewIPVersion(ipRaw)
		if err != nil {
			return nil, fmt.Errorf("parse ip version from db value `%d`: %w", ipRaw, err)
		}

		result = append(result, trustednet.TrustedBotNet{
			CIDR:      cidr,
			Source:    src,
			Type:      ipVersion,
			CreatedAt: createdAt,
		})
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trusted bot net rows: %w", err)
	}

	return result, nil
}
```
