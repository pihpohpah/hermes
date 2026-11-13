import { html, fragment } from '../core/dom.js';
import { icon } from './icon.js';

const ICONS = { info: 'info', success: 'circle-check', danger: 'triangle-alert' };

/**
 * Открытое модальное окно лежит в top layer и делает остальную страницу инертной,
 * поэтому, пока оно открыто, уведомления показываются внутри него.
 */
function toastRegion() {
  const dialog = document.querySelector('dialog[open]');
  if (!dialog) return document.getElementById('toasts');
  let region = dialog.querySelector(':scope > .toast-region');
  if (!region) {
    region = document.createElement('div');
    region.className = 'toast-region';
    region.setAttribute('role', 'status');
    region.setAttribute('aria-live', 'polite');
    dialog.append(region);
  }
  return region;
}

/**
 * Короткое уведомление внизу экрана. Озвучивается через aria-live региона #toasts.
 * toast('Метка опубликована', { type: 'success', action: { label: 'Открыть', onClick }, group: 'pins' })
 */
export function toast(message, { type = 'info', action = null, duration = 4500, group = null } = {}) {
  const region = toastRegion();
  if (!region) return () => {};
  // Уведомления одной группы (например, про маршрут) заменяют друг друга, а не копятся
  if (group) region.querySelectorAll(`[data-group="${group}"]`).forEach((node) => node.remove());
  const el = fragment(html`
    <div class="toast toast--${type}">
      ${icon(ICONS[type] || 'info')}
      <span class="toast__text">${message}</span>
      ${action ? html`<button type="button" class="toast__action">${action.label}</button>` : ''}
    </div>
  `);

  let timer;
  const close = () => {
    clearTimeout(timer);
    el.classList.add('is-leaving');
    el.addEventListener('animationend', () => el.remove(), { once: true });
    setTimeout(() => el.remove(), 400);
  };

  if (action) {
    el.querySelector('.toast__action').addEventListener('click', () => {
      action.onClick();
      close();
    });
  }

  el.addEventListener('mouseenter', () => clearTimeout(timer));
  el.addEventListener('mouseleave', () => {
    timer = setTimeout(close, duration);
  });

  if (group) el.dataset.group = group;
  region.append(el);
  while (region.children.length > 3) region.firstElementChild.remove();
  timer = setTimeout(close, duration);
  return close;
}
