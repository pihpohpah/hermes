/**
 * Минимальный слой работы с DOM без фреймворка.
 *
 * html`…` — шаблонная строка с автоматическим экранированием подстановок. Возвращает SafeHtml,
 * которую можно вкладывать в другие шаблоны без повторного экранирования. Массивы склеиваются,
 * null / undefined / false пропускаются. Для заранее доверенной разметки — raw().
 */

const ESCAPES = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };

export class SafeHtml {
  constructor(value) {
    this.value = value;
  }

  toString() {
    return this.value;
  }
}

export function escapeHtml(value) {
  return String(value).replace(/[&<>"']/g, (ch) => ESCAPES[ch]);
}

export function raw(value) {
  return new SafeHtml(String(value));
}

function serialize(value) {
  if (value === null || value === undefined || value === false) return '';
  if (value instanceof SafeHtml) return value.value;
  if (Array.isArray(value)) return value.map(serialize).join('');
  return escapeHtml(value);
}

export function html(strings, ...values) {
  let out = strings[0];
  for (let i = 0; i < values.length; i += 1) {
    out += serialize(values[i]) + strings[i + 1];
  }
  return new SafeHtml(out);
}

const FOCUS_KEYS = ['data-action', 'data-id', 'data-mode', 'data-scenario', 'data-step', 'data-value', 'data-filter', 'name', 'value', 'href'];

/** Селектор, по которому тот же по смыслу элемент можно найти после перерисовки. */
function focusSelector(el) {
  if (el.id) return `#${CSS.escape(el.id)}`;
  const parts = FOCUS_KEYS.filter((key) => el.hasAttribute(key)).map((key) => `[${key}="${CSS.escape(el.getAttribute(key))}"]`);
  return parts.length ? `${el.tagName.toLowerCase()}${parts.join('')}` : null;
}

/**
 * Заменяет содержимое элемента разметкой.
 * Если фокус был внутри, он возвращается на элемент с теми же data-action / data-id / id —
 * иначе клавиатурный пользователь после каждого клика оказывался бы в начале страницы.
 */
export function render(target, content) {
  const active = document.activeElement;
  const selector = active && active !== document.body && target.contains(active) ? focusSelector(active) : null;
  target.innerHTML = serialize(content);
  if (selector) {
    const next = target.querySelector(selector);
    if (next) next.focus({ preventScroll: true });
  }
  return target;
}

/** Создаёт один элемент из разметки. */
export function fragment(content) {
  const template = document.createElement('template');
  template.innerHTML = serialize(content).trim();
  return template.content.firstElementChild;
}

export const $ = (selector, root = document) => root.querySelector(selector);
export const $$ = (selector, root = document) => Array.from(root.querySelectorAll(selector));

/**
 * Делегированный обработчик: on(root, 'click', '[data-action="x"]', (event, el) => …).
 * Возвращает функцию отписки.
 */
export function on(root, type, selector, handler, options) {
  const listener = (event) => {
    const target = event.target instanceof Element ? event.target.closest(selector) : null;
    if (target && root.contains(target)) handler(event, target);
  };
  root.addEventListener(type, listener, options);
  return () => root.removeEventListener(type, listener, options);
}

/** Набор отписок, который view вызывает при размонтировании. */
export function createDisposer() {
  const fns = [];
  return {
    add(fn) {
      if (typeof fn === 'function') fns.push(fn);
      return fn;
    },
    dispose() {
      while (fns.length) fns.pop()();
    },
  };
}

/** Атрибуты для условного вывода: attr('aria-pressed', true) → aria-pressed="true". */
export function attr(name, value) {
  if (value === null || value === undefined || value === false) return raw('');
  if (value === true) return raw(` ${name}`);
  return raw(` ${name}="${escapeHtml(value)}"`);
}

export function debounce(fn, wait = 200) {
  let timer;
  const debounced = (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), wait);
  };
  debounced.cancel = () => clearTimeout(timer);
  return debounced;
}

export const prefersReducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches;

export const isMobileLayout = () => window.matchMedia('(max-width: 47.99rem)').matches;
