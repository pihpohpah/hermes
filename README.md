# hermes

## О сервисе

Приложение планировнаия активного отдыха ВЫХОДИ (рабочее название).

Ответственная команда: **PihPohPahPuh**

## Стек

- Backend: Go 1.27, PostgreSQL, PostGIS
- Frontend: htmx + Alpine.js (основа), React 18 + Vite (карта)
  - Карта: MapLibre GL JS + PMTiles (векторные тайлы, офлайн-архив региона)
  - Клиентский кеш: IndexedDB (Dexie.js), Cache Storage
  - Service Worker: Workbox (precache оболочки, runtime-cache фрагментов)

## Офлайн-стратегия

- Оболочка (HTML/CSS/JS) - precache, обнова при деплое.
- Страницы `/pages/*` - cache-first после первого визита.
- Фрагменты `/fragments/*` - network-first, fallback на кэш.
- Погода/прогнозы - network-first, кэш в IndexedDB TTL 30m.
- Планы поездок и репорты - IndexedDB + outbox, синхронизация при online-событии.
- Статус: индикатор «не в сети, данные на HH:MM» в шапке.
