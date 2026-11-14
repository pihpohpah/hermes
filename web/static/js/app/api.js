/* Доступ к API: обёртка {success, error, data}, CSRF, кэш карточек.
 * Мутации идут через fetch (htmx отвечает за GET-фрагменты); csrf-токен
 * бэкенд выдаёт после входа POST /users. */
import { storage } from './util.js';

export const CSRF_KEY = 'vlru-places:csrf';

export class ApiError extends Error {
  constructor(message, { status = 0, code = '' } = {}) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export async function api(path, { method = 'GET', body, formData } = {}) {
  const headers = { Accept: 'application/json' };
  const token = storage.get(CSRF_KEY, '');
  if (token) headers['X-CSRF-Token'] = token;

  const options = { method, headers, credentials: 'same-origin' };
  if (formData) {
    options.body = formData;
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(body);
  }

  const response = await fetch(path, options);
  let payload = null;
  try {
    payload = await response.json();
  } catch {
    /* пустой или не-JSON ответ обрабатываем по статусу */
  }

  if (payload && typeof payload === 'object' && 'success' in payload) {
    if (payload.success) return payload.data;
    throw new ApiError(payload.error || 'Что-то пошло не так', { status: response.status, code: payload.code || '' });
  }
  if (!response.ok) throw new ApiError(`HTTP ${response.status}`, { status: response.status });
  return payload;
}

const itemCache = new Map();

export async function fetchItem(id) {
  if (itemCache.has(id)) return itemCache.get(id);
  const item = await api(`/api/items/${encodeURIComponent(id)}`);
  if (item) {
    itemCache.set(
      id,
      item.has_coords
        ? { id: item.id, title: item.title, coords: [item.lon, item.lat], url: item.type === 'event' ? `/event/${item.id}` : `/place/${item.id}` }
        : { id: item.id, title: item.title, coords: null, url: item.type === 'event' ? `/event/${item.id}` : `/place/${item.id}` },
    );
  }
  return itemCache.get(id) || null;
}
