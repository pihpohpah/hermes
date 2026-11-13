// Демонстрационное хранилище офлайн-архива карты.
//
// Тайлы по умолчанию читаются по HTTP Range: архив целиком не скачивается.
// Кнопка «Скачать карту» делает обратное — забирает весь PMTiles в IndexedDB
// и дальше отдаёт тайлы из локальной копии, без сети.
//
// Это тестовый механизм для проверки офлайна, а не продакшн-решение:
// полноценный офлайн должен жить в Service Worker с управлением квотой,
// версионированием и докачкой.

const DB_NAME = 'hermes-map';
// v2: добавлен стор кусков (chunks) для кеша онлайн-тайлов.
const DB_VERSION = 2;
const STORE = 'archives';
const CHUNK_STORE = 'chunks';
const ARCHIVE_KEY = 'region';

function openDB() {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION);
    request.onupgradeneeded = () => {
      const db = request.result;
      if (!db.objectStoreNames.contains(STORE)) db.createObjectStore(STORE);
      if (!db.objectStoreNames.contains(CHUNK_STORE)) db.createObjectStore(CHUNK_STORE);
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

export async function loadArchive() {
  const db = await openDB();
  try {
    return await new Promise((resolve, reject) => {
      const request = db.transaction(STORE, 'readonly').objectStore(STORE).get(ARCHIVE_KEY);
      request.onsuccess = () => resolve(request.result || null);
      request.onerror = () => reject(request.error);
    });
  } finally {
    db.close();
  }
}

async function writeArchive(action) {
  const db = await openDB();
  try {
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(STORE, 'readwrite');
      action(tx.objectStore(STORE));
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
      tx.onabort = () => reject(tx.error);
    });
  } finally {
    db.close();
  }
}

export function saveArchive(blob, meta) {
  return writeArchive((store) => store.put({ blob, meta, savedAt: Date.now() }, ARCHIVE_KEY));
}

export function deleteArchive() {
  return writeArchive((store) => store.delete(ARCHIVE_KEY));
}

// Скачивание с прогрессом: content-length отдаёт сам эндпоинт.
export async function downloadArchive(url, onProgress) {
  const response = await fetch(url, { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(`эндпоинт вернул ${response.status}`);
  }

  const total = Number(response.headers.get('content-length')) || 0;
  const reader = response.body.getReader();
  const chunks = [];
  let received = 0;

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
    received += value.length;
    onProgress(received, total);
  }

  return {
    blob: new Blob(chunks, { type: 'application/vnd.pmtiles' }),
    bytes: received,
  };
}

export async function storageInfo() {
  if (!navigator.storage || !navigator.storage.estimate) return null;
  const { usage = 0, quota = 0 } = await navigator.storage.estimate();
  return { usage, quota };
}

export async function requestPersistence() {
  if (!navigator.storage || !navigator.storage.persist) return false;
  return navigator.storage.persist();
}

// Источник байтов для PMTiles поверх локального Blob.
// Интерфейс совпадает со встроенным FetchSource: getKey() + getBytes().
export function blobSource(blob, key) {
  return {
    getKey() {
      return key;
    },
    async getBytes(offset, length) {
      const data = await blob.slice(offset, offset + length).arrayBuffer();
      return { data };
    },
  };
}

export function formatBytes(bytes) {
  if (!bytes) return '0 Б';
  const units = ['Б', 'КБ', 'МБ', 'ГБ'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
}

// ---------------------------------------------------------------------------
// Кеш кусков архива.
//
// Тайлы по умолчанию читаются HTTP Range-запросами, а ответы 206 Partial Content
// браузеры кешируют ненадёжно (а часто не кешируют вовсе) — поэтому «посмотрел
// онлайн» не означает «останется офлайн». Здесь каждый полученный кусок
// складывается в IndexedDB: при повторном открытии те же диапазоны (заголовок,
// директории, уже просмотренные тайлы) отдаются с диска, без сети.
const chunkKey = (offset, length) => `${offset}:${length}`;

async function readChunk(offset, length) {
  const db = await openDB();
  try {
    return await new Promise((resolve, reject) => {
      const request = db
        .transaction(CHUNK_STORE, 'readonly')
        .objectStore(CHUNK_STORE)
        .get(chunkKey(offset, length));
      request.onsuccess = () => resolve(request.result || null);
      request.onerror = () => reject(request.error);
    });
  } finally {
    db.close();
  }
}

// Пишем пакетом: по одному открытию БД на пачку, иначе на каждый тайл
// получается отдельная транзакция и всё заметно тормозит.
const pendingChunks = [];
let flushTimer = null;

function scheduleFlush() {
  if (flushTimer) return;
  flushTimer = setTimeout(flushChunks, 300);
}

async function flushChunks() {
  flushTimer = null;
  const batch = pendingChunks.splice(0, pendingChunks.length);
  if (!batch.length) return;

  const db = await openDB();
  try {
    await new Promise((resolve, reject) => {
      const tx = db.transaction(CHUNK_STORE, 'readwrite');
      const store = tx.objectStore(CHUNK_STORE);
      for (const item of batch) {
        store.put(item.data, chunkKey(item.offset, item.length));
      }
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
      tx.onabort = () => reject(tx.error);
    });
  } finally {
    db.close();
  }
}

export async function clearChunkCache() {
  const db = await openDB();
  try {
    await new Promise((resolve, reject) => {
      const tx = db.transaction(CHUNK_STORE, 'readwrite');
      tx.objectStore(CHUNK_STORE).clear();
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
    });
  } finally {
    db.close();
  }
}

export async function chunkCacheStats() {
  const db = await openDB();
  try {
    return await new Promise((resolve, reject) => {
      const store = db.transaction(CHUNK_STORE, 'readonly').objectStore(CHUNK_STORE);
      const countReq = store.count();
      let bytes = 0;
      const cursorReq = store.openCursor();
      cursorReq.onsuccess = () => {
        const cursor = cursorReq.result;
        if (cursor) {
          const value = cursor.value;
          if (value instanceof ArrayBuffer) bytes += value.byteLength;
          cursor.continue();
        } else {
          resolve({ count: countReq.result || 0, bytes });
        }
      };
      cursorReq.onerror = () => reject(cursorReq.error);
    });
  } finally {
    db.close();
  }
}

// Источник PMTiles с прозрачным кешированием кусков.
// Сначала смотрит в IndexedDB, при промахе идёт в сеть и сохраняет ответ.
export function cachingSource(inner) {
  return {
    getKey: () => inner.getKey(),
    async getBytes(offset, length, signal, etag) {
      const cached = await readChunk(offset, length).catch(() => null);
      if (cached) {
        return { data: cached };
      }

      const result = await inner.getBytes(offset, length, signal, etag);
      const data = result && result.data;
      if (data) {
        // Копируем: исходный буфер может быть переиспользован/отсоединён.
        const copy = data instanceof ArrayBuffer ? data.slice(0) : new Uint8Array(data).buffer;
        pendingChunks.push({ offset, length, data: copy });
        scheduleFlush();
      }
      return result;
    },
  };
}
