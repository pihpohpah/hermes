# Alpine.js: Основа транзишенов и локального UI-состояния

Источник базовых правил: [Mindrally `alpine-js` skill](https://github.com/mindrally/skills/blob/HEAD/alpine-js/SKILL.md).

---

## 1. Роль Alpine.js в архитектуре

Alpine.js отвечает за все, что происходит **внутри браузера без перезагрузки страницы**:
- **Основа транзишенов (`x-transition`)**: плавные переходы открытия/закрытия модальных окон, дропдаунов, боковых панелей, аккордеонов, тостов и переключения вкладок.
- **Локальное реактивное состояние (`x-data`, `x-show`, `x-bind`, `x-model`)**: управление флагами видимости, локальной валидацией форм до сабмита, выделением чекбоксов и копированием в буфер обмена.
- **Легкие клиенты к JSON REST API**: если серверная ручка отдает JSON `{"success": true, "error": "", "data": [...]}` (как `/api/v1/bots`), `Alpine.data()` идеально справляется с загрузкой и фильтрацией без сборки тяжелого SPA.

---

## 2. Базовые директивы и правила производительности

### Основные директивы:
- `x-data` — локальное реактивное состояние компонента.
- `x-show` — переключение `display: none` (предпочтительнее `x-if` для частых переключений, так как узел остается в DOM и поддерживает `x-transition`).
- `x-if` — полное создание/удаление из DOM на `<template x-if="...">` (используй только для тяжелых веток, которые редко открываются).
- `x-transition` — декларативная анимация появления и исчезновения.
- `x-bind` (`:`) — реактивная привязка атрибутов и классов (`:class`, `:disabled`, `:aria-expanded`).
- `x-on` (`@`) — подписка на браузерные и кастомные события (`@click`, `@keydown.escape.window`, `@htmx:after-swap.window`).
- `x-model` — двусторонняя привязка полей ввода.
- `x-cloak` — скрывает неинициализированный шаблон до старта Alpine (`[x-cloak] { display: none !important; }`).

### Правила организации кода:
1. Держи объекты `x-data` маленькими и сфокусированными.
2. Не пиши длинные многострочные JS-выражения прямо в HTML-атрибутах — выноси логику в `Alpine.data('componentName', () => ({ ... }))`.
3. Для состояния, общего между несколькими блоками страницы (например, очередь уведомлений/тостов), используй `Alpine.store('notifications', { ... })`.

---

## 3. Стандарт транзишенов (`x-transition`)

Все анимации появления/исчезновения в интерфейсе строятся на `x-transition` в паре с утилитарными классами Tailwind CSS.

### 3.1. Быстрый дефолтный транзишен (fade + scale)
Для простых элементов и подсказок достаточно директивы `x-transition` с модификатором длительности:
```html
<div x-show="open" x-transition.duration.150ms x-cloak>
  Контент
</div>
```

### 3.2. Выпадающее меню (Dropdown) с кастомными классами Tailwind и закрытием по клику вне/Escape
```html
<div
  x-data="{ open: false }"
  @click.outside="open = false"
  @keydown.escape.window="open = false"
  class="relative inline-block text-left"
>
  <button
    type="button"
    @click="open = !open"
    :aria-expanded="open"
    class="inline-flex items-center gap-2 rounded border bg-white px-3 py-2 text-sm font-medium shadow-sm"
  >
    <span>Источники</span>
    <span class="transition-transform duration-150" :class="open && 'rotate-180'">▾</span>
  </button>

  <div
    x-show="open"
    x-cloak
    x-transition:enter="transition ease-out duration-150"
    x-transition:enter-start="opacity-0 scale-95 -translate-y-1"
    x-transition:enter-end="opacity-100 scale-100 translate-y-0"
    x-transition:leave="transition ease-in duration-100"
    x-transition:leave-start="opacity-100 scale-100 translate-y-0"
    x-transition:leave-end="opacity-0 scale-95 -translate-y-1"
    class="absolute right-0 z-20 mt-2 w-56 origin-top-right rounded-md border bg-white p-1 shadow-lg"
  >
    <a href="?source=yandex" class="block rounded px-3 py-2 text-sm hover:bg-slate-100">Yandex</a>
    <a href="?source=google" class="block rounded px-3 py-2 text-sm hover:bg-slate-100">Google Common</a>
    <a href="?source=google_special_crawlers" class="block rounded px-3 py-2 text-sm hover:bg-slate-100">Google Special</a>
  </div>
</div>
```

### 3.3. Модальное окно с отдельным транзишеном для фона (Backdrop) и панели (Dialog)
```html
<div
  x-data="{ open: false }"
  @open-modal.window="open = true"
  @keydown.escape.window="open = false"
>
  <button @click="open = true" class="rounded bg-slate-900 px-4 py-2 text-sm text-white">
    Добавить подсеть
  </button>

  <div
    x-show="open"
    x-cloak
    class="fixed inset-0 z-50 flex items-center justify-center p-4"
    role="dialog"
    aria-modal="true"
  >
    <div
      x-show="open"
      x-transition:enter="transition-opacity ease-out duration-200"
      x-transition:enter-start="opacity-0"
      x-transition:enter-end="opacity-100"
      x-transition:leave="transition-opacity ease-in duration-150"
      x-transition:leave-start="opacity-100"
      x-transition:leave-end="opacity-0"
      @click="open = false"
      class="fixed inset-0 bg-slate-900/50 backdrop-blur-xs"
    ></div>

    <div
      x-show="open"
      x-transition:enter="transition ease-out duration-200"
      x-transition:enter-start="opacity-0 translate-y-2 scale-95"
      x-transition:enter-end="opacity-100 translate-y-0 scale-100"
      x-transition:leave="transition ease-in duration-150"
      x-transition:leave-start="opacity-100 translate-y-0 scale-100"
      x-transition:leave-end="opacity-0 translate-y-2 scale-95"
      class="relative z-10 w-full max-w-md rounded-lg bg-white p-6 shadow-xl"
    >
      <h2 class="text-base font-semibold">Новая подсеть</h2>
      <div id="modal-body" class="mt-4">
        <!-- Сюда может подгружаться форма через hx-get -->
      </div>
    </div>
  </div>
</div>
```

---

## 4. Паттерн: Связка Alpine.js + JSON REST API (`APIResponse[T]`)

Когда бэкенд отдает стандартный JSON-контракт `{"success": bool, "error": string, "data": [...]}` (как ручка `GET /api/v1/bots` в `bot-keeper`), используй `Alpine.data()` для реактивного запроса и отображения с плавными переходами:

```html
<div x-data="botsExplorer()" x-init="fetchBots()" class="space-y-4">
  <div class="flex flex-wrap items-center gap-3">
    <select x-model="source" @change="fetchBots()" class="rounded border px-3 py-2 text-sm">
      <option value="">Все источники</option>
      <option value="yandex">Yandex</option>
      <option value="google">Google Common</option>
      <option value="google_special_crawlers">Google Special</option>
    </select>

    <select x-model="ipV" @change="fetchBots()" class="rounded border px-3 py-2 text-sm">
      <option value="">Все версии IP</option>
      <option value="4">IPv4</option>
      <option value="6">IPv6</option>
    </select>
  </div>

  <div
    x-show="error"
    x-cloak
    x-transition.opacity.duration.150ms
    class="rounded border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
    x-text="error"
  ></div>

  <div class="relative min-h-32 rounded border bg-white">
    <div
      x-show="loading"
      x-transition.opacity.duration.150ms
      class="absolute inset-0 z-10 flex items-center justify-center bg-white/70 text-sm text-slate-500"
    >
      Загрузка...
    </div>

    <table class="w-full text-left text-sm">
      <thead>
        <tr class="border-b bg-slate-50">
          <th class="px-3 py-2">CIDR</th>
          <th class="px-3 py-2">Источник</th>
          <th class="px-3 py-2">IP</th>
        </tr>
      </thead>
      <tbody>
        <template x-for="item in items" :key="item.cidr + '-' + item.source">
          <tr class="border-b last:border-0">
            <td class="px-3 py-2 font-mono" x-text="item.cidr"></td>
            <td class="px-3 py-2" x-text="item.source"></td>
            <td class="px-3 py-2" x-text="'IPv' + item.type"></td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</div>

<script>
  document.addEventListener('alpine:init', () => {
    Alpine.data('botsExplorer', () => ({
      source: '',
      ipV: '',
      items: [],
      loading: false,
      error: '',

      async fetchBots() {
        this.loading = true;
        this.error = '';

        const params = new URLSearchParams();
        if (this.source) params.append('source', this.source);
        if (this.ipV) params.append('ipV', this.ipV);

        const query = params.toString();
        const url = query ? `/api/v1/bots?${query}` : '/api/v1/bots';

        try {
          const res = await fetch(url, { headers: { Accept: 'application/json' } });
          const payload = await res.json();

          if (!res.ok || !payload.success) {
            this.error = payload.error || `HTTP ${res.status}`;
            this.items = [];
            return;
          }

          this.items = payload.data || [];
        } catch (err) {
          this.error = err.message || 'Ошибка сети';
        } finally {
          this.loading = false;
        }
      },
    }));
  });
</script>
```

---

## 5. Паттерн: Совместная работа HTMX и Alpine.js

1. **Закрытие Alpine-модалки после успешного HTMX-сабмита**:
   Бэкенд при успешном `POST` может вернуть заголовок `HX-Trigger: itemSaved`, либо кнопка/форма слушает событие `@htmx:after-request`:
   ```html
   <form
     hx-post="/ui/bots"
     hx-target="#bots-table-body"
     @htmx:after-request="if ($event.detail.successful) open = false"
   >
     <!-- Поля формы -->
   </form>
   ```
2. **Сохранение состояния Alpine при HTMX-замене**:
   Если внутри контейнера, обновляемого через `hx-swap`, находится независимый Alpine-компонент, используй расширение `alpine-morph` или обновляй через `hx-target` только внутренний список данных (`<tbody>`), оставляя внешний контейнер с `x-data` нетронутым.
