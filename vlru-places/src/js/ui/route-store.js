/**
 * Маршрут пользователя — список id мест в store.route (сохраняется между сессиями).
 */

import { store } from '../core/store.js';
import { toast } from './toast.js';
import { MAX_STOPS } from '../map/routing.js';
import { navigate } from '../core/router.js';
import { countLabel } from '../core/format.js';

export const isInRoute = (id) => store.get().route.includes(id);

export function toggleRoute(id, title = '') {
  const { route } = store.get();
  if (route.includes(id)) {
    store.set({ route: route.filter((stopId) => stopId !== id) });
    toast(`Убрали из маршрута${title ? `: ${title}` : ''}`, { group: 'route' });
    return false;
  }
  if (route.length >= MAX_STOPS) {
    toast(`В маршруте может быть до ${MAX_STOPS} точек`, { type: 'danger', group: 'route' });
    return false;
  }
  store.set({ route: [...route, id] });
  const count = route.length + 1;
  toast(count === 1 ? 'Первая точка маршрута. Добавьте ещё одну' : `В маршруте ${countLabel(count, ['точка', 'точки', 'точек'])}`, {
    type: 'success',
    group: 'route',
    action: count >= 2 ? { label: 'Показать', onClick: () => navigate('/map', { panel: 'route' }) } : null,
  });
  return true;
}

export function moveStop(id, delta) {
  const route = [...store.get().route];
  const index = route.indexOf(id);
  const target = index + delta;
  if (index < 0 || target < 0 || target >= route.length) return;
  [route[index], route[target]] = [route[target], route[index]];
  store.set({ route });
}

export const clearRoute = () => store.set({ route: [] });

/** Заменяет маршрут целиком и открывает его на карте. Прежний маршрут можно вернуть из уведомления. */
export function replaceRoute(ids) {
  const previous = store.get().route;
  const next = ids.slice(0, MAX_STOPS);
  store.set({ route: next });
  navigate('/map', { panel: 'route' });
  const replaced = previous.length > 0 && previous.join() !== next.join();
  toast(replaced ? 'Маршрут заменён' : 'Маршрут построен', {
    type: 'success',
    group: 'route',
    action: replaced ? { label: 'Вернуть прежний', onClick: () => store.set({ route: previous }) } : null,
  });
}
