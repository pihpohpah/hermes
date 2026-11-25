# Изолированные модули и React

Правила для React взяты из [Vercel `react-best-practices`](https://github.com/vercel-labs/agent-skills/blob/HEAD/skills/react-best-practices/SKILL.md).

## 1. Остров без React: карта

React в hermes нет. Единственный «остров», то есть кусок страницы с собственным долгоживущим состоянием на клиенте, это карта MapLibre. Новый виджет такого рода строится по её образцу.

1. **Узел с данными.** Сервер рендерит контейнер и кладёт параметры в `data-*`:

   ```templ
   <div class="map-stage" id="map-stage" data-stage-mode={ mode } data-scenario={ scenario } aria-label="Карта">
   	…
   	<div class="map-canvas" id="map"></div>
   </div>
   ```

   Мини-карта на странице места делает то же самое через `data-mini-map`, `data-lat`, `data-lon` (`detail.templ`).
2. **Монтирование.** `initStage()` в `stage.js` читает `dataset`, при необходимости догружает вендор (`ensureMapVendor()`: при `hx-boost`-переходе `<head>` не перечитывается) и создаёт контроллер `createPlacesMap()` из `map.js`. Ошибка создания показывается в разметке «Карта не загрузилась» с кнопкой «Повторить».
3. **Жизненный цикл.** `initPage()` в `app.js` вызывается на `DOMContentLoaded` и `htmx:load`. Если узел `#map-stage` сменился или пропал, старый контроллер уничтожается через `destroy()`, иначе живёт дальше. Своп `#map-panel-inner` карту не трогает, потому что она лежит вне цели свопа.
4. **Связь с остальной страницей** идёт через методы контроллера (`window.placesMap.setPins()`, `.select()`, `.flyTo()`) и делегированные `data-action`, а не через общий реактивный стор.

## 2. Когда допустим React

React можно подключить, только если выполнены все условия:

- задачу не решить htmx-фрагментом, Alpine-компонентом или модулем по образцу карты;
- всё состояние живёт на клиенте и часто меняется: редактор графа с undo/redo, canvas с drag-and-drop, поток данных по WebSocket с клиентской агрегацией;
- остров монтируется в один узел с `data-*`, как карта, и не превращает страницу в SPA.

Для таблиц, форм, модалок, фильтров и навигации React не нужен. Подключение React меняет сборку: `mise run assets` собирает esbuild-ом один бандл из `app.js` без npm-зависимостей и JSX. Поэтому такое решение согласуется заранее.

## 3. Правила Vercel по приоритету

| Приоритет | Категория | Влияние | Префикс правил |
| :--- | :--- | :--- | :--- |
| 1 | Водопады запросов | CRITICAL | `async-` |
| 2 | Размер бандла | CRITICAL | `bundle-` |
| 3 | Серверная производительность | HIGH | `server-` |
| 4 | Загрузка данных на клиенте | MEDIUM-HIGH | `client-` |
| 5 | Лишние ре-рендеры | MEDIUM | `rerender-` |
| 6 | Производительность рендеринга | MEDIUM | `rendering-` |
| 7 | Производительность JavaScript | LOW-MEDIUM | `js-` |
| 8 | Продвинутые паттерны хуков | LOW | `advanced-` |

Правила `async-`, `bundle-`, `client-` и `js-` относятся к любому JS, не только к React.

## 4. Главные правила с примерами

### 4.1. Водопады (`async-*`)

- **`async-parallel`.** Независимые запросы запускаются параллельно через `Promise.all()`, а не последовательными `await`.
- **`async-cheap-condition-before-await`, `async-defer-await`.** Сначала дешёвые синхронные проверки, `await` только в нужной ветке.

Так загружаются остановки маршрута в `route.js`:

```js
const loaded = await Promise.all(ids.map((id) => fetchItem(id).catch(() => null)));
if (current !== this.version) return;
this.stops = loaded.filter(Boolean);
```

Проверка `version` отбрасывает ответ, если за время запроса маршрут успел измениться.

### 4.2. Размер бандла (`bundle-*`)

- **`bundle-barrel-imports`.** Импорт напрямую из модуля, без корневых `index.js`, которые тянут лишнее.
- **`bundle-conditional`, `bundle-dynamic-imports`.** Тяжёлые зависимости грузятся лениво, только когда нужны. В hermes так грузится MapLibre: `ensureMapVendor()` подключает скрипты только на страницах с картой. В React для этого есть `lazy()` и динамический `import()`:

```tsx
import { lazy, Suspense, useState } from 'react';

const RouteGraphEditor = lazy(() => import('./RouteGraphEditor'));

export function RouteEditorIsland() {
  const [open, setOpen] = useState(false);

  return (
    <div>
      <button type="button" onClick={() => setOpen((prev) => !prev)}>Редактор маршрута</button>
      {open ? (
        <Suspense fallback={<div>Загрузка редактора…</div>}>
          <RouteGraphEditor />
        </Suspense>
      ) : null}
    </div>
  );
}
```

### 4.3. Загрузка данных и события (`client-*`)

- **`client-swr-dedup`.** Чтение REST в React-острове идёт через дедуплицирующий кэш (SWR или аналог), а не через голый `useEffect`. Вне React ту же роль играет `itemCache` в `fetchItem()` из `api.js`.
- **`client-passive-event-listeners`.** У обработчиков `scroll`, `wheel`, `touchstart` указывается `{ passive: true }`.

### 4.4. Ре-рендеры (`rerender-*`)

- **`rerender-derived-state-no-effect`.** Производное состояние вычисляется в рендере, а не через `useEffect` + `setState`.
- **`rerender-move-effect-to-event`.** Реакция на действие пользователя живёт в обработчике (`onClick`, `onSubmit`), а не в `useEffect`.
- **`rerender-functional-setstate`.** Апдейтер `setItems((prev) => …)`, чтобы колбэк не зависел от текущего состояния.
- **`rerender-lazy-state-init`.** Дорогое начальное значение передаётся функцией: `useState(() => JSON.parse(node.dataset.initial))`.
- **`rerender-transitions`, `rerender-use-deferred-value`.** Тяжёлая фильтрация оборачивается в `startTransition` или `useDeferredValue`, чтобы ввод не тормозил.
- **`rerender-no-inline-components`.** Дочерний компонент не объявляется внутри другого компонента.

```tsx
import { useDeferredValue, useMemo, useState } from 'react';

interface Place {
  id: string;
  title: string;
}

export function PlaceFilter({ places }: { places: Place[] }) {
  const [query, setQuery] = useState('');
  const deferredQuery = useDeferredValue(query);

  const filtered = useMemo(() => {
    const q = deferredQuery.trim().toLowerCase();
    if (!q) return places;
    return places.filter((p) => p.title.toLowerCase().includes(q));
  }, [places, deferredQuery]);

  return (
    <div>
      <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Название места" />
      <p>Найдено: {filtered.length}</p>
    </div>
  );
}
```

### 4.5. Рендеринг и JavaScript (`rendering-*`, `js-*`)

- **`rendering-conditional-render`.** `condition ? <Node /> : null` вместо `count && <Node />`, иначе в DOM попадёт `0`.
- **`rendering-hoist-jsx`.** Статичный JSX выносится в константы вне компонента.
- **`rendering-content-visibility`.** Длинным спискам помогает CSS `content-visibility: auto`.
- **`js-set-map-lookups`, `js-index-maps`.** Для частых проверок вхождения и поиска по ключу берутся `Set` и `Map`, а не `includes` и `find` по массиву.
- **`js-hoist-regexp`.** Регулярные выражения создаются вне рендера и циклов.
- **`js-tosorted-immutable`.** `array.toSorted()` вместо мутирующего `.sort()`.
