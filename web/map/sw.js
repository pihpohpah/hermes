// Демонстрационный Service Worker для офлайн-карты.
//
// Разделение ответственности:
//   - SW кеширует ТОЛЬКО оболочку (страницу, стиль, библиотеки, глифы).
//     Без этого страница не откроется по F5 при выключенной сети.
//   - Данные карты лежат в IndexedDB: полный архив (кнопка «Скачать карту»)
//     либо отдельные куски, полученные при онлайн-просмотре (см. offline.js).
//     Так SW не дублирует десятки мегабайт в Cache Storage.
//
// ВАЖНО про стратегию. Для HTML и JS используется network-first: cache-first
// здесь опасен — он навсегда залипает на старой версии страницы, и исправления
// просто не доезжают до пользователя.
//
// Это учебная реализация под демо, а не продакшн: в реальном приложении нужен
// Workbox с версионированием и стратегиями по типам ресурсов.

const VERSION = 'hermes-map-v3';
const SHELL_CACHE = `${VERSION}-shell`;

// Минимум, без которого страница карты не запустится.
const CORE_ASSETS = [
  '/map/',
  '/map/style.json',
  '/map/offline.js',
  '/map/favicon.svg',
  '/map/vendor/maplibre-gl.js',
  '/map/vendor/maplibre-gl.css',
  '/map/vendor/pmtiles.js',
];

// Глифы нужны для подписей: без них карта отрисуется, но без текста.
// Меняются редко, поэтому их можно отдавать cache-first.
const GLYPH_FONTS = ['Noto Sans Regular', 'Noto Sans Bold', 'Noto Sans Italic'];
const GLYPH_RANGES = ['0-255', '256-511', '768-1023', '1024-1279', '1280-1535'];

const glyphUrls = GLYPH_FONTS.flatMap((font) =>
  GLYPH_RANGES.map((url) => `/map/fonts/${encodeURIComponent(font)}/${url}.pbf`)
);

const PRECACHE = [...CORE_ASSETS, ...glyphUrls];

// Файлы, которые обновляются при разработке: их нельзя брать из кеша первыми.
const isDynamic = (pathname) =>
  pathname === '/map/' ||
  pathname.endsWith('/index.html') ||
  pathname.endsWith('.js') ||
  pathname.endsWith('.json') ||
  pathname === '/map/sw.js';

const isGlyph = (pathname) => pathname.startsWith('/map/fonts/');

self.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(SHELL_CACHE);
      // По одному: один недоступный файл не должен ломать установку.
      await Promise.all(PRECACHE.map((url) => cache.add(url).catch(() => undefined)));
      await self.skipWaiting();
    })()
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(
        keys.filter((key) => !key.startsWith(VERSION)).map((key) => caches.delete(key))
      );
      await self.clients.claim();
    })()
  );
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  if (request.method !== 'GET') return;

  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;

  // Архив и его Range-запросы не перехватываем: данные карты живут в IndexedDB,
  // а не в Cache Storage.
  if (url.pathname === '/api/map/region.pmtiles') return;
  if (url.pathname === '/api/map/relief.pmtiles') return;

  if (!url.pathname.startsWith('/map/')) return;

  // Глифы: cache-first, они неизменяемы.
  if (isGlyph(url.pathname)) {
    event.respondWith(
      (async () => {
        const cache = await caches.open(SHELL_CACHE);
        const cached = await cache.match(request);
        if (cached) return cached;
        try {
          const response = await fetch(request);
          if (response.ok) cache.put(request, response.clone());
          return response;
        } catch (error) {
          return new Response('', { status: 504 });
        }
      })()
    );
    return;
  }

  // HTML/JS/JSON: network-first, кеш — только запасной вариант.
  // Это гарантирует, что после обновления кода пользователь получит новую версию.
  if (isDynamic(url.pathname)) {
    event.respondWith(
      (async () => {
        const cache = await caches.open(SHELL_CACHE);
        try {
          const response = await fetch(request, { cache: 'no-cache' });
          if (response.ok) cache.put(request, response.clone());
          return response;
        } catch (error) {
          const cached = await cache.match(request, { ignoreSearch: true });
          if (cached) return cached;
          if (request.mode === 'navigate') {
            const fallback = await cache.match('/map/');
            if (fallback) return fallback;
          }
          return new Response('Нет сети, ресурс не закеширован', {
            status: 503,
            headers: { 'Content-Type': 'text/plain; charset=utf-8' },
          });
        }
      })()
    );
    return;
  }

  // Остальное из /map/ (картинки и т.п.): cache-first.
  event.respondWith(
    (async () => {
      const cache = await caches.open(SHELL_CACHE);
      const cached = await cache.match(request);
      if (cached) return cached;
      try {
        const response = await fetch(request);
        if (response.ok) cache.put(request, response.clone());
        return response;
      } catch (error) {
        return new Response('', { status: 504 });
      }
    })()
  );
});
