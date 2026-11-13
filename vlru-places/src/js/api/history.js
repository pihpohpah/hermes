/**
 * Сигналы для рекомендаций: что пользователь смотрел, где был и что искал.
 * В демо они лежат в store.history (localStorage) и копятся и до входа. В продакшене это события
 * POST /me/events, а история хранится на сервере.
 */

import { store } from '../core/store.js';

const LIMIT = 30;
const SEARCH_LIMIT = 10;
const MIN_QUERY = 3;

const update = (patch) => store.set({ history: { ...store.get().history, ...patch } });

const pushFront = (list, value, limit = LIMIT) => [value, ...list.filter((other) => other !== value)].slice(0, limit);

export function trackView(id) {
  const { viewed } = store.get().history;
  if (viewed[0] !== id) update({ viewed: pushFront(viewed, id) });
}

/**
 * Поиск запускается на каждый ввод с задержкой, поэтому «вод» → «водопады» — один запрос:
 * продолжение последнего запроса заменяет его, а стёртые буквы не записываются.
 */
export function trackSearch(query) {
  const q = String(query || '').trim().toLowerCase().replace(/ё/g, 'е');
  if (q.length < MIN_QUERY) return;
  const [last = '', ...rest] = store.get().history.searches;
  if (last.startsWith(q)) return;
  const previous = q.startsWith(last) ? rest : [last, ...rest];
  update({ searches: pushFront(previous, q, SEARCH_LIMIT) });
}

export const isVisited = (id) => store.get().history.visited.includes(id);

/** Отметка «Уже были здесь». Возвращает новое состояние. */
export function toggleVisited(id) {
  const { visited } = store.get().history;
  const next = !visited.includes(id);
  update({ visited: next ? pushFront(visited, id) : visited.filter((other) => other !== id) });
  return next;
}
