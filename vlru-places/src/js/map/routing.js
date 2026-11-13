/**
 * Построение маршрута между точками.
 *
 * Прототип обращается к открытому демо-сервису OSRM (routing.openstreetmap.de, FOSSGIS) — он годится
 * для показа, но не для продакшена: лимиты и отсутствие SLA. Если сервис недоступен или не нашёл путь
 * (острова без парома, закрытые зоны), маршрут считается по прямой со средней скоростью и помечается
 * как приблизительный — интерфейс показывает это пунктиром и подписью. Подробности — docs/map.md.
 */

import { haversineKm } from '../core/geo.js';

const PROFILES = {
  foot: { url: 'https://routing.openstreetmap.de/routed-foot/route/v1/foot', speedKmh: 4.5, label: 'пешком' },
  car: { url: 'https://routing.openstreetmap.de/routed-car/route/v1/driving', speedKmh: 40, label: 'на машине' },
};

const TIMEOUT_MS = 7000;
const cache = new Map();

function straightLine(stops, mode) {
  const legs = stops.slice(1).map((stop, i) => {
    const km = haversineKm(stops[i].coords, stop.coords) * 1.25; // поправка на непрямолинейность дорог
    return { km, minutes: (km / PROFILES[mode].speedKmh) * 60 };
  });
  return {
    approximate: true,
    geometry: { type: 'LineString', coordinates: stops.map((s) => s.coords) },
    km: legs.reduce((sum, leg) => sum + leg.km, 0),
    minutes: legs.reduce((sum, leg) => sum + leg.minutes, 0),
    legs,
  };
}

/**
 * @param {{id, coords:[lng,lat]}[]} stops — от 2 точек
 * @param {'foot'|'car'} mode
 * @returns {Promise<{approximate, geometry, km, minutes, legs:{km, minutes}[]}>}
 */
export async function buildRoute(stops, mode = 'foot') {
  if (stops.length < 2) return null;
  const key = `${mode}:${stops.map((s) => s.coords.join(',')).join(';')}`;
  if (cache.has(key)) return cache.get(key);

  const coords = stops.map((s) => s.coords.map((n) => n.toFixed(5)).join(',')).join(';');
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), TIMEOUT_MS);

  try {
    const response = await fetch(`${PROFILES[mode].url}/${coords}?overview=full&geometries=geojson&steps=false`, {
      signal: controller.signal,
    });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const data = await response.json();
    const best = data.routes?.[0];
    if (data.code !== 'Ok' || !best) throw new Error(data.code || 'NoRoute');

    const result = {
      approximate: false,
      geometry: best.geometry,
      km: best.distance / 1000,
      minutes: best.duration / 60,
      legs: best.legs.map((leg) => ({ km: leg.distance / 1000, minutes: leg.duration / 60 })),
    };
    cache.set(key, result);
    return result;
  } catch (error) {
    console.warn('Маршрут: сервис недоступен, считаем по прямой', error);
    return straightLine(stops, mode);
  } finally {
    clearTimeout(timer);
  }
}

export const ROUTE_MODES = [
  { id: 'foot', label: 'Пешком', icon: 'footprints' },
  { id: 'car', label: 'Авто', icon: 'car' },
];

export const MAX_STOPS = 6;
