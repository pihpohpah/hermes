/**
 * Контроллер карты: одна MapLibre-карта на всё приложение.
 *
 * Слои продукта рисуются HTML-маркерами (а не символами WebGL), чтобы маркеры были частью
 * дизайн-системы: CSS-классы .marker*, состояния hover/selected, подписи. Кластеризацию
 * считает сама MapLibre (GeoJSON source с cluster: true), а маркеры синхронизируются
 * с видимыми фичами после каждой отрисовки.
 *
 * Маркеры не попадают в порядок табуляции: клавиатурный и экранный доступ к тем же объектам
 * даёт список в панели, а сама карта управляется стрелками и +/−.
 */

import { Map as MapLibreMap, Marker } from '../../../vendor/maplibre-gl/maplibre-gl.mjs';
import { buildStyle } from './style.js';
import { MAX_BOUNDS, SCENARIO_VIEW, boundsOf } from '../core/geo.js';
import { escapeHtml, prefersReducedMotion } from '../core/dom.js';
import { formatDayShort, pinTiming } from '../core/format.js';
import { categoryOf, PIN_KIND_BY_ID } from '../data/taxonomy.js';

const SPRITE = 'assets/icons.svg';
/** С этого масштаба подписи маркеров видны всегда; раньше — только при наведении и выборе. */
const LABELS_ZOOM = 15.5;
const LOAD_TIMEOUT_MS = 12000;

const iconSvg = (name) => `<svg class="icon" aria-hidden="true"><use href="${SPRITE}#i-${name}"></use></svg>`;

const emptyCollection = () => ({ type: 'FeatureCollection', features: [] });

function itemsToGeoJson(items) {
  return {
    type: 'FeatureCollection',
    features: items
      .filter((item) => item.coords)
      .map((item) => ({
        type: 'Feature',
        geometry: { type: 'Point', coordinates: item.coords },
        properties: {
          id: item.id,
          type: item.type,
          scenario: item.scenario,
          icon: categoryOf(item).icon || 'map-pin',
          title: item.title,
          day: item.type === 'event' ? formatDayShort(item.startsAt) : '',
        },
      })),
  };
}

export function createMap(container, { onSelect, onHover, onError, onReady } = {}) {
  const view = SCENARIO_VIEW.city;
  const map = new MapLibreMap({
    container,
    style: buildStyle(),
    center: view.center,
    zoom: view.zoom,
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

  const state = {
    ready: false,
    mode: 'items',
    scenario: 'city',
    selectedId: null,
    hoveredId: null,
    markers: new Map(),
    pinMarkers: new Map(),
    stopMarkers: [],
    meMarker: null,
    pins: [],
    syncScheduled: false,
    failed: false,
  };

  /* ── Загрузка и ошибки ── */

  const loadTimer = setTimeout(() => {
    if (!state.ready) fail('timeout');
  }, LOAD_TIMEOUT_MS);

  function fail(reason) {
    if (state.failed) return;
    state.failed = true;
    onError?.(reason);
  }

  map.on('error', (event) => {
    // Ошибки отдельных тайлов карта переживает сама; критична только недоступность источника до загрузки
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

    map.addSource('items', {
      type: 'geojson',
      data: emptyCollection(),
      cluster: true,
      clusterRadius: 46,
      clusterMaxZoom: 13,
    });

    // Невидимый слой нужен, чтобы MapLibre нарезала источник на тайлы: без слоя querySourceFeatures пуст
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
    onReady?.();
  });

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
    el.dataset.scenario = props.type === 'event' ? 'timed' : props.scenario;
    el.className = props.type === 'event' ? 'marker marker--event' : 'marker';
    el.setAttribute('aria-label', props.title);
    el.innerHTML =
      props.type === 'event'
        ? `${iconSvg('calendar')}<span>${escapeHtml(props.day)}</span><span class="marker__label">${escapeHtml(props.title)}</span>`
        : `${iconSvg(props.icon)}<span class="marker__label">${escapeHtml(props.title)}</span>`;
    el.addEventListener('click', (event) => {
      event.stopPropagation();
      onSelect?.(props.id, props.type);
    });
    el.addEventListener('mouseenter', () => onHover?.(props.id));
    el.addEventListener('mouseleave', () => onHover?.(null));
    return new Marker({ element: el, anchor: 'center' });
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
    return new Marker({ element: el, anchor: 'center' });
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
    marker.getElement().style.zIndex = selected ? '5' : key === state.hoveredId ? '4' : '';
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
    const kind = PIN_KIND_BY_ID[pin.kind] || PIN_KIND_BY_ID.other;
    el.innerHTML = `${iconSvg(kind.icon)}<span class="marker__time"></span><span class="marker__label">${escapeHtml(pin.title)}</span>`;
    el.addEventListener('click', (event) => {
      event.stopPropagation();
      onSelect?.(pin.id, 'pin');
    });
    el.addEventListener('mouseenter', () => onHover?.(pin.id));
    el.addEventListener('mouseleave', () => onHover?.(null));
    return new Marker({ element: el, anchor: 'bottom-left', offset: [-4, 4] });
  }

  function updatePinTimes() {
    for (const pin of state.pins) {
      const marker = state.pinMarkers.get(pin.id);
      if (!marker) continue;
      const timing = pinTiming(pin.startsAt, pin.endsAt);
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
        const marker = createPinMarker(pin).setLngLat(pin.coords).addTo(map);
        state.pinMarkers.set(pin.id, marker);
      } else {
        state.pinMarkers.get(pin.id).setLngLat(pin.coords);
      }
    }
    updatePinTimes();
    refreshMarkerStates();
  }

  const pinTimer = setInterval(updatePinTimes, 30_000);

  /* ── Публичный API ── */

  const whenReady = (fn) => (state.ready ? fn() : map.once('load', fn));
  const duration = (ms) => (prefersReducedMotion() ? 0 : ms);

  return {
    raw: map,

    setMode(mode) {
      if (state.mode === mode) return;
      state.mode = mode;
      state.selectedId = null;
      if (mode !== 'items') clearItemMarkers();
      renderPins();
      whenReady(scheduleSync);
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

    /** Отступы, которые занимают панели: центр карты считается внутри свободной области. */
    setPadding(padding) {
      map.setPadding(padding);
    },

    flyTo(coords, zoom) {
      map.easeTo({ center: coords, zoom: zoom ?? Math.max(map.getZoom(), 13), duration: duration(600) });
    },

    fitTo(coordsList, { maxZoom = 14 } = {}) {
      if (!coordsList.length) return;
      if (coordsList.length === 1) {
        this.flyTo(coordsList[0], Math.min(maxZoom, 14));
        return;
      }
      map.fitBounds(boundsOf(coordsList), { padding: 64, maxZoom, duration: duration(600) });
    },

    resetView(key = 'city') {
      const target = SCENARIO_VIEW[key] || SCENARIO_VIEW.city;
      map.easeTo({ center: target.center, zoom: target.zoom, duration: duration(600) });
    },

    zoomIn: () => map.zoomIn({ duration: duration(250) }),
    zoomOut: () => map.zoomOut({ duration: duration(250) }),
    getCenter: () => map.getCenter().toArray(),
    getZoom: () => map.getZoom(),

    /** Маршрут: линия + номера остановок. approximate — линия по прямой (нет ответа сервиса). */
    setRoute({ stops = [], geometry = null, approximate = false } = {}) {
      state.stopMarkers.forEach((marker) => marker.remove());
      state.stopMarkers = stops.map((stop, index) => {
        const el = document.createElement('div');
        el.className = 'marker marker--stop';
        el.textContent = String(index + 1);
        el.setAttribute('aria-hidden', 'true');
        return new Marker({ element: el, anchor: 'center', offset: [16, -16] }).setLngLat(stop.coords).addTo(map);
      });
      whenReady(() => {
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
        state.meMarker = new Marker({ element: el, anchor: 'center' });
      }
      state.meMarker.setLngLat(coords).addTo(map);
      this.flyTo(coords, 14);
    },

    resize: () => map.resize(),

    isReady: () => state.ready,

    destroy() {
      clearInterval(pinTimer);
      clearTimeout(loadTimer);
      map.remove();
    },
  };
}
