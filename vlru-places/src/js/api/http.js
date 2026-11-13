/**
 * Транспорт демо-API. Сейчас читает статические JSON из /data и имитирует задержку сети.
 * Чтобы подключить настоящий backend, достаточно поменять BASE_URL и пути в модулях api/* —
 * сигнатуры функций и форма ответов описаны в docs/api.md.
 */

const BASE_URL = 'data';
const cache = new Map();

export const delay = (ms = 250) => new Promise((resolve) => setTimeout(resolve, ms));

export async function getJson(path) {
  if (!cache.has(path)) {
    const request = fetch(`${BASE_URL}/${path}`).then((response) => {
      if (!response.ok) throw new Error(`HTTP ${response.status} при загрузке ${path}`);
      return response.json();
    });
    cache.set(path, request);
    request.catch(() => cache.delete(path));
  }
  return cache.get(path);
}

/** Ошибка с кодом для UI: 'auth_required', 'validation', 'not_found'. */
export class ApiError extends Error {
  constructor(code, message) {
    super(message);
    this.code = code;
  }
}
