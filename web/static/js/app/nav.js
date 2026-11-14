/* Навигация фрагментами: JS меняет URL, htmx подменяет фрагмент,
 * плюс синхронизация данных карты (места, метки). */
import { api, fetchItem } from './api.js';
import { $$, currentParams } from './util.js';

export function panelSwap(fragmentURL, pageURL, target = '#map-panel-inner') {
  htmx.ajax('GET', fragmentURL, { target, swap: 'innerHTML' });
  history.pushState({}, '', pageURL);
}

/** Выбор места/события на карте: превью в панели. */
export function selectItem(id) {
  const params = currentParams();
  params.set('place', id);
  const qs = params.toString();
  panelSwap(`/fragments/map-panel?${qs}`, `/?${qs}`);
  window.placesMap?.select(id);
  fetchItem(id).then((item) => {
    if (item?.coords) window.placesMap?.flyTo(item.coords);
  });
}

/** Выбор метки «Сейчас»: серверное превью. */
export function selectPin(id) {
  const params = currentParams();
  params.set('pin', id);
  const qs = params.toString();
  panelSwap(`/fragments/pin/${encodeURIComponent(id)}`, `/now?${qs}`);
  window.placesMap?.select(id);
}

export async function refreshPins() {
  try {
    const pins = await api('/pins');
    window.placesMap?.setPins(pins);
    updateKindCounts(pins);
    return pins;
  } catch (error) {
    console.warn('Не удалось обновить метки', error);
    return [];
  }
}

export function updateKindCounts(pins) {
  const counts = {};
  pins.forEach((pin) => {
    counts[pin.kind] = (counts[pin.kind] || 0) + 1;
  });
  $$('[data-kind-count]').forEach((el) => {
    const n = counts[el.dataset.kindCount] || 0;
    el.textContent = n ? String(n) : '';
  });
}

export const mapItems = async (query) => {
  const data = await api(`/api/map-items?${query}`);
  if (Array.isArray(data)) return data;
  return data?.items ?? [];
};

export async function refreshMapItems({ fit = false } = {}) {
  try {
    const items = await mapItems(currentParams().toString());
    window.placesMap?.setItems(items);
    if (fit) {
      const coords = items.filter((i) => i.has_coords).map((i) => [i.lon, i.lat]);
      if (coords.length) window.placesMap?.fitTo(coords);
    }
    return items;
  } catch (error) {
    console.warn('Не удалось обновить места', error);
    return [];
  }
}
