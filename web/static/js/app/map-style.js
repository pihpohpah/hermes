/* Стиль карты: web/map/style.json (сервер вырезает источники несобранных архивов)
 * плюс цвета из CSS-токенов дизайн-системы.
 *
 * В style.json цвета записаны как "var(--map-*)"; здесь они заменяются на
 * значения токенов из app.css, поэтому карта и интерфейс меняются из одного
 * места. Режим слоя («город», «природа» или оба) лежит в metadata["hermes:mode"]. */

const STYLE_URL = '/map/style.json';
const TOKEN = /^var\((--[a-z0-9-]+)\)$/;

function resolveTokens(value, css) {
  if (typeof value === 'string') {
    const match = TOKEN.exec(value);
    if (!match) return value;
    const resolved = css.getPropertyValue(match[1]).trim();
    if (!resolved) {
      console.warn('Карта: токен не определён в app.css', match[1]);
      return '#000000';
    }
    return resolved;
  }
  if (Array.isArray(value)) return value.map((item) => resolveTokens(item, css));
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, resolveTokens(item, css)]));
  }
  return value;
}

const layerMode = (layer) => layer.metadata?.['hermes:mode'] || 'city';
const layerVisible = (layer, mode) => {
  const own = layerMode(layer);
  return own === 'both' || own === mode;
};

/** Режим слоёв для сценария: «Природа» — рельеф, если его архив собран. */
export function styleModeFor(scenario, style) {
  return scenario === 'nature' && style.sources?.relief ? 'nature' : 'city';
}

/** Загружает стиль, подставляет токены и включает слои режима сценария. */
export async function loadMapStyle(scenario) {
  const response = await fetch(STYLE_URL, { cache: 'no-cache' });
  if (!response.ok) throw new Error(`стиль карты: ответ ${response.status}`);
  const style = await response.json();
  const css = getComputedStyle(document.documentElement);
  const mode = styleModeFor(scenario, style);

  style.layers = style.layers.map((layer) => ({
    ...layer,
    ...(layer.paint && { paint: resolveTokens(layer.paint, css) }),
    layout: { ...layer.layout, visibility: layerVisible(layer, mode) ? 'visible' : 'none' },
  }));
  return { style, mode };
}

/** Переключает режим на живой карте. */
export function applyStyleMode(map, mode) {
  for (const layer of map.getStyle().layers) {
    if (!layer.metadata?.['hermes:mode']) continue;
    map.setLayoutProperty(layer.id, 'visibility', layerVisible(layer, mode) ? 'visible' : 'none');
  }
}
