---
name: frontend-hypermedia-architecture
description: Как писать фронтенд hermes. Сервер рендерит страницы и фрагменты через templ, htmx подменяет фрагменты, Alpine.js держит локальное состояние, а MapLibre живёт в изолированном JS-модуле. React в проекте нет и без веской причины не появляется.
---

# Фронтенд hermes: templ + htmx + Alpine.js

Правила основаны на скиллах [Mindrally (`htmx`, `alpine-js`)](https://github.com/mindrally/skills) и [Vercel `react-best-practices`](https://github.com/vercel-labs/agent-skills), с поправкой на код hermes.

## 1. Какой инструмент брать

Клиентского JS пишем как можно меньше. Инструмент выбирается по порядку, к следующему переходим, только если предыдущий не справляется.

1. **htmx и HTML-фрагменты.** Сервер владеет данными и разметкой. Страница (`/catalog`) и её фрагмент (`/fragments/catalog-results`) рендерятся из одного `templ`-компонента и одного загрузчика view. Ссылки и чипы фильтров несут `hx-get` на фрагмент, `hx-target` на контейнер и `hx-push-url` на адрес страницы, так что без JS они остаются обычными ссылками. `<body hx-boost="true">` превращает навигацию между страницами в свопы.
2. **Alpine.js.** Локальное состояние без похода на сервер: меню, шторки, шаги мастера, прогресс скачивания. Компоненты регистрируются через `Alpine.data()` в модулях `web/static/js/app/` (`addMaster`, `nowCreate`, `offlineMap`, `routeBar`), общее состояние через `Alpine.store('route')`.
3. **JS-модуль с собственным жизненным циклом.** То, что не выражается разметкой: карта MapLibre (`map.js`, `stage.js`), мутации через `api()` из `api.js`. Модуль монтируется в узел с `data-*` атрибутами и переживает свопы соседних фрагментов.
4. **React** в проекте не используется. Подключать его можно только как изолированный остров для задачи, где всё состояние живёт на клиенте и пункт 3 уже не справляется. Правила для такого случая лежат в [react-islands-best-practices.md](<references/react-islands-best-practices.md>).

## 2. Общие правила

1. **Комментарии.** Атрибуты `hx-*` и директивы `x-*` не комментируем. Комментарий нужен для неочевидного контракта или обхода: зачем `htmx.config.attributesToSettle = []` в `app.js`, почему `AbortController` в `offline-map.js` лежит вне реактивного состояния.
2. **Декларативность.** `document.querySelector` и ручной `addEventListener` пишем, только когда `hx-trigger`, `@click` или `@keydown.escape.window` не подходят. Обработчики, которые всё же нужны, делегируются на `document` по `data-role` или `data-action` (`catalog.js`, `actions.js`).
3. **Анимации.** Появление меню и панелей описывается в `app.css` на токенах `--dur-*` и `--ease-*` (например, `.menu` с `animation: menu-in`). Alpine только переключает видимость через `x-show`, `x-transition` без классов допустим для простых случаев. Глобальный блок `prefers-reduced-motion` в `app.css` сам гасит CSS-анимации и transition. JS-анимации проверяют это условие сами, как `prefersReducedMotion()` в `map.js`.
4. **Стили.** Рукописный CSS по BEM в `web/static/css/app.css` (`catalog__head`, `chip--removable`). Tailwind и утилитарных классов в проекте нет. После правки JS или CSS пересобрать бандл: `mise run assets`.
5. **Связь с Go.** Шаблоны пишутся только на `templ` (`web/templates/*.templ`), после правки нужен `templ generate`. JSON-ручки отвечают обёрткой `APIResponse[T]` из `internal/application/routes/http/common.go`: `{"success": bool, "error": string, "code": string, "data": ...}`, поле `code` есть только у ошибок.

## 3. Справочники

| Тема | Что внутри | Файл |
| :--- | :--- | :--- |
| htmx и REST | страница и фрагмент из одного view, `hx-target`/`hx-swap`, мутации через `api()`, ошибки | [htmx-and-rest.md](<references/htmx-and-rest.md>) |
| Alpine.js | регистрация компонентов, `Alpine.store`, анимации на CSS, совместная работа с htmx | [alpine-transitions-and-state.md](<references/alpine-transitions-and-state.md>) |
| Изолированные модули и React | монтирование карты как острова, когда допустим React, правила Vercel | [react-islands-best-practices.md](<references/react-islands-best-practices.md>) |

## 4. Порядок решения

Перед созданием UI-элемента пройди список сверху вниз:

1. Нужно загрузить или отфильтровать данные? Добавь фрагмент в `Fragments` (`routes/http/fragments.go`), отрисуй его тем же `templ`-компонентом, что и страницу, и повесь `hx-get` + `hx-target` + `hx-push-url`.
2. Нужно создать, изменить или удалить данные? Вызови `api(path, { method, body })` из `api.js`: он добавит `X-CSRF-Token` и разберёт обёртку. После успеха обнови фрагмент через `panelSwap()` из `nav.js` или `htmx.ajax`.
3. Нужно открыть, закрыть, переключить шаг или показать прогресс? Alpine: `x-data`, `x-show`, `:class`, `:aria-*`.
4. Нужен canvas, WebGL или долгоживущий клиентский объект? Отдельный модуль по образцу `stage.js`. React рассматривай только после этого шага и по [react-islands-best-practices.md](<references/react-islands-best-practices.md>).
