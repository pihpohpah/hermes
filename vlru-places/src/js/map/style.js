/**
 * Схематичный стиль карты «Места».
 *
 * Данные — векторные тайлы OpenStreetMap в схеме OpenMapTiles от OpenFreeMap (без ключа, бесплатно).
 * Стиль собран с нуля: только суша, вода, зелень, дороги, паромы и подписи на русском.
 * POI, номера домов, иконки и шильды дорог выключены — смысловые точки на карте ставит только продукт.
 * Цвета читаются из CSS-токенов (--map-*), поэтому карта и интерфейс меняются из одного места.
 *
 * Провайдер тайлов и шрифтов меняется в TILES_URL / GLYPHS_URL — см. docs/map.md.
 */

export const TILES_URL = 'https://tiles.openfreemap.org/planet';
export const GLYPHS_URL = 'https://tiles.openfreemap.org/fonts/{fontstack}/{range}.pbf';
export const ATTRIBUTION =
  '<a href="https://openfreemap.org" target="_blank" rel="noopener">OpenFreeMap</a> · © <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">участники OpenStreetMap</a>';

function readTokens() {
  const css = getComputedStyle(document.documentElement);
  const v = (name) => css.getPropertyValue(name).trim();
  return {
    land: v('--map-land'),
    water: v('--map-water'),
    park: v('--map-park'),
    wood: v('--map-wood'),
    residential: v('--map-residential'),
    building: v('--map-building'),
    road: v('--map-road'),
    roadCasing: v('--map-road-casing'),
    motorway: v('--map-motorway'),
    label: v('--map-label'),
    waterLabel: v('--map-water-label'),
    text: v('--gray-900'),
    halo: '#ffffff',
    rail: v('--gray-400'),
    ferry: '#8fb6d4',
    border: v('--gray-500'),
  };
}

const NAME = ['coalesce', ['get', 'name:ru'], ['get', 'name']];
const FONT = ['Noto Sans Regular'];
const FONT_BOLD = ['Noto Sans Bold'];
const FONT_ITALIC = ['Noto Sans Italic'];
const isPolygon = ['match', ['geometry-type'], ['MultiPolygon', 'Polygon'], true, false];
const isLine = ['match', ['geometry-type'], ['LineString', 'MultiLineString'], true, false];

const roadClass = (classes) => ['match', ['get', 'class'], classes, true, false];

/** Ширина линии в зависимости от масштаба: пары [zoom, px]. */
const width = (...stops) => ['interpolate', ['exponential', 1.5], ['zoom'], ...stops.flat()];

export function buildStyle() {
  const t = readTokens();

  return {
    version: 8,
    name: 'VL.ru Места — схематичный',
    glyphs: GLYPHS_URL,
    sources: {
      osm: { type: 'vector', url: TILES_URL, attribution: ATTRIBUTION },
    },
    layers: [
      { id: 'land', type: 'background', paint: { 'background-color': t.land } },

      {
        id: 'landcover-wood',
        type: 'fill',
        source: 'osm',
        'source-layer': 'landcover',
        filter: ['all', isPolygon, ['match', ['get', 'class'], ['wood', 'forest'], true, false]],
        paint: { 'fill-color': t.wood, 'fill-opacity': ['interpolate', ['linear'], ['zoom'], 5, 0.5, 10, 0.8, 14, 0.6] },
      },
      {
        id: 'landcover-grass',
        type: 'fill',
        source: 'osm',
        'source-layer': 'landcover',
        minzoom: 9,
        filter: ['all', isPolygon, ['match', ['get', 'class'], ['grass', 'meadow', 'scrub'], true, false]],
        paint: { 'fill-color': t.park, 'fill-opacity': 0.6 },
      },
      {
        id: 'landuse-residential',
        type: 'fill',
        source: 'osm',
        'source-layer': 'landuse',
        minzoom: 10,
        filter: ['all', isPolygon, ['match', ['get', 'class'], ['residential', 'suburb', 'neighbourhood'], true, false]],
        paint: { 'fill-color': t.residential, 'fill-opacity': ['interpolate', ['linear'], ['zoom'], 10, 0.4, 14, 0.8] },
      },
      {
        id: 'park',
        type: 'fill',
        source: 'osm',
        'source-layer': 'park',
        filter: isPolygon,
        paint: { 'fill-color': t.park, 'fill-opacity': ['interpolate', ['linear'], ['zoom'], 6, 0.5, 11, 1] },
      },
      {
        id: 'water',
        type: 'fill',
        source: 'osm',
        'source-layer': 'water',
        filter: ['all', isPolygon, ['!=', ['get', 'brunnel'], 'tunnel']],
        paint: { 'fill-color': t.water },
      },
      {
        id: 'waterway',
        type: 'line',
        source: 'osm',
        'source-layer': 'waterway',
        minzoom: 10,
        filter: isLine,
        paint: { 'line-color': t.water, 'line-width': width([10, 0.6], [16, 3]) },
      },
      {
        id: 'building',
        type: 'fill',
        source: 'osm',
        'source-layer': 'building',
        minzoom: 14,
        paint: {
          'fill-color': t.building,
          'fill-outline-color': t.roadCasing,
          'fill-opacity': ['interpolate', ['linear'], ['zoom'], 14, 0, 15, 1],
        },
      },

      /* Паромы — важны для островов */
      {
        id: 'ferry',
        type: 'line',
        source: 'osm',
        'source-layer': 'transportation',
        minzoom: 8,
        filter: ['all', isLine, ['==', ['get', 'class'], 'ferry']],
        paint: { 'line-color': t.ferry, 'line-width': 1.2, 'line-dasharray': [3, 3] },
      },

      /* Дороги: подложка (casing), затем заливка */
      {
        id: 'road-minor-casing',
        type: 'line',
        source: 'osm',
        'source-layer': 'transportation',
        minzoom: 13,
        filter: ['all', isLine, roadClass(['minor', 'service'])],
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: { 'line-color': t.roadCasing, 'line-width': width([13, 1.5], [18, 16]) },
      },
      {
        id: 'road-major-casing',
        type: 'line',
        source: 'osm',
        'source-layer': 'transportation',
        minzoom: 9,
        filter: ['all', isLine, roadClass(['primary', 'secondary', 'tertiary', 'trunk', 'motorway'])],
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: { 'line-color': t.roadCasing, 'line-width': width([9, 1], [12, 3], [18, 28]) },
      },
      {
        id: 'road-path',
        type: 'line',
        source: 'osm',
        'source-layer': 'transportation',
        minzoom: 14,
        filter: ['all', isLine, roadClass(['path', 'track'])],
        paint: { 'line-color': t.rail, 'line-width': width([14, 0.6], [18, 1.6]), 'line-dasharray': [2, 2] },
      },
      {
        id: 'road-minor',
        type: 'line',
        source: 'osm',
        'source-layer': 'transportation',
        minzoom: 13,
        filter: ['all', isLine, roadClass(['minor', 'service'])],
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: { 'line-color': t.road, 'line-width': width([13, 0.8], [18, 13]) },
      },
      {
        id: 'road-major',
        type: 'line',
        source: 'osm',
        'source-layer': 'transportation',
        minzoom: 6,
        filter: ['all', isLine, roadClass(['primary', 'secondary', 'tertiary'])],
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: { 'line-color': t.road, 'line-width': width([6, 0.4], [12, 2], [18, 24]) },
      },
      {
        id: 'road-motorway',
        type: 'line',
        source: 'osm',
        'source-layer': 'transportation',
        minzoom: 5,
        filter: ['all', isLine, roadClass(['motorway', 'trunk'])],
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: { 'line-color': t.motorway, 'line-width': width([5, 0.6], [12, 2.6], [18, 26]) },
      },
      {
        id: 'rail',
        type: 'line',
        source: 'osm',
        'source-layer': 'transportation',
        minzoom: 10,
        filter: ['all', isLine, ['==', ['get', 'class'], 'rail']],
        paint: { 'line-color': t.rail, 'line-width': width([10, 0.6], [16, 1.6]), 'line-dasharray': [4, 3] },
      },
      {
        id: 'border',
        type: 'line',
        source: 'osm',
        'source-layer': 'boundary',
        filter: ['all', ['==', ['get', 'admin_level'], 2], ['!=', ['get', 'maritime'], 1]],
        paint: { 'line-color': t.border, 'line-width': 1, 'line-dasharray': [4, 2, 1, 2] },
      },

      /* Подписи */
      {
        id: 'label-road',
        type: 'symbol',
        source: 'osm',
        'source-layer': 'transportation_name',
        minzoom: 14,
        filter: roadClass(['primary', 'secondary', 'tertiary', 'minor', 'trunk']),
        layout: {
          'symbol-placement': 'line',
          'text-field': NAME,
          'text-font': FONT,
          'text-size': ['interpolate', ['linear'], ['zoom'], 14, 11, 17, 13],
          'text-max-angle': 30,
        },
        paint: { 'text-color': t.label, 'text-halo-color': t.halo, 'text-halo-width': 1.4 },
      },
      {
        id: 'label-water',
        type: 'symbol',
        source: 'osm',
        'source-layer': 'water_name',
        minzoom: 7,
        filter: ['match', ['geometry-type'], ['Point', 'MultiPoint'], true, false],
        layout: {
          'text-field': NAME,
          'text-font': FONT_ITALIC,
          'text-size': ['interpolate', ['linear'], ['zoom'], 7, 11, 12, 14],
          'text-letter-spacing': 0.04,
          'text-max-width': 7,
        },
        paint: { 'text-color': t.waterLabel, 'text-halo-color': t.water, 'text-halo-width': 1 },
      },
      {
        id: 'label-water-line',
        type: 'symbol',
        source: 'osm',
        'source-layer': 'water_name',
        minzoom: 9,
        filter: isLine,
        layout: {
          'symbol-placement': 'line',
          'text-field': NAME,
          'text-font': FONT_ITALIC,
          'text-size': 12,
          'text-letter-spacing': 0.04,
        },
        paint: { 'text-color': t.waterLabel, 'text-halo-color': t.water, 'text-halo-width': 1 },
      },
      {
        id: 'label-neighbourhood',
        type: 'symbol',
        source: 'osm',
        'source-layer': 'place',
        minzoom: 12,
        filter: ['match', ['get', 'class'], ['suburb', 'quarter', 'neighbourhood'], true, false],
        layout: {
          'text-field': NAME,
          'text-font': FONT,
          'text-size': ['interpolate', ['linear'], ['zoom'], 12, 11, 15, 13],
          'text-transform': 'uppercase',
          'text-letter-spacing': 0.08,
          'text-max-width': 8,
        },
        paint: { 'text-color': t.rail, 'text-halo-color': t.halo, 'text-halo-width': 1.2 },
      },
      {
        id: 'label-island',
        type: 'symbol',
        source: 'osm',
        'source-layer': 'place',
        minzoom: 8,
        filter: ['match', ['get', 'class'], ['island'], true, false],
        layout: {
          'text-field': NAME,
          'text-font': FONT,
          'text-size': ['interpolate', ['linear'], ['zoom'], 8, 11, 13, 14],
          'text-max-width': 8,
        },
        paint: { 'text-color': t.label, 'text-halo-color': t.halo, 'text-halo-width': 1.4 },
      },
      {
        id: 'label-village',
        type: 'symbol',
        source: 'osm',
        'source-layer': 'place',
        minzoom: 8,
        filter: ['match', ['get', 'class'], ['village', 'hamlet'], true, false],
        layout: {
          'text-field': NAME,
          'text-font': FONT,
          'text-size': ['interpolate', ['linear'], ['zoom'], 8, 10, 12, 13],
          'text-max-width': 8,
        },
        paint: { 'text-color': t.label, 'text-halo-color': t.halo, 'text-halo-width': 1.4 },
      },
      {
        id: 'label-town',
        type: 'symbol',
        source: 'osm',
        'source-layer': 'place',
        minzoom: 6,
        filter: ['==', ['get', 'class'], 'town'],
        layout: {
          'text-field': NAME,
          'text-font': FONT,
          'text-size': ['interpolate', ['linear'], ['zoom'], 6, 11, 12, 15],
          'text-max-width': 8,
        },
        paint: { 'text-color': t.text, 'text-halo-color': t.halo, 'text-halo-width': 1.6 },
      },
      {
        id: 'label-city',
        type: 'symbol',
        source: 'osm',
        'source-layer': 'place',
        maxzoom: 12,
        filter: ['==', ['get', 'class'], 'city'],
        layout: {
          'text-field': NAME,
          'text-font': FONT_BOLD,
          'text-size': ['interpolate', ['linear'], ['zoom'], 5, 12, 10, 17],
          'text-max-width': 8,
        },
        paint: { 'text-color': t.text, 'text-halo-color': t.halo, 'text-halo-width': 1.8 },
      },
    ],
  };
}
