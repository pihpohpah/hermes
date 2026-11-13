/**
 * Лента «Для вас» в профиле: места, события и маршруты по истории пользователя (api/history.js).
 *
 * Это макет поведения, а не модель: каждый кандидат получает очки за совпадение категории и тегов
 * с тем, что пользователь смотрел или где был, и за совпадение с его поисками. Причина в подписи —
 * один из сильных сигналов кандидата, причём реже других использованный в ленте, чтобы подписи
 * не повторялись. В продакшене ленту отдаёт GET /me/recommendations.
 */

import { delay } from './http.js';
import { listCurrentItems, peekItem } from './places.js';
import { store } from '../core/store.js';
import { haversineKm } from '../core/geo.js';
import { categoryOf } from '../data/taxonomy.js';

const WEIGHT = { visited: 2, viewed: 1 };
const SEARCH_SCORE = 6;
const ROUTE_TITLES = { city: 'Прогулка по городу', nature: 'Выезд на природу', sea: 'День у моря' };
const ROUTE_STOPS = { city: 4, nature: 3, sea: 3 };
const ROUTE_POOL = 10;

const normalize = (value) => String(value || '').toLowerCase().replace(/ё/g, 'е');

function similarity(a, b) {
  const sharedTags = (a.tags || []).filter((tag) => (b.tags || []).includes(tag)).length;
  return (a.category === b.category ? 3 : 0) + sharedTags;
}

function matchesSearch(item, query) {
  const haystack = normalize([item.title, categoryOf(item).label, ...(item.tags || [])].join(' '));
  return normalize(query).split(/\s+/).every((word) => haystack.includes(word.slice(0, -1) || word));
}

function reasonText(signal) {
  if (signal.kind === 'search') return `Вы искали «${signal.query}»`;
  if (signal.kind === 'visited') return `Как «${signal.item.title}», где вы были`;
  return `Вы смотрели «${signal.item.title}»`;
}

/** Очки кандидата и сигналы, которые их дали, от сильного к слабому. */
function score(item, signals) {
  const contributions = [
    ...signals.items.map((signal) => ({ points: similarity(item, signal.item) * WEIGHT[signal.kind], reason: reasonText(signal) })),
    ...signals.searches.map((query) => ({ points: matchesSearch(item, query) ? SEARCH_SCORE : 0, reason: reasonText({ kind: 'search', query }) })),
  ]
    .filter((contribution) => contribution.points > 0)
    .sort((a, b) => b.points - a.points);
  const total = contributions.reduce((sum, contribution) => sum + contribution.points, 0);
  return { item, score: total + (item.featured ? 0.5 : 0), contributions };
}

/** Из сигналов не слабее половины лучшего берёт тот, что реже уже звучал в ленте. */
function assignReasons(entries) {
  const used = new Map();
  return entries.map(({ item, contributions }) => {
    const strong = contributions.filter((contribution) => contribution.points * 2 >= contributions[0].points);
    const { reason } = strong.reduce((best, next) => ((used.get(next.reason) || 0) < (used.get(best.reason) || 0) ? next : best));
    used.set(reason, (used.get(reason) || 0) + 1);
    return { item, reason };
  });
}

function readSignals() {
  const { visited, viewed, searches } = store.get().history;
  const toSignals = (ids, kind) => ids.map((id) => peekItem(id)).filter(Boolean).map((item) => ({ kind, item }));
  return {
    seen: new Set([...visited, ...viewed]),
    items: [...toSignals(visited, 'visited'), ...toSignals(viewed.filter((id) => !visited.includes(id)), 'viewed')],
    searches,
    tags: [...visited, ...viewed].flatMap((id) => peekItem(id)?.tags || []),
  };
}

/** Маршрут из близких друг к другу рекомендованных мест одного сценария. */
function buildRoutes(places, signals) {
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
      const stopTags = new Set(stops.flatMap((stop) => stop.tags || []));
      const themes = [...new Set(signals.tags.filter((tag) => stopTags.has(tag)))].slice(0, 2);
      return {
        id: `rec-route-${scenario}`,
        scenario,
        title: ROUTE_TITLES[scenario],
        reason: themes.length ? `Собрали из того, что вам близко: ${themes.join(', ')}` : 'Собрали из мест, которые вам подойдут',
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
  const signals = readSignals();
  const ranked = items
    .filter((item) => !signals.seen.has(item.id))
    .map((item) => score(item, signals))
    .filter((entry) => entry.contributions.length)
    .sort((a, b) => b.score - a.score);
  const places = ranked.filter((entry) => entry.item.type === 'place');
  return {
    places: assignReasons(places.slice(0, limit)),
    events: assignReasons(ranked.filter((entry) => entry.item.type === 'event').slice(0, 3)),
    routes: buildRoutes(places, signals),
  };
}
