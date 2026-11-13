# React-острова в редких кейсах (Vercel React Best Practices)

Источник базовых правил: [Vercel Engineering `vercel-react-best-practices`](https://github.com/vercel-labs/agent-skills/blob/HEAD/skills/react-best-practices/SKILL.md).

---

## 1. Когда React разрешен (и когда запрещен)

В нашей архитектуре базой являются **HTMX (Hypermedia) + REST** и **Alpine.js**.
- **Запрещено использовать React**: для таблиц, CRUD-форм, модалок, фильтров, лендингов, стандартных админок и навигации.
- **Разрешено использовать React (как изолированный Island-виджет)**:
  1. Интерактивная визуализация сетевых топологий / подсетей / графов на Canvas/WebGL/SVG с зумом и drag-and-drop.
  2. Real-time дашборды с высокой частотой обновления (WebSockets/SSE) и сложной клиентской агрегацией.
  3. Сложные многошаговые редакторы со связанным локальным графом состояния (undo/redo стек, live-компиляция правил).

Даже когда React оправдан, монтируй его **в конкретный DOM-узел** (`<div id="subnet-visualizer-island" data-initial='...'></div>`), а не превращай все приложение в тяжелое SPA.

---

## 2. Приоритизированные правила Vercel React Best Practices

Если ты пишешь или ревьюишь React-компонент, обязательно проверяй его по 8 категориям правил в порядке критичности:

| Приоритет | Категория | Влияние | Префикс правил |
| :--- | :--- | :--- | :--- |
| 1 | Устранение водопадов запросов (Eliminating Waterfalls) | CRITICAL | `async-` |
| 2 | Оптимизация размера бандла (Bundle Size Optimization) | CRITICAL | `bundle-` |
| 3 | Серверная производительность (Server-Side Performance) | HIGH | `server-` |
| 4 | Клиентская загрузка данных (Client-Side Data Fetching) | MEDIUM-HIGH | `client-` |
| 5 | Оптимизация ре-рендеров (Re-render Optimization) | MEDIUM | `rerender-` |
| 6 | Производительность рендеринга (Rendering Performance) | MEDIUM | `rendering-` |
| 7 | Производительность JavaScript (JS Performance) | LOW-MEDIUM | `js-` |
| 8 | Продвинутые паттерны хуков (Advanced Patterns) | LOW | `advanced-` |

---

## 3. Главные правила с примерами кода

### 3.1. Устранение водопадов (`async-*` — CRITICAL)
- **`async-parallel`**: Никогда не делай последовательные `await` для независимых REST-запросов. Запускай их параллельно через `Promise.all()`.
- **`async-cheap-condition-before-await` / `async-defer-await`**: Сначала проверяй быстрые синхронные условия, и только в нужной ветке вызывай `await`.

```tsx
// Плохо: последовательный водопад (Waterfall)
async function loadBotStats() {
  const yandex = await fetch('/api/v1/bots?source=yandex').then((r) => r.json());
  const google = await fetch('/api/v1/bots?source=google').then((r) => r.json());
  return { yandex: yandex.data, google: google.data };
}

// Хорошо: параллельная загрузка
async function loadBotStats() {
  const [yandexRes, googleRes] = await Promise.all([
    fetch('/api/v1/bots?source=yandex'),
    fetch('/api/v1/bots?source=google'),
  ]);
  const [yandex, google] = await Promise.all([yandexRes.json(), googleRes.json()]);
  return { yandex: yandex.data, google: google.data };
}
```

### 3.2. Оптимизация размера бандла (`bundle-*` — CRITICAL)
- **`bundle-barrel-imports`**: Избегай импортов из корневых barrel-файлов (`index.ts`), которые тянут сотни неиспользуемых иконок или утилит. Импортируй модули напрямую.
- **`bundle-conditional` / `bundle-dynamic-imports`**: Подгружай тяжелые модули (графики, редакторы кода) лениво через `React.lazy()` или динамический `import()` только при активации функции.

```tsx
import { lazy, Suspense, useState } from 'react';

const HeavyCidrMap = lazy(() => import('./HeavyCidrMap'));

export function CidrAnalyzerIsland() {
  const [showMap, setShowMap] = useState(false);

  return (
    <div>
      <button onClick={() => setShowMap((prev) => !prev)}>Показать карту подсетей</button>
      {showMap ? (
        <Suspense fallback={<div>Загрузка карты...</div>}>
          <HeavyCidrMap />
        </Suspense>
      ) : null}
    </div>
  );
}
```

### 3.3. Клиентский фетчинг и события (`client-*` — MEDIUM-HIGH)
- **`client-swr-dedup`**: Для чтения REST API в React-острове используй `SWR` (или дедуплицирующий кэш), чтобы избежать повторных запросов и гонок в `useEffect`.
- **`client-passive-event-listeners`**: Для обработчиков `scroll`, `wheel`, `touchstart` всегда указывай `{ passive: true }`.

### 3.4. Оптимизация ре-рендеров (`rerender-*` — MEDIUM)
- **`rerender-derived-state-no-effect`**: Никогда не вычисляй производное состояние через `useEffect` + `setState`. Вычисляй его прямо во время рендера.
- **`rerender-move-effect-to-event`**: Логика, вызванная действием пользователя (отправка формы, клик по фильтру), должна жить в обработчике события (`onClick`, `onSubmit`), а не в `useEffect`.
- **`rerender-functional-setstate`**: Используй функциональный апдейтер `setItems(prev => ...)`, чтобы колбэки не зависели от текущего состояния.
- **`rerender-lazy-state-init`**: Передавай функцию `useState(() => computeInitial())` для дорогих начальных вычислений (например, парсинг `dataset.initial`).
- **`rerender-transitions` / `rerender-use-deferred-value`**: Оборачивай несрочные тяжелые обновления фильтрации в `startTransition` или `useDeferredValue`, чтобы ввод в инпут оставался мгновенным.
- **`rerender-no-inline-components`**: Никогда не объявляй дочерние компоненты внутри функции другого компонента.

```tsx
import { useDeferredValue, useMemo, useState } from 'react';

interface BotNet {
  cidr: string;
  source: string;
  type: number;
}

export function CidrFilterWidget({ items }: { items: BotNet[] }) {
  const [query, setQuery] = useState('');
  const deferredQuery = useDeferredValue(query);

  // Вычисляем производное состояние в рендере, без useEffect + setState
  const filtered = useMemo(() => {
    const q = deferredQuery.trim().toLowerCase();
    if (!q) return items;
    return items.filter((n) => n.cidr.toLowerCase().includes(q));
  }, [items, deferredQuery]);

  return (
    <div>
      <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Фильтр CIDR..." />
      <p>Найдено: {filtered.length}</p>
    </div>
  );
}
```

### 3.5. Рендеринг и JavaScript-оптимизации (`rendering-*`, `js-*`)
- **`rendering-conditional-render`**: Используй тернарный оператор `condition ? <Node /> : null` вместо `count && <Node />`, чтобы случайно не отрендерить `0` в DOM.
- **`rendering-hoist-jsx`**: Выноси статичный неизменяемый JSX в константы за пределы компонента.
- **`rendering-content-visibility`**: Для длинных списков используй CSS `content-visibility: auto`.
- **`js-set-map-lookups` / `js-index-maps`**: Для частых проверок вхождения или поиска по ключу (например, проверка CIDR в списке выбранных) используй `Set` и `Map` (`O(1)`) вместо `Array.prototype.includes` / `find` (`O(N)`).
- **`js-hoist-regexp`**: Выноси создание `new RegExp(...)` или литералов регулярных выражений за пределы рендера и циклов.
- **`js-tosorted-immutable`**: Используй `array.toSorted()` вместо мутирующего `.sort()`.
