/* Маршрут: построение пути (OSRM с фолбэком на прямую), состояние и
 * Alpine-компоненты (store, плашка, редактор). */
import { countLabel, currentParams, formatDistance, formatDuration, storage } from './util.js';
import { toast } from './ui.js';
import { fetchItem } from './api.js';
import { panelSwap } from './nav.js';

export const ROUTE_MODES = [
  { id: 'foot', label: 'Пешком' },
  { id: 'car', label: 'Авто' },
];

export const MAX_STOPS = 6;

const OSRM_PROFILES = {
  foot: { url: 'https://routing.openstreetmap.de/routed-foot/route/v1/foot', speedKmh: 4.5 },
  car: { url: 'https://routing.openstreetmap.de/routed-car/route/v1/driving', speedKmh: 40 },
};

const ROUTE_TIMEOUT_MS = 7000;
const routeCache = new Map();

function haversineKm(a, b) {
  const rad = Math.PI / 180;
  const dLat = (b[1] - a[1]) * rad;
  const dLon = (b[0] - a[0]) * rad;
  const s = Math.sin(dLat / 2) ** 2 + Math.cos(a[1] * rad) * Math.cos(b[1] * rad) * Math.sin(dLon / 2) ** 2;
  return 2 * 6371 * Math.asin(Math.sqrt(s));
}

function straightLine(stops, mode) {
  const profile = OSRM_PROFILES[mode] || OSRM_PROFILES.foot;
  const legs = stops.slice(1).map((stop, i) => {
    const km = haversineKm(stops[i].coords, stop.coords) * 1.25; // поправка на непрямолинейность дорог
    return { km, minutes: (km / profile.speedKmh) * 60 };
  });
  return {
    approximate: true,
    geometry: { type: 'LineString', coordinates: stops.map((s) => s.coords) },
    km: legs.reduce((sum, leg) => sum + leg.km, 0),
    minutes: legs.reduce((sum, leg) => sum + leg.minutes, 0),
    legs,
  };
}

/** @returns {Promise<{approximate, geometry, km, minutes, legs}>|null} */
async function buildRoute(stops, mode = 'foot') {
  if (stops.length < 2) return null;
  const key = `${mode}:${stops.map((s) => s.coords.join(',')).join(';')}`;
  if (routeCache.has(key)) return routeCache.get(key);

  const coords = stops.map((s) => s.coords.map((n) => n.toFixed(5)).join(',')).join(';');
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), ROUTE_TIMEOUT_MS);

  try {
    const response = await fetch(`${OSRM_PROFILES[mode].url}/${coords}?overview=full&geometries=geojson&steps=false`, {
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
    routeCache.set(key, result);
    return result;
  } catch (error) {
    console.warn('Маршрут: сервис недоступен, считаем по прямой', error);
    return straightLine(stops, mode);
  } finally {
    clearTimeout(timer);
  }
}

export const ROUTE_KEY = 'vlru-places:route';
export const ROUTE_MODE_KEY = 'vlru-places:route-mode';

export const routeState = {
  stops: [],
  result: null,
  computing: false,
  version: 0,
  listeners: new Set(),
  subscribe(fn) {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  },
  notify() {
    this.listeners.forEach((fn) => fn(this));
  },
  async recompute() {
    const current = ++this.version;
    const ids = storage.get(ROUTE_KEY, []);
    const mode = storage.get(ROUTE_MODE_KEY, 'foot');
    this.computing = ids.length >= 2;

    const loaded = await Promise.all(ids.map((id) => fetchItem(id).catch(() => null)));
    if (current !== this.version) return;
    this.stops = loaded.filter(Boolean);
    if (this.stops.length !== ids.length) persistRoute(this.stops.map((s) => s.id));

    this.result = this.stops.length >= 2 ? await buildRoute(this.stops, mode) : null;
    if (current !== this.version) return;
    this.computing = false;

    paintRouteOnMap();
    this.notify();
    announceRoute();
  },
};

function persistRoute(ids) {
  storage.set(ROUTE_KEY, ids);
  routeState.recompute();
}

export function toggleRoute(id, title = '') {
  const ids = storage.get(ROUTE_KEY, []);
  if (ids.includes(id)) {
    storage.set(ROUTE_KEY, ids.filter((stopId) => stopId !== id));
    routeState.recompute();
    toast(`Убрали из маршрута${title ? `: ${title}` : ''}`, { group: 'route' });
    return;
  }
  if (ids.length >= MAX_STOPS) {
    toast(`В маршруте может быть до ${MAX_STOPS} точек`, { type: 'danger', group: 'route' });
    return;
  }
  storage.set(ROUTE_KEY, [...ids, id]);
  routeState.recompute();
  const count = ids.length + 1;
  toast(count === 1 ? 'Первая точка маршрута. Добавьте ещё одну' : `В маршруте ${countLabel(count, ['точка', 'точки', 'точек'])}`, {
    type: 'success',
    group: 'route',
    action: count >= 2 ? { label: 'Показать', onClick: () => openRouteEditor() } : null,
  });
}

export function clearRoute() {
  storage.set(ROUTE_KEY, []);
  routeState.recompute();
}

/** Полная замена состава маршрута (например, «Мне повезёт» подставил готовый). */
export function replaceRoute(ids) {
  storage.set(ROUTE_KEY, ids.slice(0, MAX_STOPS));
  routeState.recompute();
}

export function setRouteMode(mode) {
  storage.set(ROUTE_MODE_KEY, mode);
  routeState.recompute();
}

export function paintRouteOnMap() {
  if (!window.placesMap) return;
  const ids = storage.get(ROUTE_KEY, []);
  window.placesMap.setRoute(
    ids.length
      ? { stops: routeState.stops, geometry: routeState.result?.geometry || null, approximate: routeState.result?.approximate || false }
      : null,
  );
}

/** Озвучка итога маршрута для скринридера. */
function announceRoute() {
  const announcer = document.querySelector('[data-route-announcer]');
  if (!announcer) return;
  const ids = storage.get(ROUTE_KEY, []);
  if (!ids.length) {
    announcer.textContent = '';
    return;
  }
  announcer.textContent = `Маршрут, ${countLabel(ids.length, ['точка', 'точки', 'точек'])}: ${routeSummaryText()}`;
}

export function routeSummaryText() {
  const { stops, result, computing } = routeState;
  if (stops.length < 2) return 'Добавьте ещё одну точку';
  if (computing || !result) return 'Считаем маршрут…';
  const mode = storage.get(ROUTE_MODE_KEY, 'foot') === 'car' ? 'на машине' : 'пешком';
  return `${formatDistance(result.km)} · ${formatDuration(result.minutes)} ${mode}${result.approximate ? ', примерно' : ''}`;
}

/** URL редактора маршрута: текущая страница с panel=route. */
function routeEditorURL() {
  const params = new URLSearchParams(location.search);
  params.set('panel', 'route');
  return `${location.pathname}?${params.toString()}`;
}

export function openRouteEditor() {
  htmx.ajax('GET', `/fragments/map-panel?${new URLSearchParams(location.search).toString()}`, { target: '#map-panel-inner' });
  const params = new URLSearchParams(location.search);
  params.set('panel', 'route');
  history.pushState({}, '', `${location.pathname}?${params.toString()}`);
}

export function registerRoute() {
  // Состав маршрута — в Alpine store, чтобы x-bind:aria-pressed на кнопках
  // «В маршрут» реагировал на изменения без ручной перерисовки.
  Alpine.store('route', {
    ids: storage.get(ROUTE_KEY, []),
    has(id) {
      return this.ids.includes(id);
    },
  });

  const syncRouteStore = () => {
    Alpine.store('route').ids = storage.get(ROUTE_KEY, []);
  };

  const routeEditorOpen = () => currentParams().get('panel') === 'route';

  Alpine.data('routeBar', () => ({
    visible: false,
    stops: [],
    summary: '',
    modes: ROUTE_MODES,
    mode: storage.get(ROUTE_MODE_KEY, 'foot'),
    editorURL: '/?panel=route',
    init() {
      const refresh = () => {
        this.visible = Alpine.store('route').ids.length > 0 && !routeEditorOpen();
        document.getElementById('map-stage')?.classList.toggle('has-route', this.visible);
      };
      this.$watch('$store.route.ids', () => {
        refresh();
        routeState.recompute();
      });
      this.unsubscribe = routeState.subscribe(() => {
        this.stops = routeState.stops;
        this.summary = routeSummaryText();
        this.mode = storage.get(ROUTE_MODE_KEY, 'foot');
        this.editorURL = routeEditorURL();
      });
      refresh();
      routeState.recompute();
    },
    stopsTitle() {
      return `Маршрут · ${countLabel(this.stops.length, ['точка', 'точки', 'точек'])}`;
    },
    setMode(mode) {
      this.mode = mode;
      setRouteMode(mode);
    },
    destroy() {
      this.unsubscribe?.();
    },
  }));

  Alpine.data('routeEditor', () => ({
    stops: [],
    modes: ROUTE_MODES,
    mode: storage.get(ROUTE_MODE_KEY, 'foot'),
    distance: '…',
    duration: '…',
    approximate: false,
    yandexUrl: '',
    init() {
      this.unsubscribe = routeState.subscribe(() => this.paint());
      this.paint();
      routeState.recompute().then(() => {
        const coords = routeState.stops.map((s) => s.coords).filter(Boolean);
        if (coords.length) window.placesMap?.fitTo(coords);
      });
    },
    paint() {
      const legs = routeState.result?.legs || [];
      this.stops = routeState.stops.map((stop, index) => ({
        ...stop,
        leg: legs[index] ? `→ ${formatDistance(legs[index].km)}, ${formatDuration(legs[index].minutes)}` : '',
      }));
      this.mode = storage.get(ROUTE_MODE_KEY, 'foot');
      this.approximate = Boolean(routeState.result?.approximate);
      this.distance = routeState.computing || !routeState.result ? '…' : formatDistance(routeState.result.km);
      this.duration = routeState.computing || !routeState.result ? '…' : formatDuration(routeState.result.minutes);
      const coords = this.stops.map((s) => s.coords).filter(Boolean).map((c) => `${c[1]},${c[0]}`).join('~');
      this.yandexUrl = coords ? `https://yandex.ru/maps/?rtext=${coords}&rtt=${this.mode === 'car' ? 'auto' : 'pd'}` : '';
    },
    setMode(mode) {
      setRouteMode(mode);
    },
    move(index, delta) {
      const ids = storage.get(ROUTE_KEY, []);
      const target = index + delta;
      if (index < 0 || target < 0 || target >= ids.length) return;
      [ids[index], ids[target]] = [ids[target], ids[index]];
      storage.set(ROUTE_KEY, ids);
      routeState.recompute();
    },
    remove(id) {
      toggleRoute(id);
      syncRouteStore();
    },
    destroy() {
      this.unsubscribe?.();
    },
  }));
}
