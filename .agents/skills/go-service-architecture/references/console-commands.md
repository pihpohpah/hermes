# Устройство консольных команд (`cmd/<app>/console.go` и `internal/application/commands`)

## 1. Паттерн: Диспетчер консольных команд (`cmd/<app>/console.go`)

Фоновые задачи (обновление справочников, синхронизация с внешними API, периодические задачи Kubernetes `CronJob`) запускаются через единый интерфейс `Command`:

```go
type Command interface {
	Run(ctx context.Context) error
}
```

Диспетчер `runCommand` сопоставляет имя команды из `os.Args[1]` с конкретной структурой из `internal/application/commands`, передает ей зависимости (`logger`, `repo`) и вызывает `command.Run(ctx)`.

### Пример `cmd/<app>/console.go`

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"example.com/project/internal/application/commands"
	"example.com/project/internal/infrastructure/repository"
)

var ErrUnknownCommand = errors.New("unknown command")

const (
	yandexBotListUpdateCommand        = "yandex_bot_list_update_command"
	googleCommonBotListUpdateCommand  = "google_bot_list_update_command"
	googleSpecialBotListUpdateCommand = "google_special_bot_list_update_command"
)

type Command interface {
	Run(ctx context.Context) error
}

func runCommand(
	ctx context.Context,
	commandName string,
	logger *slog.Logger,
	repo *repository.TrustedNetRepository,
) error {
	var command Command
	switch commandName {
	case yandexBotListUpdateCommand:
		command = commands.NewYandexBotListUpdate(logger, repo)
	case googleCommonBotListUpdateCommand:
		command = commands.NewGoogleCommonBotsUpdate(logger, repo)
	case googleSpecialBotListUpdateCommand:
		command = commands.NewGoogleSpecialBotsUpdate(logger, repo)
	default:
		return ErrUnknownCommand
	}

	if err := command.Run(ctx); err != nil {
		return fmt.Errorf("run command: %w", err)
	}

	return nil
}
```

---

## 2. Паттерн: Переиспользование логики через встраивание (`internal/application/commands`)

Если несколько команд выполняют одинаковый финальный шаг (например, сохранение списка сущностей в БД и удаление устаревших записей), эта логика выносится в неэкспортируемую структуру со своим узким интерфейсом репозитория и встраивается (struct embedding) в конкретные команды.

### 2.1. Базовая структура синхронизации с БД (`bot_list_update_db_sync.go`)

```go
package commands

import (
	"context"
	"fmt"
	"log/slog"

	trustednet "example.com/project/internal/domain/trusted_net"
)

type botListUpdateDBSync struct {
	logger *slog.Logger
	repo   netListSyncRepo
}

func newBotListUpdateDBSync(logger *slog.Logger, repo netListSyncRepo) botListUpdateDBSync {
	return botListUpdateDBSync{logger, repo}
}

type netListSyncRepo interface {
	Upsert(ctx context.Context, botNet trustednet.TrustedBotNet) error
	DeleteStale(ctx context.Context, source trustednet.Source, cidrs []trustednet.TrustedBotNet) error
}

func (s botListUpdateDBSync) syncSourceBotNets(
	ctx context.Context,
	source trustednet.Source,
	botNets []trustednet.TrustedBotNet,
) error {
	for _, botNet := range botNets {
		if err := s.repo.Upsert(ctx, botNet); err != nil {
			return fmt.Errorf("upsert %s bot net `%s`: %w", source, botNet.CIDR, err)
		}
	}

	if err := s.repo.DeleteStale(ctx, source, botNets); err != nil {
		return fmt.Errorf("delete stale %s bot nets: %w", source, err)
	}
	s.logger.Info("bot nets updated", "source", source, "count", len(botNets))

	return nil
}
```

### 2.2. Конкретная команда со встроенным синхронизатором (`google_common_bot_list_update.go`)

```go
package commands

import (
	"context"
	"log/slog"

	trustednet "example.com/project/internal/domain/trusted_net"
)

type GoogleCommonBotListUpdate struct {
	botListUpdateDBSync
}

func NewGoogleCommonBotsUpdate(
	logger *slog.Logger,
	repo netListSyncRepo,
) GoogleCommonBotListUpdate {
	return GoogleCommonBotListUpdate{
		botListUpdateDBSync: newBotListUpdateDBSync(logger, repo),
	}
}

func (c GoogleCommonBotListUpdate) Run(ctx context.Context) error {
	const url = "https://developers.google.com/crawling/ipranges/common-crawlers.json"

	botNets, err := getBotNetworks(ctx, c.logger, url, trustednet.GoogleCommon)
	if err != nil {
		return err
	}

	return c.syncSourceBotNets(ctx, trustednet.GoogleCommon, botNets)
}
```

---

## 3. Паттерн: Запрос во внешний источник, валидация и дедупликация (`get_google_bot_networks.go`)

При загрузке данных из внешних источников в командах соблюдаются правила:
1. Явный `context.WithTimeout` на внешний HTTP-запрос.
2. Установка понятного `User-Agent`.
3. Проверка `StatusCode == http.StatusOK` и логирование ошибки закрытия `r.Body.Close()` в `defer`.
4. Валидация каждого элемента (например, `net.ParseCIDR`) и дедупликация через `seen := make(map[string]struct{})`.
5. Защита от пустого ответа (`if len(botNets) == 0`), чтобы случайно не удалить все записи при сбое парсинга на стороне источника.

```go
package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	trustednet "example.com/project/internal/domain/trusted_net"
)

var (
	errGoogleBotsHTTPRequest = errors.New("google bots update: http request")
	errGoogleBotsReadBody    = errors.New("google bots update: read response body")
	errGoogleBotsUnmarshal   = errors.New("google bots update: decode response")
)

const googleIPsRequestTimeout = 30 * time.Second

type googlePrefixEntry struct {
	IPv6Prefix string `json:"ipv6Prefix"`
	IPv4Prefix string `json:"ipv4Prefix"`
}

type googleResponseDto struct {
	CreationTime string              `json:"creationTime"`
	Prefixes     []googlePrefixEntry `json:"prefixes"`
}

func getBotNetworks(
	ctx context.Context,
	logger *slog.Logger,
	url string,
	source trustednet.Source,
) ([]trustednet.TrustedBotNet, error) {
	ctx, cancel := context.WithTimeout(ctx, googleIPsRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errGoogleBotsHTTPRequest, err)
	}
	req.Header.Set("User-Agent", "bot-keeper/1.0")

	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errGoogleBotsHTTPRequest, err)
	}
	defer func() {
		if cErr := r.Body.Close(); cErr != nil {
			logger.Warn("close google crawlers response body", "error", cErr)
		}
	}()
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected status code %d", errGoogleBotsHTTPRequest, r.StatusCode)
	}

	responseBody, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errGoogleBotsReadBody, err)
	}

	var response googleResponseDto
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, fmt.Errorf("%w: %w", errGoogleBotsUnmarshal, err)
	}

	botNets, err := parseGooglePrefixes(response.Prefixes, source)
	if err != nil {
		return nil, fmt.Errorf("parse google prefixes: %w", err)
	}
	if len(botNets) == 0 {
		return nil, errors.New("parsed google bot nets are empty")
	}

	return botNets, nil
}

func parseGooglePrefixes(prefixes []googlePrefixEntry, source trustednet.Source) ([]trustednet.TrustedBotNet, error) {
	result := make([]trustednet.TrustedBotNet, 0, len(prefixes))
	seen := make(map[string]struct{}, len(prefixes))

	for _, prefix := range prefixes {
		if prefix.IPv4Prefix != "" {
			if _, _, err := net.ParseCIDR(prefix.IPv4Prefix); err != nil {
				return nil, fmt.Errorf("invalid google IPv4 prefix `%s`: %w", prefix.IPv4Prefix, err)
			}
			if _, exists := seen[prefix.IPv4Prefix]; !exists {
				result = append(result, trustednet.TrustedBotNet{
					CIDR:   prefix.IPv4Prefix,
					Source: source,
					Type:   trustednet.V4,
				})
				seen[prefix.IPv4Prefix] = struct{}{}
			}
		}

		if prefix.IPv6Prefix != "" {
			if _, _, err := net.ParseCIDR(prefix.IPv6Prefix); err != nil {
				return nil, fmt.Errorf("invalid google IPv6 prefix `%s`: %w", prefix.IPv6Prefix, err)
			}
			if _, exists := seen[prefix.IPv6Prefix]; !exists {
				result = append(result, trustednet.TrustedBotNet{
					CIDR:   prefix.IPv6Prefix,
					Source: source,
					Type:   trustednet.V6,
				})
				seen[prefix.IPv6Prefix] = struct{}{}
			}
		}
	}

	return result, nil
}
```
