# Hypermedia (HTMX) и REST: Основа фронтенда

Источник базовых правил: [Mindrally `htmx` skill](https://github.com/mindrally/skills/blob/HEAD/htmx/SKILL.md), адаптированный под Go-сервисы и REST-архитектуру.

---

## 1. Ключевые принципы Hypermedia + REST

1. **Сервер управляет состоянием**:
   - Не дублируй серверные модели, правила валидации и бизнес-логику в клиентском JavaScript.
   - Сервер возвращает либо готовые HTML-фрагменты (для HTMX-взаимодействий), либо каноничный JSON REST-ответ (`APIResponse[T]`: `{"success": bool, "error": string, "data": ...}`).
2. **Минимальный payload**:
   - На HTMX-запросы (`HX-Request: true`) возвращай только изменившийся HTML-фрагмент (partial), а не всю страницу целиком.
3. **Семантичные HTTP-методы и коды ответа**:
   - Чтение и фильтрация: `GET` (`hx-get`) + синхронизация query-параметров в URL (`hx-push-url="true"` или `hx-replace-url="true"`).
   - Создание: `POST` (`hx-post`).
   - Обновление: `PUT` / `PATCH` (`hx-put`, `hx-patch`).
   - Удаление: `DELETE` (`hx-delete` + `hx-confirm`).
   - Ошибки валидации возвращают `400 Bad Request` / `422 Unprocessable Entity`, серверные ошибки — `500 Internal Server Error`.

---

## 2. Базовые атрибуты HTMX и стратегии `hx-swap`

- **Запросы**: `hx-get`, `hx-post`, `hx-put`, `hx-patch`, `hx-delete`.
- **Куда вставлять**: `hx-target="#container"` или относительные селекторы (`closest tr`, `this`, `next .error`).
- **Как вставлять (`hx-swap`)**:
  - `innerHTML` (по умолчанию) — заменить содержимое целевого контейнера.
  - `outerHTML` — заменить сам целевой элемент (отлично подходит для обновления строки таблицы или карточки целиком).
  - `beforeend` — дописать в конец списка (бесконечный скролл или добавление нового элемента).
  - `delete` — удалить целевой элемент из DOM после успешного `hx-delete`.
  - `swap:150ms settle:150ms` — задержка для плавной CSS/Alpine-анимации при замене DOM.

---

## 3. Эталонные паттерны разметки HTMX

### 3.1. Реактивная фильтрация таблицы с синхронизацией URL (на примере подсетей ботов)

Форма фильтров отправляет `GET`-запрос при изменении любого селекта или ввода текста с debounce, обновляет таблицу `#bots-table-body` и сохраняет параметры фильтрации в адресной строке браузера.

```html
<section class="space-y-4">
  <form
    hx-get="/ui/bots"
    hx-target="#bots-table-body"
    hx-trigger="change, input delay:300ms from:input[name='q']"
    hx-replace-url="true"
    hx-indicator="#loading-indicator"
    class="flex flex-wrap items-center gap-3"
  >
    <select name="source" class="rounded border px-3 py-2 text-sm">
      <option value="">Все источники</option>
      <option value="yandex">Yandex</option>
      <option value="google">Google Common</option>
      <option value="google_special_crawlers">Google Special</option>
    </select>

    <select name="ipV" class="rounded border px-3 py-2 text-sm">
      <option value="">IPv4 + IPv6</option>
      <option value="4">IPv4</option>
      <option value="6">IPv6</option>
    </select>

    <input
      type="search"
      name="q"
      placeholder="Поиск по CIDR..."
      class="rounded border px-3 py-2 text-sm"
    />

    <span id="loading-indicator" class="htmx-indicator text-xs text-slate-500">
      Обновление...
    </span>
  </form>

  <table class="w-full border-collapse text-left text-sm">
    <thead>
      <tr class="border-b bg-slate-50">
        <th class="px-3 py-2">CIDR</th>
        <th class="px-3 py-2">Источник</th>
        <th class="px-3 py-2">Версия IP</th>
        <th class="px-3 py-2"></th>
      </tr>
    </thead>
    <tbody id="bots-table-body">
      <!-- Сервер рендерит сюда строки <tr> -->
    </tbody>
  </table>
</section>
```

### 3.2. Удаление строки с подтверждением и мгновенным удалением из DOM

```html
<tr id="bot-row-42" class="border-b">
  <td class="px-3 py-2 font-mono">66.249.64.0/19</td>
  <td class="px-3 py-2">google</td>
  <td class="px-3 py-2">4</td>
  <td class="px-3 py-2 text-right">
    <button
      hx-delete="/ui/bots/42"
      hx-target="closest tr"
      hx-swap="outerHTML swap:150ms"
      hx-confirm="Удалить подсеть 66.249.64.0/19?"
      class="text-red-600 hover:underline"
    >
      Удалить
    </button>
  </td>
</tr>
```

---

## 4. Как определять роуты и хендлеры на Go для Hypermedia + REST

В архитектуре Go-сервиса (`internal/application/routes/http`) один и тот же `GetBotsUseCase` может обслуживать:
1. **Чистый JSON REST-эндпоинт** (`/api/v1/bots`) через `APIResponseWriter[[]presenter.TrustedBotNet]`.
2. **Hypermedia HTML-эндпоинт** (`/ui/bots`), который рендерит полную страницу или только partial `<tbody>`, если присутствует заголовок `HX-Request: true`.

### Пример Go-хендлера для HTMX Partial + Full Page

```go
package http

import (
	"html/template"
	"log/slog"
	"net/http"
	"strconv"

	"example.com/project/internal/application/presenter"
	trustednet "example.com/project/internal/domain/trusted_net"
)

type BotsUIHandler struct {
	logger  *slog.Logger
	tmpl    *template.Template
	useCase GetBotsUC
}

func NewBotsUIHandler(logger *slog.Logger, tmpl *template.Template, uc GetBotsUC) *BotsUIHandler {
	return &BotsUIHandler{logger: logger, tmpl: tmpl, useCase: uc}
}

func (h *BotsUIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var ipV *trustednet.IPVersion
	if rawIPV := r.URL.Query().Get("ipV"); rawIPV != "" {
		v, err := strconv.Atoi(rawIPV)
		if err != nil {
			http.Error(w, "invalid ipV", http.StatusBadRequest)
			return
		}
		parsedIPV, err := trustednet.NewIPVersion(v)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ipV = &parsedIPV
	}

	var sources []trustednet.Source
	for _, rawSrc := range r.URL.Query()["source"] {
		if rawSrc == "" {
			continue
		}
		src, err := trustednet.NewSource(rawSrc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sources = append(sources, src)
	}

	nets, err := h.useCase.Do(r.Context(), sources, ipV)
	if err != nil {
		h.logger.Error("load bots for ui failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	viewData := presenter.TrustedBotNetsToHTTP(nets)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	templateName := "bots_page.html"
	if r.Header.Get("HX-Request") == "true" {
		templateName = "bots_table_rows.html"
	}

	if err := h.tmpl.ExecuteTemplate(w, templateName, viewData); err != nil {
		h.logger.Error("render template failed", "template", templateName, "error", err)
	}
}
```

---

## 5. Работа с чистым JSON REST API без HTMX

Если бэкенд предоставляет только JSON REST API (формат `APIResponse[T]` из `internal/application/routes/http/common.go`), не нужно подключать React — используй легкий `fetch` внутри `Alpine.data()` компонента (подробный шаблон приведен в [alpine-transitions-and-state.md](<alpine-transitions-and-state.md>)).
