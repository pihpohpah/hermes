# Alpine.js: локальное состояние и анимации

Основа: [Mindrally `alpine-js` skill](https://github.com/mindrally/skills/blob/HEAD/alpine-js/SKILL.md), переложенный на hermes.

## 1. Что делает Alpine в hermes

Alpine отвечает за состояние, которое не нужно серверу:

- видимость меню, шторок и шагов мастера (`x-show`, `x-if`);
- реактивные атрибуты: `:aria-expanded`, `:aria-pressed`, `:disabled`, `:style`;
- компоненты, которые сами читают данные через `api()` или IndexedDB: прогресс скачивания офлайн-карты (`offlineMap`), мастер добавления (`addMaster`), создание метки (`nowCreate`), полоса маршрута (`routeBar`).

Данные, которые показываются всем, по-прежнему приходят фрагментом с сервера (см. [htmx-and-rest.md](<htmx-and-rest.md>)).

## 2. Директивы и организация кода

- `x-show` переключает `display: none` и подходит для частых переключений.
- `<template x-if>` создаёт и удаляет узел. Его берём для тяжёлых или редких веток (мобильное меню в `layout.templ`) и для взаимоисключающих иконок (`$store.route.has(...)` в `components.templ`).
- `x-cloak` скрывает разметку до старта Alpine. Правило `[x-cloak]{display:none!important}` подключено инлайном в `layout.templ`.
- `@click.outside`, `@keydown.escape.window` закрывают меню.

Правила:

1. Инлайновый `x-data` допустим для одного-двух флагов (`{ open: false }`). Всё, где есть методы или геттеры, выносится в `Alpine.data()`.
2. Компонент живёт в модуле `web/static/js/app/<имя>.js` и экспортирует функцию `register<Имя>()`. `app.js` вызывает её в обработчике `alpine:init`:

   ```js
   document.addEventListener('alpine:init', () => {
     registerRoute();
     registerNowCreate();
     registerAddMaster();
     registerOfflineMap();
   });
   ```
3. Общее для нескольких компонентов состояние кладётся в `Alpine.store()`. Пример: `Alpine.store('route')` в `route.js` хранит состав маршрута, и кнопки «В маршрут» по всей странице реагируют через `x-bind:aria-pressed="$store.route.has('<id>')"`.
4. Объекты с внутренним состоянием браузера (`AbortController`, экземпляр MapLibre) не кладутся в реактивные поля: прокси Alpine ломает их методы. В `offline-map.js` `AbortController` лежит в замыкании `registerOfflineMap()`.

## 3. Анимации

Анимации описываются в `web/static/css/app.css` на токенах (`--dur-fast`, `--dur-base`, `--ease-out`). Alpine только показывает и скрывает элемент, а появление проигрывает CSS:

```templ
<div class="menu-anchor" x-data="{ open: false }" @keydown.escape.window="open = false">
	<button type="button" class="header__user" aria-haspopup="menu" :aria-expanded="open" @click="open = !open">
		…
	</button>
	<div class="menu" role="menu" x-show="open" x-cloak @click.outside="open = false" @click="open = false">
		…
	</div>
</div>
```

```css
.menu {
  animation: menu-in var(--dur-base) var(--ease-out);
  transform-origin: top right;
}

@keyframes menu-in {
  from { opacity: 0; transform: scale(0.96); }
}
```

`x-transition` без аргументов годится для простого fade (так сделано у `.mobile-menu`). Если нужна анимация скрытия, классы задаются через `x-transition:enter`, `x-transition:leave` и их `-start`/`-end`, а сами классы описываются в `app.css` по BEM, например `menu--enter` и `menu--leave` (таких классов пока нет, имена условные). Утилитарных классов Tailwind в проекте нет.

Глобальный блок `@media (prefers-reduced-motion: reduce)` в `app.css` сокращает все CSS-анимации и transition до 1 мс, отдельно его поддерживать не нужно.

## 4. Компонент с данными

`offlineMap` из `web/static/js/app/offline-map.js` (сокращено):

```js
export function registerOfflineMap() {
  // AbortController — вне реактивного состояния: через прокси Alpine его методы падают.
  let controller = null;

  Alpine.data('offlineMap', () => ({
    loading: true,
    downloading: false,
    received: 0,
    total: 0,
    unavailable: false,

    async init() {
      await this.refresh();
      this.loading = false;
    },

    get percent() {
      return this.total ? Math.min(100, Math.floor((this.received / this.total) * 100)) : 0;
    },
    // refresh(), download(), cancel(), remove(), clearCache() …
  }));
}
```

Разметка в `web/templates/profile.templ`:

```templ
<section class="offline-map" x-data="offlineMap" aria-labelledby="offline-map-title">
	<p class="offline-map__text" x-text="summary">Скачайте карту Приморья, чтобы она работала без интернета.</p>
	<div class="meter offline-map__meter" x-show="downloading" x-cloak>
		<div class="meter__track" role="progressbar" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="percent">
			<div class="meter__fill" :style="{ width: percent + '%' }"></div>
		</div>
	</div>
</section>
```

Текст внутри `x-text` отрисован сервером и виден до старта Alpine. Ошибки чтения хранилища и сервера логируются через `console.warn`, недоступный сервер карты дополнительно выставляет `unavailable`. JSON-ручки вызываются через `api()` из `api.js`, а не через голый `fetch`: он добавляет CSRF и разбирает обёртку `{success, error, code, data}`.

## 5. Alpine и htmx вместе

1. **Settle отключён.** После свопа htmx копирует `class` и `style` со старых узлов на новые и затирает инлайн-стили `x-show`, из-за чего меню открывалось само. Поэтому в `app.js` стоит `htmx.config.attributesToSettle = []`. Не включай settle обратно.
2. **Компонент вне цели свопа.** Alpine-компонент с долгим состоянием держи снаружи `hx-target`. `routeBar` в `map.templ` лежит рядом с `#map-panel-inner`, а не внутри него, поэтому своп панели его не пересоздаёт.
3. **Реакция на своп.** Если после свопа нужно что-то пересчитать, слушай `htmx:afterSwap` на `document.body` и проверяй `event.detail.target.id`, как в `catalog.js`. Для инициализации после `hx-boost`-перехода есть `htmx:load` (`initPage` в `app.js`).
4. **Закрытие после запроса.** Если форма отправляется через htmx, Alpine-флаг сбрасывается по событию: `@htmx:after-request="if ($event.detail.successful) open = false"`.
