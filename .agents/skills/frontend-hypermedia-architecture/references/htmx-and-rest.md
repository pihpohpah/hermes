# htmx и REST

Основа: [Mindrally `htmx` skill](https://github.com/mindrally/skills/blob/HEAD/htmx/SKILL.md), переложенный на hermes.

## 1. Принципы

1. **Состоянием управляет сервер.** Модели, валидация и бизнес-правила живут в Go. Клиент получает либо HTML-фрагмент, либо JSON в обёртке `APIResponse[T]` (`internal/application/routes/http/common.go`):

   ```json
   {"success": false, "error": "place not found", "code": "NOT_FOUND", "data": null}
   ```

   `code` заполняется только у ошибок (`INVALID_ARGUMENT`, `NOT_FOUND`, `UNAUTHORIZED`, `FORBIDDEN`, `CONFLICT`, `TIMEOUT`, `INTERNAL_ERROR` и др., см. `errorCode`).
2. **Страница и фрагмент рендерятся из одного view.** У каждой обновляемой области есть пара ручек: страница (`GET /catalog`, `Pages.CatalogPage`) и фрагмент (`GET /fragments/catalog-results`, `Fragments.CatalogResults`). Обе вызывают `loadCatalogView` и отдают один и тот же `templ`-компонент: страница целиком, фрагмент только `CatalogResults(view)`. Заголовок `HX-Request` для выбора шаблона не используется.
3. **htmx отвечает за GET.** Чтение, фильтры, вкладки и пагинация идут через `hx-get` на `/fragments/*`. Мутации (`POST /pins`, `PUT /pins/{id}/going`, `POST /submissions`) идут через `api()` из `web/static/js/app/api.js`: он подставляет `X-CSRF-Token` из `localStorage` и разбирает обёртку. `hx-post`, `hx-put` и `hx-delete` в шаблонах пока не встречаются; если они понадобятся, CSRF-заголовок придётся передать через `hx-headers`.
4. **Коды ответа.** Ошибки домена переводятся в статус функцией `statusCode`: `models.ErrInvalidArgument` в 400, `ErrNotFound` в 404, `ErrUnauthorized` в 401, `ErrForbidden` в 403, `ErrConflict` в 409, остальное в 500.

## 2. Атрибуты htmx

- `hx-get` — адрес фрагмента, обычно из хелпера view: `q.fragmentURL()` даёт `/fragments/catalog-results?...`.
- `hx-target` — контейнер с постоянным id: `#catalog-results`, `#map-panel-inner`, `#profile-tab`.
- `hx-push-url` — адрес полной страницы (`q.pageURL()`), чтобы ссылку можно было открыть заново и кнопка «Назад» работала.
- `hx-swap` — по умолчанию `innerHTML`, и в проекте другие варианты пока не нужны. `outerHTML` подходит для замены карточки целиком, `beforeend` для догрузки списка.
- `hx-trigger` — для поллинга: `load, every 60s` у счётчика «Сейчас».
- `hx-disable` — на формах поиска (`data-role="catalog-search"`, `"map-search"`). Ввод обрабатывает `catalog.js` с debounce 250 мс и вызывает `panelSwap()` из `nav.js`.

## 3. Примеры из кода

### 3.1. Чип фильтра справочника

`web/templates/catalog.templ`: ссылка работает и без JS, а с htmx подменяет только результаты.

```templ
<a
	class="chip"
	href={ templ.SafeURL(q.with(func(n *CatalogQueryView) { n.Category = category.ID }).pageURL()) }
	hx-get={ q.with(func(n *CatalogQueryView) { n.Category = category.ID }).fragmentURL() }
	hx-target="#catalog-results"
	hx-push-url={ q.with(func(n *CatalogQueryView) { n.Category = category.ID }).pageURL() }
	aria-pressed={ strconv.FormatBool(q.Category == category.ID) }
>
	@icon(category.Icon, "")
	{ category.Label }
</a>

<div id="catalog-results">
	@CatalogResults(view)
</div>
```

Выбранное состояние передаётся через `aria-pressed`, а не через класс `is-active`.

### 3.2. Поллинг счётчика

`web/templates/layout.templ`:

```templ
<span hx-get="/fragments/live-count" hx-trigger="load, every 60s" hx-swap="innerHTML">
	@LiveCount(v.LivePins)
</span>
```

### 3.3. Мутация и обновление фрагмента

`web/static/js/app/actions.js`, кнопка «Я приду» с `data-action="pin-going"`:

```js
const joined = el.getAttribute('aria-pressed') === 'true';
try {
  await api(`/pins/${encodeURIComponent(id)}/going`, { method: joined ? 'DELETE' : 'PUT' });
  toast(joined ? 'Отметку сняли' : 'Отметили: вы идёте. Автор увидит это');
  await refreshPins();
  htmx.ajax('GET', `/fragments/pin/${encodeURIComponent(id)}`, { target: '#map-panel-inner' });
} catch (error) {
  toast(error.message || 'Не получилось отметиться', { type: 'danger' });
}
```

Мутация идёт через `api()`, потом точки на карте и фрагмент панели перечитываются с сервера. Клиент сам счётчик не пересчитывает. `api()` возвращает `payload.data` или бросает `ApiError` с полями `status` и `code`. Текст ошибки показывается пользователю, поэтому сообщения сервера должны быть понятными.

## 4. Go: страница и фрагмент

`internal/application/routes/http/fragments.go`:

```go
// GET /fragments/catalog-results?... — сетка или список результатов справочника.
func (f *Fragments) CatalogResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	view, err := f.pages.loadCatalogView(r.Context(), r, parseCatalogQuery(r))
	if err != nil {
		f.error(w, r, "catalog results fragment", err)

		return
	}

	f.render(w, r, templates.CatalogResults(view))
}
```

`Pages.CatalogPage` в `pages.go` устроен так же, но рендерит `templates.CatalogPage(view)`. Новые ручки регистрируются в `internal/application/routes/router.go`: страницы рядом с `GET /catalog`, фрагменты рядом с `GET /fragments/*`, JSON рядом с `GET /api/*`.

## 5. Ошибки во фрагментах

`Fragments.error` пишет в лог и отвечает `http.Error` со статусом из `statusCode`. htmx по умолчанию не вставляет ответы 4xx и 5xx, поэтому контейнер остаётся со старым содержимым. Если пользователю нужно увидеть ошибку на месте, верни разметку пустого состояния со статусом 200 или обработай `htmx:responseError` в JS.

## 6. JSON без htmx

Если данные нужны скрипту, а не разметке (точки карты `GET /api/map-items`, карточка `GET /api/items/{id}`), используй `api()` из модуля или из `Alpine.data()`. Шаблон компонента описан в [alpine-transitions-and-state.md](<alpine-transitions-and-state.md>).
