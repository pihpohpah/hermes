/* Тайлы карты: PMTiles-протокол для MapLibre и офлайн-хранилище на устройстве.
 *
 * Архивы читаются у нашего сервера HTTP Range-запросами: сначала всегда сеть.
 * Каждый полученный кусок тихо складывается в IndexedDB, поэтому просмотренные
 * области открываются и без сети. Из профиля архивы можно скачать целиком —
 * тогда без сети работает вся карта. Локальные данные — только запасной путь
 * на случай, когда сервер недоступен.
 *
 * Куски и копии привязаны к ETag архива: после пересборки архива на сервере
 * старые смещения указывают в другие байты, и смешивать версии нельзя. */

export const ARCHIVES = [
  { id: 'region', url: '/api/map/region.pmtiles', required: true },
  { id: 'relief', url: '/api/map/relief.pmtiles', required: false },
];

const DB_NAME = 'hermes-map';
// v3: куски и копии ключуются ETag-ом; несовместимые данные демо (v1–v2) сбрасываются.
const DB_VERSION = 3;
const COPIES = 'archives';
const CHUNKS = 'chunks';
const META = 'meta';
const FLUSH_DELAY_MS = 300;

/* ── IndexedDB ── */

let dbPromise = null;

function openDb() {
  if (dbPromise) return dbPromise;
  dbPromise = new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION);
    request.onupgradeneeded = (event) => {
      const db = request.result;
      if (event.oldVersion < 3) {
        for (const name of Array.from(db.objectStoreNames)) db.deleteObjectStore(name);
      }
      db.createObjectStore(COPIES);
      db.createObjectStore(CHUNKS);
      db.createObjectStore(META);
    };
    request.onsuccess = () => {
      const db = request.result;
      db.onversionchange = () => {
        db.close();
        dbPromise = null;
      };
      resolve(db);
    };
    request.onerror = () => {
      dbPromise = null;
      reject(request.error);
    };
  });
  return dbPromise;
}

/** Транзакция над сторами; результат — то, что вернула fn (для IDBRequest — его result). */
async function transact(stores, mode, fn) {
  const db = await openDb();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(stores, mode);
    const result = fn(tx);
    tx.oncomplete = () => resolve(result instanceof IDBRequest ? result.result : result);
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

const chunkKey = (id, etag, offset, length) => `${id}|${etag}|${offset}:${length}`;
const chunkRange = (id) => IDBKeyRange.bound(`${id}|`, `${id}|\uffff`);

/* ── Скачанные копии ── */

const copyCache = new Map();

/** Копия архива: { blob, etag, bytes, savedAt } или null. */
export function loadCopy(id) {
  if (!copyCache.has(id)) {
    copyCache.set(
      id,
      transact(COPIES, 'readonly', (tx) => tx.objectStore(COPIES).get(id)).then(
        (copy) => copy || null,
        (error) => {
          console.warn('Карта: не удалось прочитать офлайн-копию', id, error);
          copyCache.delete(id);
          return null;
        },
      ),
    );
  }
  return copyCache.get(id);
}

async function saveCopy(id, copy) {
  await transact(COPIES, 'readwrite', (tx) => tx.objectStore(COPIES).put(copy, id));
  copyCache.set(id, Promise.resolve(copy));
}

export async function deleteCopies() {
  await transact(COPIES, 'readwrite', (tx) => tx.objectStore(COPIES).clear());
  copyCache.clear();
}

/* ── Кеш кусков ── */

// Версия (ETag) кусков, лежащих в кеше, по архивам. Кеш хранит одну версию:
// пришёл кусок с новым ETag — старые куски архива удаляются.
const chunkVersions = new Map();
const pendingChunks = [];
let flushTimer = null;

function chunkVersion(id) {
  if (!chunkVersions.has(id)) {
    chunkVersions.set(
      id,
      transact(META, 'readonly', (tx) => tx.objectStore(META).get(`version:${id}`)).catch((error) => {
        console.warn('Карта: не удалось прочитать версию кеша', id, error);
        return null;
      }),
    );
  }
  return chunkVersions.get(id);
}

async function rememberChunk(id, etag, offset, length, data) {
  if (!etag || !data) return;
  const known = await chunkVersion(id);
  if (known !== etag) {
    chunkVersions.set(id, Promise.resolve(etag));
    await transact([META, CHUNKS], 'readwrite', (tx) => {
      tx.objectStore(META).put(etag, `version:${id}`);
      tx.objectStore(CHUNKS).delete(chunkRange(id));
    });
  }
  // Копия: исходный буфер может быть передан воркеру и отсоединён.
  pendingChunks.push({ key: chunkKey(id, etag, offset, length), data: data.slice(0) });
  if (!flushTimer) flushTimer = setTimeout(flushChunks, FLUSH_DELAY_MS);
}

// Пишем пачкой: транзакция на каждый тайл заметно тормозит карту.
async function flushChunks() {
  flushTimer = null;
  const batch = pendingChunks.splice(0, pendingChunks.length);
  if (!batch.length) return;
  try {
    await transact(CHUNKS, 'readwrite', (tx) => {
      const store = tx.objectStore(CHUNKS);
      for (const item of batch) store.put(item.data, item.key);
    });
  } catch (error) {
    console.warn('Карта: не удалось сохранить куски в кеш', error);
  }
}

function readChunk(id, etag, offset, length) {
  return transact(CHUNKS, 'readonly', (tx) => tx.objectStore(CHUNKS).get(chunkKey(id, etag, offset, length)));
}

export async function clearChunkCache() {
  pendingChunks.length = 0;
  await transact([CHUNKS, META], 'readwrite', (tx) => {
    tx.objectStore(CHUNKS).clear();
    tx.objectStore(META).clear();
  });
  chunkVersions.clear();
}

export async function chunkCacheBytes() {
  return transact(CHUNKS, 'readonly', (tx) => {
    const total = { bytes: 0 };
    tx.objectStore(CHUNKS).openCursor().onsuccess = (event) => {
      const cursor = event.target.result;
      if (!cursor) return;
      total.bytes += cursor.value?.byteLength || 0;
      cursor.continue();
    };
    return total;
  }).then((total) => total.bytes);
}

/* ── Источник PMTiles: сначала сеть, потом устройство ── */

/** Байты из устройства в той же версии архива, что уже читает PMTiles. */
async function readLocal(id, offset, length, etag) {
  const copy = await loadCopy(id);
  // Заголовок (etag ещё неизвестен) берём из копии: дальше вся сессия читает её версию.
  if (copy && (!etag || etag === copy.etag)) {
    return { data: await copy.blob.slice(offset, offset + length).arrayBuffer(), etag: copy.etag };
  }
  const version = etag || (await chunkVersion(id));
  if (!version) return null;
  const data = await readChunk(id, version, offset, length).catch((error) => {
    console.warn('Карта: не удалось прочитать кусок из кеша', id, error);
    return null;
  });
  return data ? { data, etag: version } : null;
}

class OnlineFirstSource {
  constructor(archive) {
    this.archive = archive;
    this.network = new window.pmtiles.FetchSource(archive.url);
  }

  getKey() {
    return this.archive.url;
  }

  async getBytes(offset, length, signal, etag) {
    let networkError = null;
    if (navigator.onLine !== false) {
      try {
        const result = await this.network.getBytes(offset, length, signal, etag);
        rememberChunk(this.archive.id, result.etag, offset, length, result.data).catch((error) =>
          console.warn('Карта: не удалось обновить кеш', error),
        );
        return result;
      } catch (error) {
        if (error?.name === 'AbortError' || error instanceof window.pmtiles.EtagMismatch) throw error;
        networkError = error;
      }
    }
    const local = await readLocal(this.archive.id, offset, length, etag);
    if (local) return local;
    throw networkError || new Error(`Нет сети, а нужного куска архива ${this.archive.id} нет на устройстве`);
  }
}

let protocolReady = false;

/** Регистрирует протокол pmtiles:// в MapLibre. Повторный вызов ничего не делает. */
export function ensureMapProtocol() {
  if (protocolReady) return;
  if (!window.maplibregl || !window.pmtiles) throw new Error('MapLibre или PMTiles не загружены');
  const protocol = new window.pmtiles.Protocol();
  window.maplibregl.addProtocol('pmtiles', protocol.tile);
  for (const archive of ARCHIVES) protocol.add(new window.pmtiles.PMTiles(new OnlineFirstSource(archive)));
  protocolReady = true;
}

/* ── Скачивание целиком (профиль) ── */

async function headArchive(archive) {
  const response = await fetch(archive.url, { method: 'HEAD', cache: 'no-store' });
  if (!response.ok) {
    if (archive.required) throw new Error(`архив карты недоступен: ${response.status}`);
    return null;
  }
  return { archive, etag: response.headers.get('ETag'), bytes: Number(response.headers.get('Content-Length')) || 0 };
}

/** Какие архивы есть на сервере и сколько они весят. */
export async function serverArchives() {
  const heads = await Promise.all(ARCHIVES.map(headArchive));
  return heads.filter(Boolean);
}

/** Есть ли на сервере версия новее скачанной. */
export async function copiesOutdated() {
  const heads = await serverArchives();
  for (const head of heads) {
    const copy = await loadCopy(head.archive.id);
    if (!copy || copy.etag !== head.etag) return true;
  }
  return false;
}

/**
 * Скачивает все доступные архивы в IndexedDB.
 * onProgress(received, total) — суммарно по архивам; signal отменяет загрузку.
 */
export async function downloadMap({ onProgress, signal } = {}) {
  const heads = await serverArchives();
  const total = heads.reduce((sum, head) => sum + head.bytes, 0);
  let received = 0;

  const downloaded = [];
  for (const head of heads) {
    const response = await fetch(head.archive.url, { cache: 'no-store', signal });
    if (!response.ok) throw new Error(`архив ${head.archive.id}: ответ ${response.status}`);
    const reader = response.body.getReader();
    const parts = [];
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      parts.push(value);
      received += value.length;
      onProgress?.(received, total);
    }
    const blob = new Blob(parts, { type: 'application/vnd.pmtiles' });
    downloaded.push({ id: head.archive.id, copy: { blob, etag: response.headers.get('ETag'), bytes: blob.size, savedAt: Date.now() } });
  }

  // Сохраняем только когда скачано всё: половина набора хуже прежней копии.
  for (const { id, copy } of downloaded) await saveCopy(id, copy);
  if (navigator.storage?.persist) {
    navigator.storage.persist().catch((error) => console.warn('Карта: хранилище не стало постоянным', error));
  }
}

/** Состояние офлайн-данных для профиля. */
export async function offlineStatus() {
  const copies = (await Promise.all(ARCHIVES.map((archive) => loadCopy(archive.id)))).filter(Boolean);
  return {
    copyBytes: copies.reduce((sum, copy) => sum + copy.bytes, 0),
    savedAt: copies.length ? Math.max(...copies.map((copy) => copy.savedAt)) : null,
    chunkBytes: await chunkCacheBytes().catch((error) => {
      console.warn('Карта: не удалось посчитать кеш', error);
      return 0;
    }),
  };
}
