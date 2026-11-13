# Устройство `usecase`, `presenter` и `routes` (HTTP + gRPC)

## 1. Паттерн: UseCase с Consumer-Driven интерфейсом (`internal/application/usecase`)

Каждый сценарий использования оформляется отдельной структурой с методом `Do(ctx, ...)`.
Зависимость на хранилище описывается неэкспортируемым интерфейсом `repo` прямо в файле сценария и содержит только те методы, которые нужны этому сценарию.

### Пример `internal/application/usecase/get_bots.go`

```go
package usecase

import (
	"context"

	trustednet "example.com/project/internal/domain/trusted_net"
)

type GetBotsUseCase struct {
	repo repo
}

func NewGetBotsUseCase(repo repo) *GetBotsUseCase {
	return &GetBotsUseCase{repo}
}

type repo interface {
	List(
		ctx context.Context,
		source []trustednet.Source,
		ipV *trustednet.IPVersion,
	) ([]trustednet.TrustedBotNet, error)
}

func (u *GetBotsUseCase) Do(
	ctx context.Context,
	sources []trustednet.Source,
	ipV *trustednet.IPVersion,
) (result []trustednet.TrustedBotNet, err error) {
	return u.repo.List(ctx, sources, ipV)
}
```

---

## 2. Паттерн: Presenter / DTO Mapper (`internal/application/presenter`)

Доменные модели не содержат тегов сериализации (`json` или `protobuf`). За преобразование доменных структур в структуры ответа для HTTP и gRPC отвечает пакет `presenter`.

### Пример `internal/application/presenter/bots.go`

```go
package presenter

import (
	"fmt"

	servicev1 "example.com/project-proto/proto/v1"

	trustednet "example.com/project/internal/domain/trusted_net"
)

type TrustedBotNet struct {
	CIDR   string               `json:"cidr"`
	Source trustednet.Source    `json:"source"`
	Type   trustednet.IPVersion `json:"type"`
}

func TrustedBotNetsToHTTP(in []trustednet.TrustedBotNet) []TrustedBotNet {
	out := make([]TrustedBotNet, len(in))
	for i := range in {
		out[i] = TrustedBotNet{
			CIDR:   in[i].CIDR,
			Source: in[i].Source,
			Type:   in[i].Type,
		}
	}

	return out
}

func TrustedBotNetsToProto(in []trustednet.TrustedBotNet) ([]*servicev1.TrustedBotNet, error) {
	out := make([]*servicev1.TrustedBotNet, 0, len(in))
	for i := range in {
		item, err := trustedBotNetToProto(in[i])
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}

	return out, nil
}

func trustedBotNetToProto(n trustednet.TrustedBotNet) (*servicev1.TrustedBotNet, error) {
	var src servicev1.Source
	switch n.Source {
	case trustednet.Yandex:
		src = servicev1.Source_SOURCE_YANDEX
	case trustednet.GoogleCommon:
		src = servicev1.Source_SOURCE_GOOGLE
	case trustednet.GoogleSpecial:
		src = servicev1.Source_SOURCE_GOOGLE_SPECIAL_CRAWLERS
	default:
		return nil, fmt.Errorf("unknown trusted bot source: %s", n.Source)
	}

	var ipV servicev1.IPVersion
	switch n.Type {
	case trustednet.V4:
		ipV = servicev1.IPVersion_IP_VERSION_V4
	case trustednet.V6:
		ipV = servicev1.IPVersion_IP_VERSION_V6
	default:
		return nil, fmt.Errorf("unknown trusted bot ip version: %d", n.Type)
	}

	return &servicev1.TrustedBotNet{
		Cidr:      n.CIDR,
		Source:    src,
		IpVersion: ipV,
	}, nil
}
```

---

## 3. Паттерн: Определение HTTP-роутера (`internal/application/routes/router.go`)

Роутер строится на стандартном `http.ServeMux` (Go `net/http`).
В нем регистрируются инфраструктурные эндпоинты (`/health`, `/metrics`, `/swagger/`, `net/http/pprof` через blank import) и бизнес-хендлеры, реализующие интерфейс `http.Handler` (`ServeHTTP`).

### Пример `internal/application/routes/router.go`

```go
package routes

import (
	"net/http"
	_ "net/http/pprof" // trigger init

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/swaggo/http-swagger/v2"

	httproutes "example.com/project/internal/application/routes/http"
)

func NewHTTPRouter(
	getBotsHandler *httproutes.GetBotsHandler,
	healthCheckHandler *httproutes.HealthCheckHandler,
) *http.ServeMux {
	mux := http.DefaultServeMux

	mux.Handle("/health", healthCheckHandler)
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/swagger/", httpSwagger.Handler())

	mux.Handle("/api/v1/bots", getBotsHandler)

	return mux
}
```

---

## 4. Паттерн: Generic Response Envelope и HTTP-хендлеры (`internal/application/routes/http`)

### 4.1. Единый формат HTTP-ответа (`common.go`)
Все HTTP-ручки возвращают единую JSON-обертку `APIResponse[T]`:
```json
{
  "success": true,
  "error": "",
  "data": [...]
}
```
Для записи ответа и логирования ошибок используется параметризованный дженерик-тип `APIResponseWriter[data any]`.

```go
// internal/application/routes/http/common.go
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type APIResponse[data any] struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Data    data   `json:"data"`
}

type APIResponseWriter[data any] struct {
	logger *slog.Logger
}

func NewAPIResponseWriter[data any](logger *slog.Logger) *APIResponseWriter[data] {
	return &APIResponseWriter[data]{
		logger: logger,
	}
}

func (a *APIResponseWriter[data]) Error(writer http.ResponseWriter, code int, err error) {
	a.logger.Error("request failed", "error", err)

	writer.Header().Add("Content-Type", "application/json")
	writer.WriteHeader(code)

	if err := json.NewEncoder(writer).Encode(APIResponse[any]{false, err.Error(), nil}); err != nil {
		a.logger.Error("json encode of error response failed", "error", err)
	}
}

func (a *APIResponseWriter[data]) Success(writer http.ResponseWriter, d data) {
	r := APIResponse[data]{
		Success: true,
		Error:   "",
		Data:    d,
	}

	writer.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(r); err != nil {
		a.logger.Error("json encode of success response failed", "error", err)
	}
}
```

### 4.2. Бизнес HTTP-хендлер (`bots.go`) и HealthCheck (`health_check.go`)
Каждый HTTP-хендлер:
1. Хранит типизированный `*APIResponseWriter[ResponseDTO]` и локальный интерфейс `UseCase`.
2. Реализует `ServeHTTP(w http.ResponseWriter, r *http.Request)` и документируется Swagger-комментариями.
3. Парсит и валидирует входные параметры через конструкторы `domain` (`NewIPVersion`, `NewSource`), при ошибке возвращая `http.StatusBadRequest`.
4. Вызывает `useCase.Do(r.Context(), ...)`, при ошибке возвращая `http.StatusInternalServerError`.
5. Маппит результат через `presenter` и отправляет через `responseWriter.Success`.

```go
// internal/application/routes/http/bots.go
package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"example.com/project/internal/application/presenter"
	trustednet "example.com/project/internal/domain/trusted_net"
)

type GetBotsHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[[]presenter.TrustedBotNet]
	useCase        GetBotsUC
}

func NewGetBotsHandler(
	logger *slog.Logger,
	uc GetBotsUC,
) *GetBotsHandler {
	return &GetBotsHandler{logger, NewAPIResponseWriter[[]presenter.TrustedBotNet](logger), uc}
}

type GetBotsUC interface {
	Do(ctx context.Context, source []trustednet.Source, ipV *trustednet.IPVersion) ([]trustednet.TrustedBotNet, error)
}

// ServeHTTP handles GET /api/v1/bots and returns trusted bot CIDRs for the requested sources.
//
// @Summary		Returns list of search bots IP
// @Description	Returns list of search bots IP
// @Tags		sessions
// @Param source	query	[]trustednet.Source	false	"источники подсеток"	collectionFormat(multi)
// @Param ipV 		query	int					false	"ip version"
// @Produce		json
// @Success		200	{object}	APIResponse[[]presenter.TrustedBotNet]
// @Failure		400	{object}	APIResponse[any]
// @Failure		500	{object}	APIResponse[any]
// @Router		/api/v1/bots [get]
func (h *GetBotsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusBadRequest, errors.New("method GET expected"))

		return
	}

	var ipV *trustednet.IPVersion
	sources := make([]trustednet.Source, 0)
	if ipVString := r.URL.Query().Get("ipV"); ipVString != "" {
		v, err := strconv.Atoi(ipVString)
		if err != nil {
			h.responseWriter.Error(w, http.StatusBadRequest, errors.New("ipV expected as integer"))

			return
		}
		version, err := trustednet.NewIPVersion(v)
		if err != nil {
			h.responseWriter.Error(w, http.StatusBadRequest, err)

			return
		}
		ipV = &version
	}

	if r.URL.Query().Has("source") {
		for _, s := range r.URL.Query()["source"] {
			s, err := trustednet.NewSource(s)
			if err != nil {
				h.responseWriter.Error(w, http.StatusBadRequest, err)

				return
			}
			sources = append(sources, s)
		}
	}

	nets, err := h.useCase.Do(r.Context(), sources, ipV)
	if err != nil {
		h.responseWriter.Error(w, http.StatusInternalServerError, err)

		return
	}

	h.responseWriter.Success(w, presenter.TrustedBotNetsToHTTP(nets))
}
```

```go
// internal/application/routes/http/health_check.go
package http

import (
	"database/sql"
	"log/slog"
	"net/http"
)

type HealthCheckHandler struct {
	logger         *slog.Logger
	db             *sql.DB
	responseWriter *APIResponseWriter[any]
}

func NewHealthCheckHandler(
	logger *slog.Logger,
	db *sql.DB,
) *HealthCheckHandler {
	return &HealthCheckHandler{logger, db, NewAPIResponseWriter[any](logger)}
}

func (h *HealthCheckHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.db.PingContext(r.Context()); err != nil {
		h.responseWriter.Error(w, http.StatusInternalServerError, err)

		return
	}

	h.responseWriter.Success(w, nil)
}
```

---

## 5. Паттерн: Определение gRPC-сервера (`internal/application/routes/grpc`)

gRPC-сервер переиспользует тот же `UseCase`, что и HTTP-хендлер:
1. Встраивает `Unimplemented<Service>Server` для прямой совместимости контрактов.
2. Объявляет собственный интерфейс `GetBotsUC`.
3. Преобразует proto-enum в доменные Value Objects (`trustednet.NewSource`, `trustednet.NewIPVersion`), возвращая `status.Error(codes.InvalidArgument, ...)` при некорректных входных данных.
4. Вызывает `s.uc.Do(ctx, ...)` и преобразует результат через `presenter.TrustedBotNetsToProto(nets)`.

### Пример `internal/application/routes/grpc/bots_server.go`

```go
package grpc

import (
	"context"
	"fmt"
	"strings"

	servicev1 "example.com/project-proto/proto/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"example.com/project/internal/application/presenter"
	trustednet "example.com/project/internal/domain/trusted_net"
)

type GetBotsUC interface {
	Do(ctx context.Context, sources []trustednet.Source, ipV *trustednet.IPVersion) ([]trustednet.TrustedBotNet, error)
}

type BotsServer struct {
	servicev1.UnimplementedBotsServiceServer
	uc GetBotsUC
}

func NewBotsServer(uc GetBotsUC) *BotsServer {
	return &BotsServer{uc: uc}
}

func (s *BotsServer) GetBots(
	ctx context.Context,
	req *servicev1.GetBotsRequest,
) (*servicev1.GetBotsResponse, error) {
	sources, err := protoSourcesToTrusted(req.GetSources())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	var ipV *trustednet.IPVersion
	if req.IpVersion != nil {
		v := req.GetIpVersion()
		if v == servicev1.IPVersion_IP_VERSION_UNSPECIFIED {
			return nil, status.Error(codes.InvalidArgument, "ip_version must be v4 or v6 when set")
		}
		tv, err := trustednet.NewIPVersion(int(v))
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		ipV = &tv
	}

	nets, err := s.uc.Do(ctx, sources, ipV)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	items, err := presenter.TrustedBotNetsToProto(nets)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &servicev1.GetBotsResponse{Items: items}, nil
}

func protoSourcesToTrusted(srcs []servicev1.Source) ([]trustednet.Source, error) {
	if len(srcs) == 0 {
		return nil, nil
	}

	out := make([]trustednet.Source, 0, len(srcs))
	for _, src := range srcs {
		sourceName, ok := servicev1.Source_name[int32(src)]
		if !ok {
			return nil, fmt.Errorf("unknown source enum value: %v", src)
		}
		source, err := trustednet.NewSource(strings.ToLower(sourceName[len("SOURCE_"):]))
		if err != nil {
			return nil, err
		}
		out = append(out, source)
	}

	return out, nil
}
```
