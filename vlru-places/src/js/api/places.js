/**
 * Места и события.
 * listItems — поиск и фильтрация для карты и справочника; getItem — карточка; getSimilar — «Похожие».
 * Фильтрация сейчас выполняется на клиенте; в продакшене те же параметры уходят в запрос к серверу.
 */

import { getJson, delay, ApiError } from './http.js';
import { distanceFromCenter } from '../core/geo.js';
import { COLLECTIONS, SCENARIOS } from '../data/taxonomy.js';
import { trackSearch } from './history.js';

let dataset = null;

async function load() {
  if (dataset) return dataset;
  const [places, events] = await Promise.all([getJson('places.json'), getJson('events.json')]);
  const placeById = new Map(places.map((place) => [place.id, place]));

  const normalizedPlaces = places.map((place) => ({
    ...place,
    type: 'place',
    distanceKm: distanceFromCenter(place.coords),
  }));

  const normalizedEvents = events.map((event) => {
    const place = event.placeId ? placeById.get(event.placeId) : null;
    const coords = event.coords || place?.coords;
    return {
      ...event,
      type: 'event',
      coords,
      photos: event.photos?.length ? event.photos : place?.photos || [],
      district: event.district || place?.district || '',
      distanceKm: coords ? distanceFromCenter(coords) : 0,
    };
  });

  dataset = {
    items: [...normalizedPlaces, ...normalizedEvents],
    byId: new Map([...normalizedPlaces, ...normalizedEvents].map((item) => [item.id, item])),
  };
  return dataset;
}

const normalize = (value) =>
  String(value || '')
    .toLowerCase()
    .replace(/ё/g, 'е')
    .trim();

function matchesQuery(item, query) {
  if (!query) return true;
  const haystack = normalize(
    [item.title, item.subtitle, item.short, item.district, item.address, item.venue, ...(item.tags || [])].join(' '),
  );
  return normalize(query)
    .split(/\s+/)
    .every((word) => haystack.includes(word));
}

/** Сравнение с фильтрами из FILTERS (src/js/data/taxonomy.js). */
export function matchesFilters(item, filters = {}) {
  if (filters.free && !item.price?.free) return false;
  if (filters.walkable && item.distanceKm > 2.5) return false;
  if (filters.kids && !(item.tags || []).includes('с детьми')) return false;
  if (filters.distance && item.distanceKm > Number(filters.distance)) return false;
  if (filters.difficulty?.length && item.type === 'place' && !filters.difficulty.includes(item.difficulty)) return false;
  if (filters.noCar && item.type === 'place' && !(item.access || []).some((a) => ['walk', 'transit', 'ferry'].includes(a))) return false;
  if (filters.beach?.length && item.type === 'place' && !filters.beach.includes(item.beach)) return false;
  if (filters.swim && item.type === 'place' && !item.swimSeason) return false;
  return true;
}

export function countActiveFilters(filters = {}) {
  return Object.values(filters).filter((value) => (Array.isArray(value) ? value.length : Boolean(value))).length;
}

const SORTERS = {
  recommended: (a, b) => Number(b.featured || 0) - Number(a.featured || 0) || Number(Boolean(b.photos?.length)) - Number(Boolean(a.photos?.length)) || a.title.localeCompare(b.title, 'ru'),
  distance: (a, b) => a.distanceKm - b.distanceKm,
  title: (a, b) => a.title.localeCompare(b.title, 'ru'),
  date: (a, b) => new Date(a.startsAt) - new Date(b.startsAt),
};

/**
 * @param {object} params
 * @param {'city'|'nature'|'sea'|'all'} params.scenario
 * @param {'all'|'place'|'event'} [params.type]
 * @param {string} [params.q]            — поисковая строка
 * @param {string} [params.category]     — id категории
 * @param {object} [params.filters]      — значения фильтров сценария
 * @param {string} [params.collection]   — id подборки
 * @param {string} [params.sort]
 * @returns {Promise<{items, total, categoryCounts, typeCounts}>}
 */
export async function listItems({ scenario = 'city', type = 'all', q = '', category = '', filters = {}, collection = '', sort = 'recommended', latency = 180 } = {}) {
  const { items } = await load();
  await delay(latency);
  trackSearch(q);
  const now = Date.now();

  const inScope = items.filter(
    (item) =>
      (scenario === 'all' || item.scenario === scenario) &&
      (item.type !== 'event' || new Date(item.endsAt || item.startsAt).getTime() > now) &&
      matchesQuery(item, q) &&
      matchesFilters(item, filters) &&
      (!collection || COLLECTIONS.find((c) => c.id === collection)?.match(item)),
  );

  const typeCounts = {
    all: inScope.length,
    place: inScope.filter((item) => item.type === 'place').length,
    event: inScope.filter((item) => item.type === 'event').length,
  };

  const byType = inScope.filter((item) => type === 'all' || item.type === type);

  const categoryCounts = {};
  byType.forEach((item) => {
    if (item.type === 'place') categoryCounts[item.category] = (categoryCounts[item.category] || 0) + 1;
  });

  const result = byType
    .filter((item) => !category || (item.type === 'place' && item.category === category))
    .sort(SORTERS[sort] || SORTERS.recommended);

  return { items: result, total: result.length, categoryCounts, typeCounts };
}

export async function getItem(id) {
  const { byId } = await load();
  await delay(120);
  const item = byId.get(id);
  if (!item) throw new ApiError('not_found', `Объект ${id} не найден`);
  return item;
}

/** Синхронный доступ после первой загрузки — для маркеров и маршрута. */
export function peekItem(id) {
  return dataset?.byId.get(id) || null;
}

export async function getSimilar(item, limit = 4) {
  const { items } = await load();
  const now = Date.now();
  return items
    .filter((other) => other.id !== item.id && other.scenario === item.scenario && (other.type !== 'event' || new Date(other.startsAt).getTime() > now))
    .map((other) => ({
      other,
      score: (other.category === item.category ? 3 : 0) + (other.district === item.district ? 2 : 0) + (other.tags || []).filter((t) => (item.tags || []).includes(t)).length - other.distanceKm / 500,
    }))
    .sort((a, b) => b.score - a.score)
    .slice(0, limit)
    .map(({ other }) => other);
}

/** События, привязанные к месту. */
export async function getEventsAt(placeId) {
  const { items } = await load();
  const now = Date.now();
  return items.filter((item) => item.type === 'event' && item.placeId === placeId && new Date(item.startsAt).getTime() > now).sort(SORTERS.date);
}

/** Прошедшие события не показываются в выдаче — и не должны попадать в счётчики. */
const isCurrent = (item, now = Date.now()) => item.type !== 'event' || new Date(item.endsAt || item.startsAt).getTime() > now;

export async function getCollectionCounts(scenario = 'all') {
  const { items } = await load();
  return Object.fromEntries(
    COLLECTIONS.map((collection) => [
      collection.id,
      items.filter((item) => isCurrent(item) && (scenario === 'all' || item.scenario === scenario) && collection.match(item)).length,
    ]),
  );
}

/** Все актуальные места и события — сырьё для рекомендаций и «Мне повезёт». */
export async function listCurrentItems() {
  const { items } = await load();
  const now = Date.now();
  return items.filter((item) => isCurrent(item, now));
}

export async function getScenarioCounts() {
  const { items } = await load();
  return Object.fromEntries(SCENARIOS.map((s) => [s.id, items.filter((item) => isCurrent(item) && item.scenario === s.id).length]));
}
