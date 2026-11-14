/* Места VL.ru — базовые утилиты клиента (DOM, форматирование, хранилище). */

export const $ = (sel, root = document) => root.querySelector(sel);
export const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

export const debounce = (fn, ms) => {
  let timer;
  const wrapped = (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  };
  wrapped.cancel = () => clearTimeout(timer);
  return wrapped;
};

export const escapeHtml = (value) =>
  String(value).replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));

export const icon = (name, cls = '') =>
  `<svg class="icon${cls ? ` ${cls}` : ''}" aria-hidden="true"><use href="/icons.svg#i-${name}"></use></svg>`;

export const plural = (n, forms) => {
  const abs = Math.abs(n) % 100;
  const mod = abs % 10;
  if (abs > 10 && abs < 20) return forms[2];
  if (mod > 1 && mod < 5) return forms[1];
  if (mod === 1) return forms[0];
  return forms[2];
};
export const countLabel = (n, forms) => `${n} ${plural(n, forms)}`;

export const formatDistance = (km) => (km < 1 ? `${Math.round(km * 1000)} м` : `${(Math.round(km * 10) / 10).toFixed(km < 10 ? 1 : 0)} км`);

export const formatDuration = (minutes) => {
  const total = Math.max(1, Math.round(minutes));
  if (total < 60) return `${total} мин`;
  const hours = Math.floor(total / 60);
  const rest = total % 60;
  if (hours >= 24) {
    const days = Math.round(hours / 24);
    return plural(days, ['день', 'дня', 'дней']);
  }
  if (!rest) return `${hours} ${plural(hours, ['час', 'часа', 'часов'])}`;
  return `${hours} ч ${rest} мин`;
};

export const isMobileLayout = () => window.matchMedia('(max-width: 47.99rem)').matches;

export const storage = {
  get(key, fallback = null) {
    try {
      const raw = localStorage.getItem(key);
      return raw === null ? fallback : JSON.parse(raw);
    } catch {
      return fallback;
    }
  },
  set(key, value) {
    try {
      localStorage.setItem(key, JSON.stringify(value));
    } catch {
      /* приватный режим — состояние просто не сохранится */
    }
  },
  remove(key) {
    try {
      localStorage.removeItem(key);
    } catch {
      /* см. выше */
    }
  },
};

export const currentParams = () => new URLSearchParams(location.search);
