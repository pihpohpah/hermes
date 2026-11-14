/* Геометрия сцены карты: центр города, слоты верхней панели, отступы карты. */
import { $$, isMobileLayout } from './util.js';

export const CITY_CENTER = [131.8855, 43.1155];

export function layoutSlots() {
  const stage = document.getElementById('map-stage');
  if (!stage) return;
  const top = stage.querySelector('[data-top]');
  if (!top) return;
  const mobile = isMobileLayout();
  $$('[data-top-slot]', stage).forEach((node) => {
    const home = stage.querySelector(`[data-slot-home="${node.dataset.topSlot}"]`);
    if (mobile && node.parentElement !== top) top.append(node);
    if (!mobile && home && node.parentElement !== home) home.append(node);
  });
  if (mobile) {
    const head = stage.querySelector('[data-head]');
    const order = head ? $$('[data-slot-home]', head).map((home) => home.dataset.slotHome) : [];
    $$('[data-top-slot]', top)
      .sort((a, b) => order.indexOf(a.dataset.topSlot) - order.indexOf(b.dataset.topSlot))
      .forEach((node) => top.append(node));
  }
  stage.style.setProperty('--sheet-top', `${Math.ceil(top.getBoundingClientRect().height) + 20}px`);
  requestAnimationFrame(updatePadding);
}

/** Отступы карты под панель: центр (и точка выбора) — в середине свободной области. */
export function updatePadding() {
  const stage = document.getElementById('map-stage');
  const map = window.placesMap;
  if (!stage || !map?.isReady?.()) return;
  const top = stage.querySelector('[data-top]');
  const panel = stage.querySelector('#map-panel');
  const picker = stage.querySelector('[data-picker]');
  let padding;
  if (isMobileLayout()) {
    const topHeight = top ? top.getBoundingClientRect().height : 0;
    const panelTop = panel ? panel.getBoundingClientRect().top - stage.getBoundingClientRect().top : stage.clientHeight;
    const bottom = Math.max(120, stage.clientHeight - panelTop);
    padding = { top: topHeight + 24, bottom: Math.min(bottom, stage.clientHeight * 0.6) + 16, left: 16, right: 16 };
  } else {
    const collapsed = stage.classList.contains('is-panel-collapsed');
    padding = { top: 24, bottom: 24, right: 72, left: collapsed || !panel ? 24 : panel.offsetWidth + 40 };
  }
  map.setPadding(padding);
  if (picker) picker.style.inset = `${padding.top}px ${padding.right}px ${padding.bottom}px ${padding.left}px`;
}
