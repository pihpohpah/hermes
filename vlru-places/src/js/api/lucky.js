/**
 * «Мне повезёт»: случайное место, событие или маршрут в выбранном сценарии.
 *
 * Логика намеренно простая — прототип показывает идею. Маршрут начинается со случайного места,
 * а каждая следующая точка — ближайшая к предыдущей; из трёх ближайших предпочитается другая категория,
 * чтобы прогулка не состояла из одних музеев. В продакшене — GET /lucky?type&scenario&stops.
 */

import { delay } from './http.js';
import { listCurrentItems } from './places.js';
import { haversineKm } from '../core/geo.js';

export const LUCKY_TYPES = [
  { id: 'any', label: 'Что угодно', icon: 'sparkles' },
  { id: 'place', label: 'Место', icon: 'map-pin' },
  { id: 'event', label: 'Событие', icon: 'calendar' },
  { id: 'route', label: 'Маршрут', icon: 'route' },
];

const pick = (list) => list[Math.floor(Math.random() * list.length)];

function buildRoute(places, size) {
  const pool = [...places];
  const stops = [pool.splice(Math.floor(Math.random() * pool.length), 1)[0]];
  while (stops.length < size && pool.length) {
    const last = stops[stops.length - 1];
    pool.sort((a, b) => haversineKm(last.coords, a.coords) - haversineKm(last.coords, b.coords));
    const nearest = pool.slice(0, 3);
    const next = nearest.find((place) => place.category !== last.category) || nearest[0];
    pool.splice(pool.indexOf(next), 1);
    stops.push(next);
  }
  return stops.map((stop, i) => ({ item: stop, fromPreviousKm: i ? haversineKm(stops[i - 1].coords, stop.coords) : null }));
}

/** Кандидаты для анимации перебора и для броска. */
export async function getLuckyPool(scenario = 'any') {
  const items = await listCurrentItems();
  return items.filter((item) => scenario === 'any' || item.scenario === scenario);
}

/**
 * @param {object} params
 * @param {'any'|'place'|'event'|'route'} params.type
 * @param {'any'|'city'|'nature'|'sea'} params.scenario
 * @param {number} [params.stops] — точек в маршруте, 2–6
 * @param {string} [params.exclude] — id прошлого результата, чтобы «Ещё раз» не повторялся
 * @returns {Promise<null | {type: 'place'|'event', item} | {type: 'route', stops: {item, fromPreviousKm}[]}>}
 */
export async function rollLucky({ type = 'any', scenario = 'any', stops = 3, exclude = '' } = {}) {
  const pool = await getLuckyPool(scenario);
  await delay(150);
  const places = pool.filter((item) => item.type === 'place');
  const events = pool.filter((item) => item.type === 'event');
  const available = { place: places.length > 0, event: events.length > 0, route: places.length >= 2 };
  const resolved = type === 'any' ? pick(Object.keys(available).filter((key) => available[key])) : type;
  if (!resolved || !available[resolved]) return null;

  if (resolved === 'route') return { type: 'route', stops: buildRoute(places, Math.min(stops, places.length)) };
  const list = resolved === 'place' ? places : events;
  const fresh = list.length > 1 ? list.filter((item) => item.id !== exclude) : list;
  return { type: resolved, item: pick(fresh) };
}
