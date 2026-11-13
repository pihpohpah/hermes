/**
 * Hash-роутер: #/путь?параметры.
 * Hash выбран, чтобы прототип работал на любом статическом сервере без настройки rewrite-правил.
 * Для продакшена достаточно заменить parseLocation/navigate на History API — контракт view не изменится.
 */

const routes = [];
let notFound = null;
let current = null;
let listeners = [];

/** Регистрирует маршрут: pattern вида '/place/:id'. */
export function route(pattern, view) {
  const keys = [];
  const regex = new RegExp(
    `^${pattern.replace(/\//g, '\\/').replace(/:(\w+)/g, (_, key) => {
      keys.push(key);
      return '([^/]+)';
    })}\\/?$`,
  );
  routes.push({ pattern, regex, keys, view });
}

export function fallback(view) {
  notFound = view;
}

export function parseLocation(hash = window.location.hash) {
  const value = hash.replace(/^#/, '') || '/';
  const [path, search = ''] = value.split('?');
  return { path: path || '/', query: Object.fromEntries(new URLSearchParams(search)) };
}

function match(path) {
  for (const entry of routes) {
    const result = entry.regex.exec(path);
    if (result) {
      try {
        const params = Object.fromEntries(entry.keys.map((key, i) => [key, decodeURIComponent(result[i + 1])]));
        return { ...entry, params };
      } catch (error) {
        console.warn('Некорректный адрес, показываем 404', error);
        return null;
      }
    }
  }
  return null;
}

export function buildHash(path, query = {}) {
  const params = new URLSearchParams();
  Object.entries(query).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '' && value !== false) params.set(key, value);
  });
  const search = params.toString();
  return `#${path}${search ? `?${search}` : ''}`;
}

/**
 * Переход. replace: true — без новой записи в истории (фильтры, выбор точки).
 * merge: true — дополнить текущие параметры, а не заменить.
 */
export function navigate(path, query = {}, { replace = false, merge = false } = {}) {
  const base = merge ? { ...parseLocation().query, ...query } : query;
  const hash = buildHash(path ?? parseLocation().path, base);
  if (hash === window.location.hash) return;
  if (replace) {
    history.replaceState(null, '', hash);
    resolve();
  } else {
    window.location.hash = hash;
  }
}

/** Обновляет параметры текущего маршрута без новой записи в истории. */
export function setQuery(patch) {
  navigate(null, patch, { replace: true, merge: true });
}

export function onRouteChange(listener) {
  listeners.push(listener);
  return () => {
    listeners = listeners.filter((fn) => fn !== listener);
  };
}

function resolve() {
  const { path, query } = parseLocation();
  const found = match(path);
  const next = found
    ? { pattern: found.pattern, view: found.view, params: found.params, query, path }
    : { pattern: '*', view: notFound, params: {}, query, path };
  const previous = current;
  current = next;
  listeners.forEach((listener) => listener(next, previous));
}

export function startRouter(defaultPath) {
  window.addEventListener('hashchange', resolve);
  if (!window.location.hash || window.location.hash === '#' || window.location.hash === '#/') {
    history.replaceState(null, '', `#${defaultPath}`);
  }
  resolve();
}
