/**
 * Лента «Для вас» в профиле.
 *
 * TODO(mock, рекомендательная система): это временная заглушка, а не рекомендации. Кандидаты берутся
 * случайным образом: без скоринга, без фильтров и без учёта истории пользователя (просмотры,
 * посещения и поиски наполняют store.history, но здесь не используются — лента не «подстраивается»).
 * Персонализированные причины в подписях карточек тоже не выдумываем, чтобы макет не вводил в заблуждение.
 * В продакшене ленту должен отдавать настоящий рекомендательный сервис: GET /me/recommendations.
 */

import { delay } from './http.js';
import { listCurrentItems } from './places.js';
import { haversineKm } from '../core/geo.js';

const EVENT_LIMIT = 3;
const ROUTE_TITLES = { city: 'Прогулка по городу', nature: 'Выезд на природу', sea: 'День у моря' };
const ROUTE_STOPS = { city: 4, nature: 3, sea: 3 };
const ROUTE_POOL = 10;
const ROUTE_REASON = 'Случайный маршрут: точки подобраны близко друг к другу';

/** Тасование Фишера — Йетса. */
function shuffle(list) {
  const copy = [...list];
  for (let i = copy.length - 1; i > 0; i -= 1) {
    const j = Math.floor(Math.random() * (i + 1));
    [copy[i], copy[j]] = [copy[j], copy[i]];
  }
  return copy;
}

/** Маршрут из случайных мест одного сценария: каждая следующая точка — ближайшая к предыдущей. */
function buildRoutes(places) {
  const byScenario = {};
  places.forEach((entry) => (byScenario[entry.item.scenario] ??= []).push(entry));
  return Object.entries(byScenario)
    .filter(([scenario, entries]) => entries.length >= ROUTE_STOPS[scenario])
    .slice(0, 2)
    .map(([scenario, entries]) => {
      const pool = entries.slice(0, ROUTE_POOL).map((entry) => entry.item);
      const stops = [pool.shift()];
      while (stops.length < ROUTE_STOPS[scenario] && pool.length) {
        const last = stops[stops.length - 1];
        pool.sort((a, b) => haversineKm(last.coords, a.coords) - haversineKm(last.coords, b.coords));
        stops.push(pool.shift());
      }
      return {
        id: `rec-route-${scenario}`,
        scenario,
        title: ROUTE_TITLES[scenario],
        reason: ROUTE_REASON,
        stops,
      };
    });
}

/**
 * @returns {Promise<{ places: {item, reason}[], events: {item, reason}[], routes: {id, scenario, title, reason, stops}[] }>}
 */
export async function getRecommendations({ limit = 8 } = {}) {
  const items = await listCurrentItems();
  await delay(250);
  const ranked = shuffle(items).map((item) => ({ item, reason: '' }));
  const places = ranked.filter((entry) => entry.item.type === 'place');
  return {
    places: places.slice(0, limit),
    events: ranked.filter((entry) => entry.item.type === 'event').slice(0, EVENT_LIMIT),
    routes: buildRoutes(places),
  };
}
