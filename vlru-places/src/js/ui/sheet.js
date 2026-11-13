/**
 * Нижняя шторка для мобильной карты: три положения peek → half → full.
 * Тянуть можно за ручку и заголовок панели; короткий тап по ручке переключает положение по кругу.
 * Ручка — кнопка, поэтому шторка доступна и с клавиатуры, и для скринридера (aria-expanded).
 */

const STATES = ['peek', 'half', 'full'];
const LABELS = { peek: 'Развернуть панель', half: 'Развернуть панель на весь экран', full: 'Свернуть панель' };

export function bindSheet(panel, { handle, dragZones = [], onChange } = {}) {
  let current = panel.dataset.sheet || 'peek';
  let drag = null;
  /** После перетаскивания браузер всё равно шлёт click по ручке — его нужно пропустить. */
  let suppressClick = false;

  const isActive = () => window.matchMedia('(max-width: 47.99rem)').matches;

  const set = (state, { silent = false } = {}) => {
    current = state;
    panel.dataset.sheet = state;
    panel.style.removeProperty('--sheet-offset');
    handle.setAttribute('aria-expanded', String(state !== 'peek'));
    handle.setAttribute('aria-label', LABELS[state]);
    if (!silent) onChange?.(state);
  };

  /** Смещение каждого положения в пикселях от верхней границы шторки. */
  const offsets = () => {
    const height = panel.getBoundingClientRect().height;
    const peek = parseFloat(getComputedStyle(panel.parentElement).getPropertyValue('--sheet-peek')) * 16 || 140;
    return { peek: height - peek, half: height * 0.45, full: 0 };
  };

  const start = (event) => {
    if (!isActive() || event.button > 0) return;
    if (event.target.closest('input, select, textarea, a, button:not(.map-panel__handle)')) return;
    drag = { y: event.clientY, start: offsets()[current], moved: false, id: event.pointerId };
    panel.classList.add('is-dragging');
  };

  const move = (event) => {
    if (!drag || event.pointerId !== drag.id) return;
    const dy = event.clientY - drag.y;
    if (Math.abs(dy) > 4) drag.moved = true;
    if (!drag.moved) return;
    const max = offsets().peek;
    const next = Math.min(max, Math.max(0, drag.start + dy));
    panel.style.setProperty('--sheet-offset', `${next}px`);
  };

  const end = (event) => {
    if (!drag || event.pointerId !== drag.id) return;
    panel.classList.remove('is-dragging');
    const wasMoved = drag.moved;
    const dy = event.clientY - drag.y;
    drag = null;
    if (!wasMoved) return;
    suppressClick = true;
    setTimeout(() => {
      suppressClick = false;
    }, 0);
    const o = offsets();
    const position = Math.min(o.peek, Math.max(0, (o[current] ?? 0) + dy));
    // Ближайшее положение с поправкой на направление жеста
    const sorted = STATES.map((state) => ({ state, distance: Math.abs(o[state] - position) })).sort((a, b) => a.distance - b.distance);
    let target = sorted[0].state;
    if (Math.abs(dy) > 40 && target === current) {
      const index = STATES.indexOf(current) + (dy < 0 ? 1 : -1);
      target = STATES[Math.max(0, Math.min(STATES.length - 1, index))];
    }
    set(target);
  };

  const zones = [handle, ...dragZones];
  zones.forEach((zone) => zone.addEventListener('pointerdown', start));
  window.addEventListener('pointermove', move);
  window.addEventListener('pointerup', end);
  window.addEventListener('pointercancel', end);

  handle.addEventListener('click', () => {
    if (suppressClick) return;
    const index = STATES.indexOf(current);
    set(current === 'full' ? 'peek' : STATES[index + 1]);
  });

  set(current, { silent: true });

  return {
    set,
    get: () => current,
    destroy() {
      zones.forEach((zone) => zone.removeEventListener('pointerdown', start));
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', end);
      window.removeEventListener('pointercancel', end);
    },
  };
}
