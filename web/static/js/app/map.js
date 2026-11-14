/* Контроллер карты: одна MapLibre-карта на сцену (порт map.js макета).
 *
 * Подложка — офлайн-архивы PMTiles нашего сервера (map-tiles.js) в стиле
 * web/map/style.json с цветами из токенов дизайн-системы (map-style.js).
 * Маркеры — HTML-элементы дизайн-системы (.marker*), кластеризацию считает
 * MapLibre (GeoJSON source с cluster: true). Выбор объекта уходит наружу
 * кастомным событием "places:select" (его ловит stage.js и делает htmx-своп). */
import { ensureMapProtocol } from './map-tiles.js';
import { applyStyleMode, loadMapStyle, styleModeFor } from './map-style.js';

const SPRITE = '/icons.svg';
const LABELS_ZOOM = 15.5;
const LOAD_TIMEOUT_MS = 12000;

const CITY_CENTER = [131.8855, 43.1155];
const SCENARIO_VIEW = {
  city: { center: [131.905, 43.098], zoom: 11.6 },
  nature: { center: [131.95, 43.35], zoom: 7.4 },
  sea: { center: [131.92, 42.98], zoom: 8.6 },
  now: { center: [131.9, 43.11], zoom: 11.8 },
};
const MAX_BOUNDS = [
  [128.5, 41.2],
  [140.5, 48.8],
];

const ICONS = {
  sport: 'volleyball', walk: 'footprints', music: 'guitar', games: 'gamepad-2',
  help: 'hand-helping', other: 'message-circle',
};

const iconSvg = (name) => `<svg class="icon" aria-hidden="true"><use href="${SPRITE}#i-${name}"></use></svg>`;
const escapeHtml = (value) => String(value).replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));
const prefersReducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches;

function formatDuration(minutes) {
  const total = Math.max(1, Math.round(minutes));
  if (total < 60) return `${total} мин`;
  const hours = Math.floor(total / 60);
  const rest = total % 60;
  if (hours >= 24) {
    const days = Math.round(hours / 24);
    return `${days} дн.`;
  }
  return rest ? `${hours} ч ${rest} мин` : `${hours} ч`;
}

// pinTiming — статус метки (порт format.js).
function pinTiming(startsAt, endsAt, now = Date.now()) {
  const start = new Date(startsAt).getTime();
  const end = new Date(endsAt).getTime();
  if (now < start) {
    const minutes = Math.round((start - now) / 60000);
    if (minutes < 1) return { state: 'soon', short: 'сейчас' };
    const d = formatDuration(minutes);
    return { state: 'soon', short: d };
  }
  if (now < end) return { state: 'live', short: 'идёт' };
  return { state: 'ended', short: 'всё' };
}

const emptyCollection = () => ({ type: 'FeatureCollection', features: [] });

function itemsToGeoJson(items) {
  return {
    type: 'FeatureCollection',
    features: items
      .filter((item) => item.has_coords)
      .map((item) => ({
        type: 'Feature',
        geometry: { type: 'Point', coordinates: [item.lon, item.lat] },
        properties: {
          id: item.id,
          type: item.type,
          scenario: item.type === 'event' ? 'timed' : item.scenario,
          icon: item.icon || 'map-pin',
          title: item.title,
          day: item.type === 'event' ? dayShort(item.starts_at) : '',
        },
      })),
  };
}

const MONTHS_SHORT = ['янв', 'фев', 'мар', 'апр', 'мая', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек'];
function dayShort(iso) {
  const date = new Date(iso);
  return `${date.getDate()} ${MONTHS_SHORT[date.getMonth()]}`;
}

function boundsOf(coordsList) {
  const lngs = coordsList.map((c) => c[0]);
  const lats = coordsList.map((c) => c[1]);
  return [
    [Math.min(...lngs), Math.min(...lats)],
    [Math.max(...lngs), Math.max(...lats)],
  ];
}

export function createPlacesMap(container, { onError, scenario = 'city' } = {}) {
  const initialView = SCENARIO_VIEW[scenario] || SCENARIO_VIEW.city;
  let map = null;
  let styleMode = 'city';

  const state = {
    ready: false,
    mode: 'items',
    scenario,
    selectedId: null,
    hoveredId: null,
    markers: new Map(),
    pinMarkers: new Map(),
    stopMarkers: [],
    meMarker: null,
    pins: [],
    syncScheduled: false,
    failed: false,
    destroyed: false,
    padding: null,
  };

  // Вызовы до готовности карты (данные, камера, маршрут) ждут в очереди.
  const pending = [];
  const whenReady = (fn) => (state.ready ? fn() : pending.push(fn));

  const emit = (id, type) => container.dispatchEvent(new CustomEvent('places:select', { detail: { id, type }, bubbles: true }));
  const emitHover = (id) => container.dispatchEvent(new CustomEvent('places:hover', { detail: { id }, bubbles: true }));

  const loadTimer = setTimeout(() => {
    if (!state.ready) fail('timeout');
  }, LOAD_TIMEOUT_MS);

  function fail(reason) {
    if (state.failed) return;
    state.failed = true;
    onError?.(reason);
  }

  function createMap(style) {
    map = new maplibregl.Map({
      container,
      style,
      center: initialView.center,
      zoom: initialView.zoom,
      minZoom: 5,
      maxZoom: 18,
      maxBounds: MAX_BOUNDS,
      attributionControl: { compact: true },
      dragRotate: false,
      pitchWithRotate: false,
      touchPitch: false,
      keyboard: true,
      fadeDuration: 150,
      locale: {
        'AttributionControl.ToggleAttribution': 'Показать или скрыть атрибуцию',
        'Map.Title': 'Карта. Перемещение — стрелки, масштаб — плюс и минус',
      },
    });
    map.touchZoomRotate.disableRotation();
    if (state.padding) map.setPadding(state.padding);

    map.on('error', (event) => {
      if (!state.ready) {
        console.warn('Карта: ошибка загрузки', event.error);
        fail('network');
      }
    });

    map.on('load', () => {
      clearTimeout(loadTimer);
      map.resize();
      state.ready = true;
      state.failed = false;

      map.addSource('items', { type: 'geojson', data: emptyCollection(), cluster: true, clusterRadius: 46, clusterMaxZoom: 13 });
      map.addLayer({
        id: 'items-anchor',
        type: 'circle',
        source: 'items',
        paint: { 'circle-radius': 0, 'circle-opacity': 0 },
      });

      map.addSource('route', { type: 'geojson', data: emptyCollection() });
      map.addLayer({
        id: 'route-casing',
        type: 'line',
        source: 'route',
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: { 'line-color': '#ffffff', 'line-width': ['interpolate', ['linear'], ['zoom'], 8, 5, 16, 10] },
      });
      map.addLayer({
        id: 'route-line',
        type: 'line',
        source: 'route',
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: {
          'line-color': getComputedStyle(document.documentElement).getPropertyValue('--map-route').trim() || '#ee5a08',
          'line-width': ['interpolate', ['linear'], ['zoom'], 8, 2.5, 16, 5],
          'line-dasharray': ['case', ['get', 'approximate'], ['literal', [1.5, 1.5]], ['literal', [1, 0]]],
        },
      });

      map.on('render', scheduleSync);
      map.on('zoom', updateLabels);
      updateLabels();
      pending.splice(0).forEach((fn) => fn());
      container.dispatchEvent(new CustomEvent('places:ready', { bubbles: true }));
    });
  }

  try {
    ensureMapProtocol();
  } catch (error) {
    console.error('Карта: нет MapLibre или PMTiles', error);
    clearTimeout(loadTimer);
    fail('crash');
  }
  if (!state.failed) {
    loadMapStyle(scenario).then(
      ({ style, mode }) => {
        if (state.destroyed) return;
        styleMode = mode;
        createMap(style);
      },
      (error) => {
        console.warn('Карта: стиль не загрузился', error);
        fail('network');
      },
    );
  }

  function updateLabels() {
    container.classList.toggle('show-labels', map.getZoom() >= LABELS_ZOOM);
  }

  /* ── Маркеры мест и событий ── */

  function scheduleSync() {
    if (state.syncScheduled) return;
    state.syncScheduled = true;
    requestAnimationFrame(() => {
      state.syncScheduled = false;
      syncItemMarkers();
    });
  }

  function createItemMarker(props) {
    const el = document.createElement('button');
    el.type = 'button';
    el.tabIndex = -1;
    el.dataset.id = props.id;
    el.dataset.scenario = props.scenario;
    el.className = props.type === 'event' ? 'marker marker--event' : 'marker';
    el.setAttribute('aria-label', props.title);
    el.innerHTML =
      props.type === 'event'
        ? `${iconSvg('calendar')}<span>${escapeHtml(props.day)}</span><span class="marker__label">${escapeHtml(props.title)}</span>`
        : `${iconSvg(props.icon)}<span class="marker__label">${escapeHtml(props.title)}</span>`;
    el.addEventListener('click', (event) => {
      event.stopPropagation();
      emit(props.id, props.type);
    });
    el.addEventListener('mouseenter', () => emitHover(props.id));
    el.addEventListener('mouseleave', () => emitHover(null));
    return new maplibregl.Marker({ element: el, anchor: 'center' });
  }

  function createClusterMarker(props, coords) {
    const el = document.createElement('button');
    el.type = 'button';
    el.tabIndex = -1;
    const count = props.point_count;
    el.className = 'marker marker--cluster';
    el.dataset.scenario = state.scenario;
    el.dataset.size = count >= 20 ? 'lg' : count >= 8 ? 'md' : 'sm';
    el.setAttribute('aria-label', `${count} объектов рядом, приблизить`);
    el.textContent = String(count);
    el.addEventListener('click', async (event) => {
      event.stopPropagation();
      try {
        const zoom = await map.getSource('items').getClusterExpansionZoom(props.cluster_id);
        map.easeTo({ center: coords, zoom: Math.min(zoom + 0.3, 17), duration: prefersReducedMotion() ? 0 : 500 });
      } catch (error) {
        console.warn('Не удалось раскрыть кластер', error);
        map.easeTo({ center: coords, zoom: map.getZoom() + 2 });
      }
    });
    return new maplibregl.Marker({ element: el, anchor: 'center' });
  }

  function syncItemMarkers() {
    if (!state.ready || state.mode !== 'items') return;
    const features = map.querySourceFeatures('items');
    const seen = new Set();

    for (const feature of features) {
      const props = feature.properties;
      const key = props.cluster ? `cluster-${props.cluster_id}` : props.id;
      if (seen.has(key)) continue;
      seen.add(key);
      if (!state.markers.has(key)) {
        const coords = feature.geometry.coordinates;
        const marker = props.cluster ? createClusterMarker(props, coords) : createItemMarker(props);
        marker.setLngLat(coords).addTo(map);
        state.markers.set(key, marker);
        applyMarkerState(key, marker);
      }
    }

    for (const [key, marker] of state.markers) {
      if (!seen.has(key)) {
        marker.remove();
        state.markers.delete(key);
      }
    }
  }

  function applyMarkerState(key, marker) {
    const el = marker.getElement();
    if (key.startsWith('cluster-')) return;
    const selected = key === state.selectedId;
    el.setAttribute('aria-pressed', String(selected));
    el.classList.toggle('is-hovered', key === state.hoveredId);
    el.style.zIndex = selected ? '5' : key === state.hoveredId ? '4' : '';
  }

  function refreshMarkerStates() {
    for (const [key, marker] of state.markers) applyMarkerState(key, marker);
    for (const [key, marker] of state.pinMarkers) {
      const el = marker.getElement();
      const selected = key === state.selectedId;
      el.setAttribute('aria-pressed', String(selected));
      el.classList.toggle('is-hovered', key === state.hoveredId);
      el.style.zIndex = selected ? '5' : key === state.hoveredId ? '4' : '';
    }
  }

  function clearItemMarkers() {
    for (const marker of state.markers.values()) marker.remove();
    state.markers.clear();
  }

  /* ── Быстрые метки ── */

  function createPinMarker(pin) {
    const el = document.createElement('button');
    el.type = 'button';
    el.tabIndex = -1;
    el.dataset.id = pin.id;
    el.className = 'marker marker--pin';
    el.setAttribute('aria-label', pin.title);
    el.innerHTML = `${iconSvg(ICONS[pin.kind] || 'message-circle')}<span class="marker__time"></span><span class="marker__label">${escapeHtml(pin.title)}</span>`;
    el.addEventListener('click', (event) => {
      event.stopPropagation();
      emit(pin.id, 'pin');
    });
    el.addEventListener('mouseenter', () => emitHover(pin.id));
    el.addEventListener('mouseleave', () => emitHover(null));
    return new maplibregl.Marker({ element: el, anchor: 'bottom-left', offset: [-4, 4] });
  }

  function updatePinTimes() {
    for (const pin of state.pins) {
      const marker = state.pinMarkers.get(pin.id);
      if (!marker) continue;
      const timing = pinTiming(pin.starts_at, pin.ends_at);
      const el = marker.getElement();
      el.classList.toggle('is-live', timing.state === 'live');
      el.classList.toggle('is-mine', Boolean(pin.mine));
      el.querySelector('.marker__time').textContent = timing.short;
    }
  }

  function renderPins() {
    const ids = new Set(state.pins.map((pin) => pin.id));
    for (const [id, marker] of state.pinMarkers) {
      if (!ids.has(id) || state.mode !== 'pins') {
        marker.remove();
        state.pinMarkers.delete(id);
      }
    }
    if (state.mode !== 'pins') return;
    for (const pin of state.pins) {
      if (!state.pinMarkers.has(pin.id)) {
        const marker = createPinMarker(pin).setLngLat([pin.lon, pin.lat]).addTo(map);
        state.pinMarkers.set(pin.id, marker);
      } else {
        state.pinMarkers.get(pin.id).setLngLat([pin.lon, pin.lat]);
      }
    }
    updatePinTimes();
    refreshMarkerStates();
  }

  const pinTimer = setInterval(updatePinTimes, 30000);

  /* ── Публичный API ── */

  const duration = (ms) => (prefersReducedMotion() ? 0 : ms);

  const api = {
    setMode(mode) {
      if (state.mode === mode) return;
      state.mode = mode;
      state.selectedId = null;
      if (mode !== 'items') clearItemMarkers();
      whenReady(() => {
        renderPins();
        scheduleSync();
      });
    },

    setItems(items, { scenario } = {}) {
      if (scenario) state.scenario = scenario;
      whenReady(() => {
        clearItemMarkers();
        map.getSource('items').setData(itemsToGeoJson(items));
        scheduleSync();
      });
    },

    setPins(pins) {
      state.pins = pins;
      whenReady(renderPins);
    },

    select(id) {
      state.selectedId = id;
      refreshMarkerStates();
    },

    hover(id) {
      state.hoveredId = id;
      refreshMarkerStates();
    },

    setPadding(padding) {
      state.padding = padding;
      map?.setPadding(padding);
    },

    flyTo(coords, zoom) {
      whenReady(() => map.easeTo({ center: coords, zoom: zoom ?? Math.max(map.getZoom(), 13), duration: duration(600) }));
    },

    fitTo(coordsList, { maxZoom = 14 } = {}) {
      if (!coordsList.length) return;
      if (coordsList.length === 1) {
        this.flyTo(coordsList[0], Math.min(maxZoom, 14));
        return;
      }
      whenReady(() => map.fitBounds(boundsOf(coordsList), { padding: 64, maxZoom, duration: duration(600) }));
    },

    resetView(key = 'city') {
      const target = SCENARIO_VIEW[key] || SCENARIO_VIEW.city;
      whenReady(() => map.easeTo({ center: target.center, zoom: target.zoom, duration: duration(600) }));
    },

    /** Сценарий меняет режим слоёв (рельеф у «Природы») и стартовый вид. */
    setScenario(key) {
      state.scenario = key;
      whenReady(() => {
        styleMode = styleModeFor(key, map.getStyle());
        applyStyleMode(map, styleMode);
        this.resetView(key);
      });
    },

    getStyleMode: () => styleMode,
    zoomIn: () => whenReady(() => map.zoomIn({ duration: duration(250) })),
    zoomOut: () => whenReady(() => map.zoomOut({ duration: duration(250) })),
    getCenter: () => (map ? map.getCenter().toArray() : initialView.center),
    getZoom: () => (map ? map.getZoom() : initialView.zoom),

    setRoute({ stops = [], geometry = null, approximate = false } = {}) {
      whenReady(() => {
        state.stopMarkers.forEach((marker) => marker.remove());
        state.stopMarkers = stops.map((stop, index) => {
          const el = document.createElement('div');
          el.className = 'marker marker--stop';
          el.textContent = String(index + 1);
          el.setAttribute('aria-hidden', 'true');
          return new maplibregl.Marker({ element: el, anchor: 'center', offset: [16, -16] }).setLngLat(stop.coords).addTo(map);
        });
        const data = geometry
          ? { type: 'FeatureCollection', features: [{ type: 'Feature', geometry, properties: { approximate } }] }
          : emptyCollection();
        map.getSource('route').setData(data);
      });
    },

    showMe(coords) {
      if (!state.meMarker) {
        const el = document.createElement('div');
        el.className = 'marker marker--me';
        el.setAttribute('aria-hidden', 'true');
        state.meMarker = new maplibregl.Marker({ element: el, anchor: 'center' });
      }
      whenReady(() => state.meMarker.setLngLat(coords).addTo(map));
      this.flyTo(coords, 14);
    },

    resize: () => map?.resize(),
    isReady: () => state.ready,

    destroy() {
      state.destroyed = true;
      pending.length = 0;
      clearInterval(pinTimer);
      clearTimeout(loadTimer);
      map?.remove();
    },
  };

  return api;
}

/** Карта на общей подложке без сцены: превью места, выбор точки в мастере. */
export async function createBaseMap(container, { scenario = 'city', ...options } = {}) {
  ensureMapProtocol();
  const { style } = await loadMapStyle(scenario);
  return new maplibregl.Map({ container, style, maxBounds: MAX_BOUNDS, attributionControl: { compact: true }, ...options });
}

/** Неинтерактивная карта-превью с маркером (страница места). */
export async function createMiniMap(container, { center, zoom, scenario = 'city' }) {
  const map = await createBaseMap(container, { scenario, center, zoom, interactive: false });
  const el = document.createElement('div');
  el.className = 'marker marker--default';
  el.setAttribute('aria-hidden', 'true');
  new maplibregl.Marker({ element: el, anchor: 'bottom' }).setLngLat(center).addTo(map);
  return map;
}
