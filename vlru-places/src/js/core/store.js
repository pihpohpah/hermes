/**
 * Глобальное состояние приложения.
 *
 * Хранит только то, что переживает смену экрана: сценарий, пользователя, маршрут, отметки
 * «Я приду», созданные метки, отправленные предложения и историю для рекомендаций. Состояние конкретного экрана
 * (поиск, выбранная точка) живёт в URL — так ссылки остаются рабочими.
 *
 * Часть полей сохраняется в localStorage (ключ STORAGE_KEY), чтобы демо переживало перезагрузку.
 * В продакшене пользователь и его данные приходят с сервера, а localStorage не нужен.
 */

const STORAGE_KEY = 'vlru-places:v1';
const PERSISTED = ['scenario', 'user', 'accounts', 'joinedPins', 'myPins', 'submissions', 'route', 'routeMode', 'history'];

const initialState = {
  scenario: 'city',
  user: null,
  accounts: {},
  joinedPins: [],
  myPins: [],
  submissions: [],
  route: [],
  routeMode: 'foot',
  filters: {},
  /**
   * Сигналы для рекомендаций (src/js/api/history.js), новые — в начале списка.
   * Начальные значения — демо-профиль, чтобы лента «Для вас» не была пустой с первого входа.
   */
  history: {
    viewed: ['mys-tobizina', 'tssi-zarya'],
    visited: ['tokarevskiy-mayak', 'orlinoe-gnezdo', 'millionka'],
    searches: ['водопады', 'закат'],
  },
};

function load() {
  try {
    const saved = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
    const restored = { ...initialState, ...saved };
    if (!['city', 'nature', 'sea'].includes(restored.scenario)) restored.scenario = initialState.scenario;
    return restored;
  } catch (error) {
    console.warn('Не удалось прочитать сохранённое состояние, используется начальное', error);
    return { ...initialState };
  }
}

let state = load();
const listeners = new Set();

function persist() {
  try {
    const subset = Object.fromEntries(PERSISTED.map((key) => [key, state[key]]));
    localStorage.setItem(STORAGE_KEY, JSON.stringify(subset));
  } catch (error) {
    console.warn('Не удалось сохранить состояние', error);
  }
}

export const store = {
  get() {
    return state;
  },

  /** Частичное обновление: объект или функция от текущего состояния. */
  set(patch) {
    const next = typeof patch === 'function' ? patch(state) : patch;
    const changed = Object.keys(next).filter((key) => next[key] !== state[key]);
    if (!changed.length) return;
    state = { ...state, ...next };
    if (changed.some((key) => PERSISTED.includes(key))) persist();
    listeners.forEach((listener) => listener(state, changed));
  },

  /** Подписка; keys ограничивает срабатывание нужными полями. Возвращает отписку. */
  subscribe(listener, keys) {
    const wrapped = keys
      ? (current, changed) => {
          if (changed.some((key) => keys.includes(key))) listener(current, changed);
        }
      : listener;
    listeners.add(wrapped);
    return () => listeners.delete(wrapped);
  },

  /** Сброс демо-данных пользователя (кнопка в профиле). */
  reset() {
    try {
      localStorage.removeItem(STORAGE_KEY);
    } catch (error) {
      console.warn('Не удалось очистить сохранённое состояние', error);
    }
    const previous = state;
    state = { ...initialState };
    listeners.forEach((listener) => listener(state, Object.keys(previous)));
  },
};
