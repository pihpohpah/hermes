/**
 * Места VL.ru — клиентская логика (htmx + Alpine.js).
 *
 * Сервер отдаёт страницы и HTML-фрагменты (htmx их подменяет). Здесь живёт то, что
 * декларативно не выражается: карта (map.js), диалоги, маршрут, метки «Сейчас»,
 * мастер добавления и делегированные обработчики поиска/фильтров.
 */
(() => {
  'use strict';

  /* ── Утилиты ── */

  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

  const debounce = (fn, ms) => {
    let timer;
    const wrapped = (...args) => {
      clearTimeout(timer);
      timer = setTimeout(() => fn(...args), ms);
    };
    wrapped.cancel = () => clearTimeout(timer);
    return wrapped;
  };

  const escapeHtml = (value) =>
    String(value).replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));

  const icon = (name, cls = '') =>
    `<svg class="icon${cls ? ` ${cls}` : ''}" aria-hidden="true"><use href="/icons.svg#i-${name}"></use></svg>`;

  const plural = (n, forms) => {
    const abs = Math.abs(n) % 100;
    const mod = abs % 10;
    if (abs > 10 && abs < 20) return forms[2];
    if (mod > 1 && mod < 5) return forms[1];
    if (mod === 1) return forms[0];
    return forms[2];
  };
  const countLabel = (n, forms) => `${n} ${plural(n, forms)}`;

  const formatDistance = (km) => (km < 1 ? `${Math.round(km * 1000)} м` : `${(Math.round(km * 10) / 10).toFixed(km < 10 ? 1 : 0)} км`);

  const formatDuration = (minutes) => {
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

  const isMobileLayout = () => window.matchMedia('(max-width: 47.99rem)').matches;

  const storage = {
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

  /* ── Запросы к API ──
   * Мутации идут через fetch (htmx отвечает за GET-фрагменты). Сервер отдаёт
   * обёртку {success, error, data} и выдаёт csrf-токен после входа. */

  const CSRF_KEY = 'vlru-places:csrf';

  class ApiError extends Error {
    constructor(message, { status = 0, code = '' } = {}) {
      super(message);
      this.status = status;
      this.code = code;
    }
  }

  async function api(path, { method = 'GET', body, formData } = {}) {
    const headers = { Accept: 'application/json' };
    const token = storage.get(CSRF_KEY, '');
    if (token) headers['X-CSRF-Token'] = token;

    const options = { method, headers, credentials: 'same-origin' };
    if (formData) {
      options.body = formData;
    } else if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
      options.body = JSON.stringify(body);
    }

    const response = await fetch(path, options);
    let payload = null;
    try {
      payload = await response.json();
    } catch {
      /* пустой или не-JSON ответ обрабатываем по статусу */
    }

    if (payload && typeof payload === 'object' && 'success' in payload) {
      if (payload.success) return payload.data;
      throw new ApiError(payload.error || 'Что-то пошло не так', { status: response.status, code: payload.code || '' });
    }
    if (!response.ok) throw new ApiError(`HTTP ${response.status}`, { status: response.status });
    return payload;
  }

  /* ── Тосты ── */

  const TOAST_ICONS = { info: 'info', success: 'circle-check', danger: 'triangle-alert' };

  function toastRegion() {
    const open = document.querySelector('dialog[open]');
    if (!open) return document.getElementById('toasts');
    let region = open.querySelector(':scope > .toast-region');
    if (!region) {
      region = document.createElement('div');
      region.className = 'toast-region';
      region.setAttribute('role', 'status');
      region.setAttribute('aria-live', 'polite');
      open.append(region);
    }
    return region;
  }

  function toast(message, { type = 'info', action = null, duration = 4500, group = null } = {}) {
    const region = toastRegion();
    if (!region) return () => {};
    if (group) region.querySelectorAll(`[data-group="${group}"]`).forEach((node) => node.remove());

    const el = document.createElement('div');
    el.className = `toast toast--${type}`;
    el.innerHTML = `${icon(TOAST_ICONS[type] || 'info')}<span class="toast__text">${escapeHtml(message)}</span>${
      action ? '<button type="button" class="toast__action"></button>' : ''
    }`;

    let timer;
    const close = () => {
      clearTimeout(timer);
      el.classList.add('is-leaving');
      setTimeout(() => el.remove(), 400);
    };

    if (action) {
      const button = el.querySelector('.toast__action');
      button.textContent = action.label;
      button.addEventListener('click', () => {
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

  /* ── Диалоги ── */

  function openDialog({ title = '', lead = '', body = '', footer = '', className = '', wide = false, onMount } = {}) {
    const dialog = document.createElement('dialog');
    dialog.className = ['dialog', wide ? 'dialog--wide' : '', className].filter(Boolean).join(' ');
    dialog.innerHTML = `
      <div class="dialog__header">
        <h2 class="dialog__title">${escapeHtml(title)}</h2>
        <button type="button" class="icon-btn" data-dialog-close aria-label="Закрыть">${icon('x')}</button>
      </div>
      ${lead ? `<p class="dialog__lead">${lead}</p>` : ''}
      <div class="dialog__body">${body}</div>
      ${footer ? `<div class="dialog__footer">${footer}</div>` : ''}`;

    const apiEl = {
      el: dialog,
      close: () => dialog.close(),
      setContent({ body: nextBody, footer: nextFooter, lead: nextLead }) {
        const bodyNode = $('.dialog__body', dialog);
        if (nextBody !== undefined) bodyNode.innerHTML = nextBody;
        const footerNode = $('.dialog__footer', dialog);
        if (nextFooter !== undefined) {
          if (footerNode) footerNode.innerHTML = nextFooter;
          else if (nextFooter) {
            const node = document.createElement('div');
            node.className = 'dialog__footer';
            node.innerHTML = nextFooter;
            dialog.append(node);
          }
        }
        const leadNode = $('.dialog__lead', dialog);
        if (nextLead !== undefined && leadNode) leadNode.innerHTML = nextLead;
      },
    };

    dialog.addEventListener('click', (event) => {
      if (event.target === dialog || event.target.closest('[data-dialog-close]')) dialog.close();
    });
    dialog.addEventListener('close', () => dialog.remove());
    dialog.addEventListener('cancel', () => dialog.remove());

    document.body.append(dialog);
    dialog.showModal();
    onMount?.(dialog, () => dialog.close(), apiEl);
    return apiEl;
  }

  function confirmDialog({ title, text = '', confirmLabel = 'Подтвердить', danger = false }) {
    return new Promise((resolve) => {
      let done = false;
      openDialog({
        title,
        body: text ? `<p>${escapeHtml(text)}</p>` : '',
        footer: `<button type="button" class="btn btn--ghost" data-dialog-close>Отмена</button>
          <button type="button" class="btn ${danger ? 'btn--danger' : 'btn--primary'}" data-confirm>${escapeHtml(confirmLabel)}</button>`,
        onMount(el, close) {
          $('[data-confirm]', el).addEventListener('click', () => {
            done = true;
            close();
          });
          el.addEventListener('close', () => resolve(done));
        },
      });
    });
  }

  /* ── Авторизация ──
   * Бэкенд выдаёт csrf-токен после POST /users; его наличие в localStorage —
   * признак «вошёл» для этого клиента. */

  const isLoggedIn = () => Boolean(storage.get(CSRF_KEY, ''));

  function openAuthDialog({ reason = '' } = {}) {
    return new Promise((resolve) => {
      let result = null;
      openDialog({
        title: 'Вход в VL.ru',
        lead: escapeHtml(reason || 'Имя увидят участники ваших меток и редакторы справочника.'),
        className: 'dialog--auth',
        body: `
          <form class="auth-form" novalidate>
            <div class="field">
              <label class="field__label" for="auth-name">Имя</label>
              <input class="input" id="auth-name" name="name" type="text" autocomplete="given-name" maxlength="40" placeholder="Например, Аня" required />
              <p class="field__error" id="auth-name-error" role="alert"></p>
            </div>
            <label class="check">
              <input type="checkbox" name="rules" />
              <span class="check__text">Согласен с правилами меток: без рекламы, личных данных и оскорблений</span>
            </label>
            <p class="field__error" id="auth-rules-error" role="alert"></p>
            <button type="submit" class="btn btn--primary btn--lg btn--block">Войти</button>
          </form>`,
        onMount(el, close) {
          const form = $('form', el);
          const nameInput = form.elements.name;
          const nameError = $('#auth-name-error', form);
          const rulesError = $('#auth-rules-error', form);
          setTimeout(() => nameInput.focus(), 50);

          form.addEventListener('submit', async (event) => {
            event.preventDefault();
            const name = nameInput.value.trim();
            if (name.length < 2) {
              nameError.textContent = 'Представьтесь — хотя бы пара букв';
              nameInput.focus();
              return;
            }
            nameError.textContent = '';
            if (!form.elements.rules.checked) {
              rulesError.textContent = 'Чтобы продолжить, примите правила';
              return;
            }
            rulesError.textContent = '';

            const submit = $('button[type="submit"]', form);
            submit.classList.add('is-loading');
            try {
              const data = await api('/users', {
                method: 'POST',
                body: { display_name: name, timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Vladivostok' },
              });
              storage.set(CSRF_KEY, data.csrf_token || '');
              result = data;
              toast(`Готово, ${data.display_name}. Теперь можно ставить метки`, { type: 'success' });
              close();
            } catch (error) {
              nameError.textContent = error.message || 'Не получилось войти. Попробуйте ещё раз';
            } finally {
              submit.classList.remove('is-loading');
            }
          });

          el.addEventListener('close', () => resolve(result));
        },
      });
    });
  }

  /** Выполнить действие, сначала запросив вход для гостя. */
  function requireAuth(reason) {
    if (isLoggedIn()) return Promise.resolve(true);
    return openAuthDialog({ reason }).then((user) => Boolean(user));
  }

  /* ── Справочники (зеркало presenter/taxonomy.go для JS-диалогов) ── */

  const SCENARIO_LABELS = { city: 'Город', nature: 'Природа', sea: 'Море' };

  const CATEGORY_LABELS = {
    city: { walks: 'Прогулки', viewpoints: 'Смотровые', history: 'История', museums: 'Музеи', art: 'Арт-пространства', stage: 'Музыка и сцена' },
    nature: { hiking: 'Сопки и тропы', waterfalls: 'Водопады', lakes: 'Озёра', parks: 'Парки и сады', capes: 'Мысы и скалы', reserves: 'Заповедные места' },
    sea: { beaches: 'Пляжи', bays: 'Бухты', lighthouses: 'Маяки', islands: 'Острова', diving: 'Дайвинг', boats: 'Морские прогулки' },
    event: { concert: 'Концерт', exhibition: 'Выставка', tour: 'Экскурсия', festival: 'Фестиваль', sport: 'Спорт', party: 'Андеграунд', lecture: 'Лекция', market: 'Маркет' },
  };

  const TYPE_LABELS = { place: 'Место', event: 'Событие', fix: 'Правка карточки' };

  const DISTANCE_OPTIONS = [
    { value: '', label: 'Любое' },
    { value: '30', label: 'До 30 км' },
    { value: '100', label: 'До 100 км' },
    { value: '250', label: 'До 250 км' },
  ];

  const DIFFICULTY_OPTIONS = [
    { value: 'easy', label: 'Лёгкая' },
    { value: 'medium', label: 'Средняя' },
    { value: 'hard', label: 'Сложная' },
  ];

  const BEACH_OPTIONS = [
    { value: 'sand', label: 'Песок' },
    { value: 'pebble', label: 'Галька' },
    { value: 'rocks', label: 'Скалы' },
  ];

  // Зеркало presenter.Filters: набор полей расширенных фильтров по сценарию.
  const FILTERS = {
    city: [
      { id: 'free', type: 'toggle', label: 'Бесплатно' },
      { id: 'walkable', type: 'toggle', label: 'Пешком от центра', hint: 'Не дальше 2,5 км от Центральной площади' },
      { id: 'kids', type: 'toggle', label: 'Можно с детьми' },
    ],
    nature: [
      { id: 'distance', type: 'single', label: 'Расстояние от центра', options: DISTANCE_OPTIONS },
      { id: 'difficulty', type: 'multi', label: 'Сложность', options: DIFFICULTY_OPTIONS },
      { id: 'noCar', type: 'toggle', label: 'Можно без машины', hint: 'Автобус, электричка или паром' },
      { id: 'free', type: 'toggle', label: 'Бесплатно' },
    ],
    sea: [
      { id: 'beach', type: 'multi', label: 'Берег', options: BEACH_OPTIONS },
      { id: 'distance', type: 'single', label: 'Расстояние от центра', options: DISTANCE_OPTIONS },
      { id: 'swim', type: 'toggle', label: 'Можно купаться', hint: 'Есть купальный сезон' },
      { id: 'noCar', type: 'toggle', label: 'Можно без машины', hint: 'Автобус, электричка или паром' },
    ],
  };

  const FILTER_IDS = ['free', 'walkable', 'kids', 'distance', 'difficulty', 'noCar', 'beach', 'swim'];

  /* ── Маршрут: построение пути (порт map/routing.js) ── */

  const ROUTE_MODES = [
    { id: 'foot', label: 'Пешком' },
    { id: 'car', label: 'Авто' },
  ];

  const MAX_STOPS = 6;

  const OSRM_PROFILES = {
    foot: { url: 'https://routing.openstreetmap.de/routed-foot/route/v1/foot', speedKmh: 4.5 },
    car: { url: 'https://routing.openstreetmap.de/routed-car/route/v1/driving', speedKmh: 40 },
  };

  const ROUTE_TIMEOUT_MS = 7000;
  const routeCache = new Map();

  function haversineKm(a, b) {
    const rad = Math.PI / 180;
    const dLat = (b[1] - a[1]) * rad;
    const dLon = (b[0] - a[0]) * rad;
    const s = Math.sin(dLat / 2) ** 2 + Math.cos(a[1] * rad) * Math.cos(b[1] * rad) * Math.sin(dLon / 2) ** 2;
    return 2 * 6371 * Math.asin(Math.sqrt(s));
  }

  function straightLine(stops, mode) {
    const profile = OSRM_PROFILES[mode] || OSRM_PROFILES.foot;
    const legs = stops.slice(1).map((stop, i) => {
      const km = haversineKm(stops[i].coords, stop.coords) * 1.25; // поправка на непрямолинейность дорог
      return { km, minutes: (km / profile.speedKmh) * 60 };
    });
    return {
      approximate: true,
      geometry: { type: 'LineString', coordinates: stops.map((s) => s.coords) },
      km: legs.reduce((sum, leg) => sum + leg.km, 0),
      minutes: legs.reduce((sum, leg) => sum + leg.minutes, 0),
      legs,
    };
  }

  /** @returns {Promise<{approximate, geometry, km, minutes, legs}>|null} */
  async function buildRoute(stops, mode = 'foot') {
    if (stops.length < 2) return null;
    const key = `${mode}:${stops.map((s) => s.coords.join(',')).join(';')}`;
    if (routeCache.has(key)) return routeCache.get(key);

    const coords = stops.map((s) => s.coords.map((n) => n.toFixed(5)).join(',')).join(';');
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), ROUTE_TIMEOUT_MS);

    try {
      const response = await fetch(`${OSRM_PROFILES[mode].url}/${coords}?overview=full&geometries=geojson&steps=false`, {
        signal: controller.signal,
      });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const data = await response.json();
      const best = data.routes?.[0];
      if (data.code !== 'Ok' || !best) throw new Error(data.code || 'NoRoute');

      const result = {
        approximate: false,
        geometry: best.geometry,
        km: best.distance / 1000,
        minutes: best.duration / 60,
        legs: best.legs.map((leg) => ({ km: leg.distance / 1000, minutes: leg.duration / 60 })),
      };
      routeCache.set(key, result);
      return result;
    } catch (error) {
      console.warn('Маршрут: сервис недоступен, считаем по прямой', error);
      return straightLine(stops, mode);
    } finally {
      clearTimeout(timer);
    }
  }

  /* ── Маршрут: состояние ── */

  const ROUTE_KEY = 'vlru-places:route';
  const ROUTE_MODE_KEY = 'vlru-places:route-mode';

  const routeState = {
    stops: [],
    result: null,
    computing: false,
    version: 0,
    listeners: new Set(),
    subscribe(fn) {
      this.listeners.add(fn);
      return () => this.listeners.delete(fn);
    },
    notify() {
      this.listeners.forEach((fn) => fn(this));
    },
    async recompute() {
      const current = ++this.version;
      const ids = storage.get(ROUTE_KEY, []);
      const mode = storage.get(ROUTE_MODE_KEY, 'foot');
      this.computing = ids.length >= 2;

      const loaded = await Promise.all(ids.map((id) => fetchItem(id).catch(() => null)));
      if (current !== this.version) return;
      this.stops = loaded.filter(Boolean);
      if (this.stops.length !== ids.length) persistRoute(this.stops.map((s) => s.id));

      this.result = this.stops.length >= 2 ? await buildRoute(this.stops, mode) : null;
      if (current !== this.version) return;
      this.computing = false;

      paintRouteOnMap();
      this.notify();
      announceRoute();
    },
  };

  function persistRoute(ids) {
    storage.set(ROUTE_KEY, ids);
    routeState.recompute();
  }

  function toggleRoute(id, title = '') {
    const ids = storage.get(ROUTE_KEY, []);
    if (ids.includes(id)) {
      storage.set(ROUTE_KEY, ids.filter((stopId) => stopId !== id));
      routeState.recompute();
      toast(`Убрали из маршрута${title ? `: ${title}` : ''}`, { group: 'route' });
      return;
    }
    if (ids.length >= MAX_STOPS) {
      toast(`В маршруте может быть до ${MAX_STOPS} точек`, { type: 'danger', group: 'route' });
      return;
    }
    storage.set(ROUTE_KEY, [...ids, id]);
    routeState.recompute();
    const count = ids.length + 1;
    toast(count === 1 ? 'Первая точка маршрута. Добавьте ещё одну' : `В маршруте ${countLabel(count, ['точка', 'точки', 'точек'])}`, {
      type: 'success',
      group: 'route',
      action: count >= 2 ? { label: 'Показать', onClick: () => openRouteEditor() } : null,
    });
  }

  function moveStop(id, delta) {
    const route = [...storage.get(ROUTE_KEY, [])];
    const index = route.indexOf(id);
    const target = index + delta;
    if (index < 0 || target < 0 || target >= route.length) return;
    [route[index], route[target]] = [route[target], route[index]];
    storage.set(ROUTE_KEY, route);
    routeState.recompute();
  }

  function clearRoute() {
    storage.set(ROUTE_KEY, []);
    routeState.recompute();
  }

  function setRouteMode(mode) {
    storage.set(ROUTE_MODE_KEY, mode);
    routeState.recompute();
  }

  function paintRouteOnMap() {
    if (!window.placesMap) return;
    const ids = storage.get(ROUTE_KEY, []);
    window.placesMap.setRoute(
      ids.length
        ? { stops: routeState.stops, geometry: routeState.result?.geometry || null, approximate: routeState.result?.approximate || false }
        : null,
    );
  }

  /** Озвучка итога маршрута для скринридера. */
  function announceRoute() {
    const announcer = document.querySelector('[data-route-announcer]');
    if (!announcer) return;
    const ids = storage.get(ROUTE_KEY, []);
    if (!ids.length) {
      announcer.textContent = '';
      return;
    }
    announcer.textContent = `Маршрут, ${countLabel(ids.length, ['точка', 'точки', 'точек'])}: ${routeSummaryText()}`;
  }

  function routeSummaryText() {
    const { stops, result, computing } = routeState;
    if (stops.length < 2) return 'Добавьте ещё одну точку';
    if (computing || !result) return 'Считаем маршрут…';
    const mode = storage.get(ROUTE_MODE_KEY, 'foot') === 'car' ? 'на машине' : 'пешком';
    return `${formatDistance(result.km)} · ${formatDuration(result.minutes)} ${mode}${result.approximate ? ', примерно' : ''}`;
  }

  /** URL редактора маршрута: текущая страница с panel=route. */
  function routeEditorURL() {
    const params = new URLSearchParams(location.search);
    params.set('panel', 'route');
    return `${location.pathname}?${params.toString()}`;
  }

  function openRouteEditor() {
    htmx.ajax('GET', `/fragments/map-panel?${new URLSearchParams(location.search).toString()}`, { target: '#map-panel-inner' });
    const params = new URLSearchParams(location.search);
    params.set('panel', 'route');
    history.pushState({}, '', `${location.pathname}?${params.toString()}`);
  }

  const itemCache = new Map();

  async function fetchItem(id) {
    if (itemCache.has(id)) return itemCache.get(id);
    const item = await api(`/api/items/${encodeURIComponent(id)}`);
    if (item) {
      itemCache.set(
        id,
        item.has_coords
          ? { id: item.id, title: item.title, coords: [item.lon, item.lat], url: item.type === 'event' ? `/event/${item.id}` : `/place/${item.id}` }
          : { id: item.id, title: item.title, coords: null, url: item.type === 'event' ? `/event/${item.id}` : `/place/${item.id}` },
      );
    }
    return itemCache.get(id) || null;
  }

  /** Открыть редактор маршрута в панели карты (если её нет — перейти на карту). */
  function ensureRouteEditor() {
    if ($('#map-stage')) {
      openRouteEditor();
    } else {
      location.href = `/?${new URLSearchParams({ panel: 'route' }).toString()}`;
    }
  }

  /* ── Alpine: store и компоненты ── */

  document.addEventListener('alpine:init', () => {
    // Состав маршрута — в Alpine store, чтобы x-bind:aria-pressed на кнопках
    // «В маршрут» реагировал на изменения без ручной перерисовки.
    Alpine.store('route', {
      ids: storage.get(ROUTE_KEY, []),
      has(id) {
        return this.ids.includes(id);
      },
    });

    const syncRouteStore = () => {
      Alpine.store('route').ids = storage.get(ROUTE_KEY, []);
    };

    const routeEditorOpen = () => new URLSearchParams(location.search).get('panel') === 'route';

    Alpine.data('routeBar', () => ({
      visible: false,
      stops: [],
      summary: '',
      modes: ROUTE_MODES,
      editorURL: '/?panel=route',
      init() {
        const refresh = () => {
          this.visible = Alpine.store('route').ids.length > 0 && !routeEditorOpen();
          document.getElementById('map-stage')?.classList.toggle('has-route', this.visible);
        };
        this.$watch('$store.route.ids', () => {
          refresh();
          routeState.recompute();
        });
        this.unsubscribe = routeState.subscribe(() => {
          this.stops = routeState.stops;
          this.summary = routeSummaryText();
          this.editorURL = routeEditorURL();
        });
        refresh();
        routeState.recompute();
      },
      stopsTitle() {
        return `Маршрут · ${countLabel(this.stops.length, ['точка', 'точки', 'точек'])}`;
      },
      setMode(mode) {
        setRouteMode(mode);
      },
      destroy() {
        this.unsubscribe?.();
      },
    }));

    Alpine.data('routeEditor', () => ({
      stops: [],
      modes: ROUTE_MODES,
      mode: storage.get(ROUTE_MODE_KEY, 'foot'),
      distance: '…',
      duration: '…',
      approximate: false,
      yandexUrl: '',
      init() {
        this.unsubscribe = routeState.subscribe(() => this.paint());
        this.paint();
        routeState.recompute().then(() => {
          const coords = routeState.stops.map((s) => s.coords).filter(Boolean);
          if (coords.length) window.placesMap?.fitTo(coords);
        });
      },
      paint() {
        const legs = routeState.result?.legs || [];
        this.stops = routeState.stops.map((stop, index) => ({
          ...stop,
          leg: legs[index] ? `→ ${formatDistance(legs[index].km)}, ${formatDuration(legs[index].minutes)}` : '',
        }));
        this.mode = storage.get(ROUTE_MODE_KEY, 'foot');
        this.approximate = Boolean(routeState.result?.approximate);
        this.distance = routeState.computing || !routeState.result ? '…' : formatDistance(routeState.result.km);
        this.duration = routeState.computing || !routeState.result ? '…' : formatDuration(routeState.result.minutes);
        const coords = this.stops.map((s) => s.coords).filter(Boolean).map((c) => `${c[1]},${c[0]}`).join('~');
        this.yandexUrl = coords ? `https://yandex.ru/maps/?rtext=${coords}&rtt=${this.mode === 'car' ? 'auto' : 'pd'}` : '';
      },
      setMode(mode) {
        setRouteMode(mode);
      },
      move(index, delta) {
        const ids = storage.get(ROUTE_KEY, []);
        const target = index + delta;
        if (index < 0 || target < 0 || target >= ids.length) return;
        [ids[index], ids[target]] = [ids[target], ids[index]];
        storage.set(ROUTE_KEY, ids);
        routeState.recompute();
      },
      remove(id) {
        toggleRoute(id);
        syncRouteStore();
      },
      destroy() {
        this.unsubscribe?.();
      },
    }));

    /* «Сейчас»: создание метки — два шага, черновик шага в sessionStorage. */

    const NOW_STEP_KEY = 'vlru-places:now-create';

    const pickerNode = () => document.querySelector('#map-stage [data-picker]');

    const setSheetState = (state) => {
      const panel = document.getElementById('map-panel');
      if (!panel || !isMobileLayout()) return;
      panel.dataset.sheet = state;
      const handle = panel.querySelector('.map-panel__handle');
      handle?.setAttribute('aria-expanded', String(state !== 'peek'));
    };

    Alpine.data('nowCreate', () => ({
      step: 'list',
      draft: { placeLabel: '', kind: '', title: '', startsInMin: 0, durationMin: 120, need: 0, note: '' },
      errors: {},
      sending: false,
      init() {
        const saved = storage.get(NOW_STEP_KEY, null);
        if (saved === 'place' || saved === 'details') {
          this.step = saved;
          if (saved === 'place') this.showPicker(true);
        }
      },
      showPicker(visible) {
        const node = pickerNode();
        if (node) node.hidden = !visible;
      },
      start() {
        requireAuth('Чтобы поставить метку, войдите — это займёт минуту.').then((ok) => {
          if (!ok) return;
          this.draft = { placeLabel: '', kind: '', title: '', startsInMin: 0, durationMin: 120, need: 0, note: '' };
          this.errors = {};
          this.step = 'place';
          storage.set(NOW_STEP_KEY, 'place');
          this.showPicker(true);
          setSheetState('full');
        });
      },
      cancel() {
        this.step = 'list';
        this.errors = {};
        storage.remove(NOW_STEP_KEY);
        this.showPicker(false);
      },
      submitPlace() {
        this.draft.coords = window.placesMap ? window.placesMap.getCenter() : null;
        this.step = 'details';
        storage.set(NOW_STEP_KEY, 'details');
        this.showPicker(false);
      },
      async submitDetails() {
        const errors = {};
        if (!this.draft.kind) errors.kind = 'Выберите тип метки';
        if (this.draft.title.trim().length < 5) errors.title = 'Опишите встречу хотя бы в пяти символах';
        this.errors = errors;
        if (Object.keys(errors).length) return;

        this.sending = true;
        try {
          const now = Date.now();
          const startsAt = new Date(now + Number(this.draft.startsInMin) * 60000);
          const endsAt = new Date(now + (Number(this.draft.startsInMin) + Number(this.draft.durationMin)) * 60000);
          const [lon, lat] = this.draft.coords || [0, 0];
          const pin = await api('/pins', {
            method: 'POST',
            body: {
              kind: this.draft.kind,
              title: this.draft.title.trim(),
              note: this.draft.note.trim(),
              place_label: this.draft.placeLabel.trim() || 'Точка на карте',
              lat,
              lon,
              starts_at: startsAt.toISOString(),
              ends_at: endsAt.toISOString(),
              need: Number(this.draft.need) || 0,
            },
          });
          storage.remove(NOW_STEP_KEY);
          this.step = 'list';
          toast(`Метка опубликована. Исчезнет через ${formatDuration((new Date(pin.ends_at) - Date.now()) / 60000)}`, { type: 'success' });
          await refreshPins();
          selectPin(pin.id);
        } catch (error) {
          console.error('Не удалось опубликовать метку', error);
          toast(error.status === 401 ? 'Войдите, чтобы опубликовать метку' : 'Не получилось опубликовать. Попробуйте ещё раз', { type: 'danger' });
        } finally {
          this.sending = false;
        }
      },
      destroy() {
        this.showPicker(false);
      },
    }));

    /* Добавление места/события/правки: трёхшаговый мастер, черновик в localStorage. */

    const ADD_DRAFT_KEY = 'vlru-places:add-draft';
    const DESCRIPTION_MIN = 30;
    const MAX_PHOTOS = 5;
    let photoSeq = 0;
    const photoKey = () => `p${Date.now().toString(36)}-${++photoSeq}`;

    Alpine.data('addMaster', () => ({
      step: 1,
      steps: ['Что', 'Подробности', 'Отправка'],
      draft: null,
      errors: {},
      sending: false,
      pickerVisible: false,
      pickerMap: null,
      pickerMarker: null,
      init() {
        const params = new URLSearchParams(location.search);
        this.step = Math.min(3, Math.max(1, Number(params.get('step')) || 1));

        const saved = storage.get(ADD_DRAFT_KEY, null);
        const fixItem = this.$el.dataset.fixItem || '';
        this.draft = {
          type: params.get('type') || 'place',
          scenario: params.get('scenario') || 'city',
          category: '',
          fixReasons: [],
          title: fixItem,
          address: '',
          coords: null,
          date: '',
          time: '',
          description: '',
          hours: '',
          price: '',
          link: '',
          consent: false,
          photos: [],
          ...(saved && saved.v === 1 ? saved.draft : {}),
          // Обрыв загрузки не переживает: файлов уже нет, серверные URL — есть.
          photos: (saved?.draft?.photos || []).filter((p) => p && p.src).map((p) => ({ ...p, key: photoKey() })),
        };
        this.$watch('draft', () => this.persist(), { deep: true });
      },
      persist() {
        storage.set(ADD_DRAFT_KEY, {
          v: 1,
          draft: {
            ...this.draft,
            photos: this.draft.photos
              .filter((p) => p.src)
              .map(({ src, src_sm, width, height, url }) => ({ src, src_sm, width, height, url })),
          },
        });
      },
      get photosBusy() {
        return this.draft.photos.some((p) => p.uploading);
      },
      gotoStep(n) {
        this.step = Math.min(3, Math.max(1, n));
        const params = new URLSearchParams(location.search);
        if (this.step === 1) params.delete('step');
        else params.set('step', String(this.step));
        const query = params.toString();
        history.pushState({}, '', `${location.pathname}${query ? `?${query}` : ''}`);
        this.$nextTick(() => {
          this.$refs.stepTitle?.focus();
          if (this.step === 2) this.scrollTop();
        });
      },
      scrollTop() {
        window.scrollTo({ top: 0, behavior: 'smooth' });
      },
      typeLabel() {
        return TYPE_LABELS[this.draft.type] || this.draft.type;
      },
      scenarioLabel() {
        return SCENARIO_LABELS[this.draft.scenario] || this.draft.scenario;
      },
      categoryLabel() {
        const byType = this.draft.type === 'event' ? CATEGORY_LABELS.event : CATEGORY_LABELS[this.draft.scenario];
        return byType?.[this.draft.category] || this.draft.category || '—';
      },
      whenLabel() {
        if (!this.draft.date) return '';
        const [year, month, day] = this.draft.date.split('-').map(Number);
        const date = new Date(year, month - 1, day);
        const months = ['января', 'февраля', 'марта', 'апреля', 'мая', 'июня', 'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря'];
        return `${day} ${months[month - 1]}${this.draft.time ? ` в ${this.draft.time}` : ''}`;
      },
      validate(step) {
        const errors = {};
        if (step === 1) {
          if (this.draft.type === 'fix') {
            if (!this.draft.fixReasons.length) errors.fixReasons = 'Отметьте хотя бы один пункт';
          } else if (!this.draft.category) {
            errors.category = 'Выберите категорию';
          }
        }
        if (step === 2) {
          if (this.draft.title.trim().length < 3) errors.title = 'Напишите название — хотя бы три буквы';
          if (this.draft.type !== 'fix' && this.draft.address.trim().length < 3 && !this.draft.coords) {
            errors.address = 'Укажите адрес, ориентир или точку на карте';
          }
          if (this.draft.type === 'event') {
            if (!this.draft.date) errors.date = 'Выберите дату';
            if (!this.draft.time) errors.time = 'Укажите время начала';
          }
          const min = this.draft.type === 'fix' ? 10 : DESCRIPTION_MIN;
          if (this.draft.description.trim().length < min) {
            errors.description = `Добавьте пару предложений — хотя бы ${min} символов`;
          }
        }
        if (step === 3 && !this.draft.consent) {
          errors.consent = 'Подтвердите, что информация достоверна';
        }
        this.errors = errors;
        return !Object.keys(errors).length;
      },
      submit() {
        if (this.sending) return;
        if (!this.validate(this.step)) return;
        if (this.step < 3) {
          this.gotoStep(this.step + 1);
          return;
        }
        this.send();
      },
      // Фото уходят на сервер сразу при выборе: в черновике лежит либо
      // заглушка с object URL (uploading), либо серверный {src, src_sm}.
      // key у каждого превью своё — иначе x-for по индексам путается при
      // удалении оборвавшихся загрузок.
      addPhotos(event) {
        const files = Array.from(event.target.files || []).filter((file) => file.type.startsWith('image/'));
        const room = MAX_PHOTOS - this.draft.photos.length;
        if (files.length > room) toast(`Можно добавить ещё ${room}. Лишние фото не загрузили`, { type: 'danger' });
        files.slice(0, room).forEach((file) => this.uploadPhoto(file));
        event.target.value = '';
      },
      async uploadPhoto(file) {
        const localURL = URL.createObjectURL(file);
        this.draft.photos.push({ key: photoKey(), file, url: localURL, uploading: true });
        const index = this.draft.photos.length - 1;
        try {
          const data = await api('/photos', { method: 'POST', formData: (() => { const fd = new FormData(); fd.append('file', file); return fd; })() });
          // Меняем запись через индекс массива: локальный объект после push —
          // сырая ссылка, мутировать его реактивность не заметит.
          this.draft.photos[index] = {
            ...this.draft.photos[index],
            file: null,
            uploading: false,
            src: data.url,
            src_sm: data.url_sm,
            width: data.width,
            height: data.height,
            url: data.url_sm || data.url,
          };
          URL.revokeObjectURL(localURL);
        } catch (error) {
          console.warn('Фото не загрузилось', error);
          if (this.draft.photos[index] && this.draft.photos[index].file === file) this.draft.photos.splice(index, 1);
          URL.revokeObjectURL(localURL);
          toast('Фото не загрузилось — попробуйте другое или меньший размер', { type: 'danger' });
        }
      },
      removePhoto(index) {
        const [removed] = this.draft.photos.splice(index, 1);
        if (removed?.file) URL.revokeObjectURL(removed.url);
      },
      togglePicker() {
        this.pickerVisible = !this.pickerVisible;
        if (!this.pickerVisible) {
          this.draft.coords = null;
          this.destroyPicker();
          return;
        }
        this.$nextTick(() => {
          const canvas = this.$el.querySelector('[data-picker-map]');
          if (!canvas || !window.maplibregl) {
            this.pickerVisible = false;
            toast('Карта недоступна — укажите адрес текстом', { type: 'danger' });
            return;
          }
          try {
            const center = this.draft.coords || [131.8855, 43.1155];
            this.pickerMap = new window.maplibregl.Map({
              container: canvas,
              style: '/map/style.json',
              center,
              zoom: 12,
              dragRotate: false,
              pitchWithRotate: false,
              attributionControl: { compact: true },
            });
            this.pickerMap.on('click', (e) => this.setPickerCoords(e.lngLat.toArray()));
            this.pickerMap.on('load', () => {
              if (this.draft.coords) this.setPickerCoords(this.draft.coords);
            });
          } catch (error) {
            console.warn('Карта выбора точки недоступна', error);
            this.pickerVisible = false;
            toast('Карта недоступна — укажите адрес текстом', { type: 'danger' });
          }
        });
      },
      setPickerCoords(coords) {
        this.draft.coords = coords;
        if (!this.pickerMarker && this.pickerMap) {
          this.pickerMarker = new window.maplibregl.Marker().setLngLat(coords).addTo(this.pickerMap);
        } else {
          this.pickerMarker?.setLngLat(coords);
        }
      },
      destroyPicker() {
        this.pickerMap?.remove();
        this.pickerMap = null;
        this.pickerMarker = null;
      },
      async send() {
        if (this.draft.photos.some((p) => p.uploading)) {
          toast('Подождите, фото ещё загружаются', { type: 'danger' });
          return;
        }
        const ok = await requireAuth('Войдите, чтобы следить за статусом предложения в профиле.');
        if (!ok) return;
        if (!isLoggedIn()) return;

        this.sending = true;
        try {
          const submission = {
            type: this.draft.type,
            scenario: this.draft.type === 'event' ? '' : this.draft.scenario,
            category: this.draft.category,
            title: this.draft.title.trim(),
            description: this.draft.description.trim(),
            address: this.draft.address.trim(),
            coords: this.draft.coords ? [this.draft.coords[0], this.draft.coords[1]] : null,
            date: this.draft.date,
            time: this.draft.time,
            hours: this.draft.hours,
            price: this.draft.price,
            link: this.draft.link,
            fix_reasons: this.draft.fixReasons,
            photos: this.draft.photos.map(({ src, src_sm, width, height }) => ({ src, src_sm, width, height })),
          };
          await api('/submissions', { method: 'POST', body: submission });
          this.draft.photos.forEach((photo) => URL.revokeObjectURL(photo.url));
          storage.remove(ADD_DRAFT_KEY);
          location.href = `/add/success?type=${encodeURIComponent(this.draft.type)}`;
        } catch (error) {
          console.error('Не удалось отправить предложение', error);
          toast('Не получилось отправить. Проверьте соединение и попробуйте ещё раз', { type: 'danger' });
        } finally {
          this.sending = false;
        }
      },
      destroy() {
        this.destroyPicker();
      },
    }));
  });

  /* ── Навигация фрагментов (htmx-своп + pushState; popstate → перезагрузка) ── */

  function panelSwap(fragmentURL, pageURL, target = '#map-panel-inner') {
    htmx.ajax('GET', fragmentURL, { target, swap: 'innerHTML' });
    history.pushState({}, '', pageURL);
  }

  window.addEventListener('popstate', () => location.reload());

  function currentParams() {
    return new URLSearchParams(location.search);
  }

  /** Выбор места/события на карте: превью в панели. */
  function selectItem(id) {
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
  function selectPin(id) {
    const params = currentParams();
    params.set('pin', id);
    const qs = params.toString();
    panelSwap(`/fragments/pin/${encodeURIComponent(id)}`, `/now?${qs}`);
    window.placesMap?.select(id);
  }

  async function refreshPins() {
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

  function updateKindCounts(pins) {
    const counts = {};
    pins.forEach((pin) => {
      counts[pin.kind] = (counts[pin.kind] || 0) + 1;
    });
    $$('[data-kind-count]').forEach((el) => {
      const n = counts[el.dataset.kindCount] || 0;
      el.textContent = n ? String(n) : '';
    });
  }

  const mapItems = async (query) => {
    const data = await api(`/api/map-items?${query}`);
    if (Array.isArray(data)) return data;
    return data?.items ?? [];
  };

  async function refreshMapItems({ fit = false } = {}) {
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

  function nowCreateComponent() {
    const el = document.querySelector('[x-data="nowCreate"]');
    if (!el || !window.Alpine) return null;
    try {
      return window.Alpine.$data(el);
    } catch {
      return null;
    }
  }

  /* ── Диалог фильтров (каталог) ── */

  function filtersFromParams(params, scenario) {
    const value = {};
    for (const filter of FILTERS[scenario] || []) {
      if (filter.type === 'toggle') value[filter.id] = params.get(filter.id) === '1';
      else if (filter.type === 'single') value[filter.id] = params.get(filter.id) || '';
      else value[filter.id] = params.getAll(filter.id);
    }
    return value;
  }

  function filterFieldHtml(filter, value) {
    if (filter.type === 'toggle') {
      return `
        <label class="switch filter-field">
          <span>
            <span class="filter-field__label">${escapeHtml(filter.label)}</span>
            ${filter.hint ? `<span class="filter-field__hint">${escapeHtml(filter.hint)}</span>` : ''}
          </span>
          <input type="checkbox" name="${filter.id}" ${value ? 'checked' : ''} />
        </label>`;
    }
    const multi = filter.type === 'multi';
    const selected = multi ? value || [] : value || '';
    return `
      <fieldset class="fieldset filter-field">
        <legend class="filter-field__label">${escapeHtml(filter.label)}</legend>
        <div class="chip-row chip-row--wrap">
          ${filter.options
            .map((option) => {
              const checked = multi ? selected.includes(option.value) : selected === option.value;
              return `<label class="chip chip--lg">
                <input type="${multi ? 'checkbox' : 'radio'}" name="${filter.id}" value="${escapeHtml(option.value)}" ${checked ? 'checked' : ''} />
                ${escapeHtml(option.label)}
              </label>`;
            })
            .join('')}
        </div>
      </fieldset>`;
  }

  function readFiltersForm(form, scenario) {
    const result = {};
    for (const filter of FILTERS[scenario] || []) {
      if (filter.type === 'toggle') result[filter.id] = form.querySelector(`[name="${filter.id}"]`).checked;
      else if (filter.type === 'single') result[filter.id] = form.querySelector(`input[name="${filter.id}"]:checked`)?.value || '';
      else result[filter.id] = $$('input', form).filter((input) => input.name === filter.id && input.checked).map((input) => input.value);
    }
    return result;
  }

  function openFiltersDialog({ scenario }) {
    const filters = FILTERS[scenario] || [];
    if (!filters.length) return;
    const value = filtersFromParams(currentParams(), scenario);

    const toQuery = (filtersValue) => {
      const params = currentParams();
      FILTER_IDS.forEach((id) => params.delete(id));
      for (const [id, v] of Object.entries(filtersValue)) {
        if (typeof v === 'boolean') {
          if (v) params.set(id, '1');
        } else if (Array.isArray(v)) {
          v.forEach((item) => item && params.append(id, item));
        } else if (v) {
          params.set(id, v);
        }
      }
      return params;
    };

    const count = async (filtersValue) => {
      try {
        return (await mapItems(toQuery(filtersValue).toString())).length;
      } catch {
        return 0;
      }
    };

    openDialog({
      title: `Фильтры: ${(SCENARIO_LABELS[scenario] || scenario).toLowerCase()}`,
      body: `<form class="filters-form" id="filters-form">${filters.map((filter) => filterFieldHtml(filter, value[filter.id])).join('')}</form>`,
      footer: `<button type="button" class="btn btn--ghost" data-action="reset-filters">Сбросить</button>
        <button type="submit" form="filters-form" class="btn btn--primary" data-apply>Показать</button>`,
      onMount(el, close) {
        const form = $('#filters-form', el);
        const apply = $('[data-apply]', el);

        const refresh = debounce(async () => {
          const total = await count(readFiltersForm(form, scenario));
          apply.textContent = total ? `Показать ${countLabel(total, ['результат', 'результата', 'результатов'])}` : 'Ничего не найдено';
          apply.disabled = total === 0;
        }, 120);

        form.addEventListener('change', refresh);
        refresh();

        $('[data-action="reset-filters"]', el).addEventListener('click', () => {
          form.reset();
          refresh();
        });

        form.addEventListener('submit', (event) => {
          event.preventDefault();
          const params = toQuery(readFiltersForm(form, scenario));
          const qs = params.toString();
          panelSwap(`/fragments/catalog-results?${qs}`, `/catalog?${qs}`, '#catalog-results');
          close();
        });
      },
    });
  }

  /* ── Диалоги: поделиться, календарь, фото, жалоба, имя ── */

  async function shareLink({ title, text = '', url }) {
    const absolute = new URL(url, location.origin).href;
    if (navigator.share) {
      try {
        await navigator.share({ title, text, url: absolute });
        return;
      } catch (error) {
        if (error.name === 'AbortError') return;
        console.warn('Системное меню «Поделиться» недоступно, копируем ссылку', error);
      }
    }
    try {
      await navigator.clipboard.writeText(absolute);
      toast('Ссылка скопирована', { type: 'success' });
    } catch (error) {
      console.warn('Не удалось скопировать ссылку', error);
      toast(`Ссылка: ${absolute}`, { duration: 8000 });
    }
  }

  /* Экранирование текста по RFC 5545 и фолдинг строк (порт из detail.js макета). */
  const icsText = (value = '') => String(value).replace(/[\\,;]/g, (ch) => `\\${ch}`).replace(/\r?\n/g, '\\n');

  function icsFold(line) {
    if (new TextEncoder().encode(line).length <= 75) return line;
    const parts = [];
    let current = '';
    for (const ch of line) {
      if (new TextEncoder().encode(current + ch).length > (parts.length ? 74 : 75)) {
        parts.push(current);
        current = '';
      }
      current += ch;
    }
    parts.push(current);
    return parts.join('\r\n ');
  }

  function downloadIcs(dataset) {
    const stamp = (iso) => new Date(iso).toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '');
    const startsAt = dataset.icsStarts;
    if (!startsAt) return;
    const end = dataset.icsEnds || new Date(new Date(startsAt).getTime() + 2 * 3600000).toISOString();
    const ics = [
      'BEGIN:VCALENDAR',
      'VERSION:2.0',
      'PRODID:-//VL.ru//Mesta//RU',
      'BEGIN:VEVENT',
      `UID:${dataset.icsId}@places.vl.ru`,
      `DTSTAMP:${stamp(new Date().toISOString())}`,
      `DTSTART:${stamp(startsAt)}`,
      `DTEND:${stamp(end)}`,
      `SUMMARY:${icsText(dataset.icsTitle)}`,
      `LOCATION:${icsText(`${dataset.icsVenue || ''}, ${dataset.icsAddress || ''}`)}`,
      `DESCRIPTION:${icsText(dataset.icsShort || '')}`,
      'END:VEVENT',
      'END:VCALENDAR',
    ]
      .map(icsFold)
      .join('\r\n');

    const blob = new Blob([ics], { type: 'text/calendar;charset=utf-8' });
    const link = document.createElement('a');
    link.href = URL.createObjectURL(blob);
    link.download = `${dataset.icsId}.ics`;
    link.click();
    setTimeout(() => URL.revokeObjectURL(link.href), 1000);
    toast('Файл календаря скачан', { type: 'success' });
  }

  function openLightbox(photos, start, title) {
    let index = Math.max(0, start);
    const body = () => {
      const photo = photos[index];
      const credits = [
        photo.author ? `Фото: ${escapeHtml(photo.author)}` : '',
        photo.license
          ? `<a href="${escapeHtml(photo.license_url || '#')}" target="_blank" rel="noopener">${escapeHtml(photo.license)}</a>`
          : '',
        photo.source ? `<a href="${escapeHtml(photo.source)}" target="_blank" rel="noopener">источник</a>` : '',
      ]
        .filter(Boolean)
        .join(' · ');
      return `
        <figure class="lightbox">
          <img src="${escapeHtml(photo.src)}" alt="${escapeHtml(photo.alt || '')}" />
          <figcaption>
            <span>${escapeHtml(photo.alt || '')}</span>
            ${credits ? `<span class="detail__muted">${credits}</span>` : ''}
          </figcaption>
        </figure>`;
    };
    const footer = () =>
      photos.length > 1
        ? `<button type="button" class="btn btn--outline" data-step="-1">${icon('chevron-left')}Назад</button>
           <span class="lightbox__counter tabular">${index + 1} из ${photos.length}</span>
           <button type="button" class="btn btn--outline" data-step="1">Дальше${icon('chevron-right')}</button>`
        : '';

    const step = (delta) => {
      index = (index + delta + photos.length) % photos.length;
      dialog.setContent({ body: body(), footer: footer() });
    };

    const dialog = openDialog({
      title,
      body: body(),
      footer: footer(),
      wide: true,
      className: 'dialog--lightbox',
      onMount(el) {
        $(':scope .dialog__footer', el)?.addEventListener('click', (event) => {
          const button = event.target.closest('[data-step]');
          if (button) step(Number(button.dataset.step));
        });
        el.onkeydown = (event) => {
          if (event.key === 'ArrowRight') step(1);
          if (event.key === 'ArrowLeft') step(-1);
        };
      },
    });
  }

  function openRenameDialog() {
    const current =
      $('.profile__name')?.textContent?.trim() || $('.header__user-name')?.textContent?.trim() || '';
    openDialog({
      title: 'Как вас зовут?',
      body: `
        <form class="auth-form" id="rename-form" novalidate>
          <div class="field">
            <label class="field__label" for="rename-name">Имя</label>
            <input class="input" id="rename-name" name="name" type="text" maxlength="40" value="${escapeHtml(current)}" required />
            <p class="field__error" id="rename-name-error" role="alert"></p>
          </div>
        </form>`,
      footer: `<button type="button" class="btn btn--ghost" data-dialog-close>Отмена</button>
        <button type="submit" form="rename-form" class="btn btn--primary">Сохранить</button>`,
      onMount(el, close) {
        const input = $('#rename-name', el);
        setTimeout(() => {
          input.focus();
          input.select();
        }, 50);
        $('#rename-form', el).addEventListener('submit', async (event) => {
          event.preventDefault();
          const name = input.value.trim();
          const error = $('#rename-name-error', el);
          if (name.length < 2) {
            error.textContent = 'Имя — хотя бы пара букв';
            input.focus();
            return;
          }
          try {
            await api('/users/me', { method: 'PATCH', body: { display_name: name } });
            close();
            location.reload();
          } catch (err) {
            error.textContent = err.message || 'Не получилось сохранить';
          }
        });
      },
    });
  }

  function openReportDialog(pinId) {
    const reasons = ['Реклама или спам', 'Оскорбления', 'Опасная встреча', 'Личные данные', 'Другое'];
    openDialog({
      title: 'Пожаловаться на метку',
      body: `
        <form id="report-form" class="report-form">
          ${reasons
            .map((reason, i) => `<label class="check"><input type="radio" name="reason" value="${escapeHtml(reason)}" ${i === 0 ? 'checked' : ''} /><span>${escapeHtml(reason)}</span></label>`)
            .join('')}
        </form>`,
      footer: `<button type="button" class="btn btn--ghost" data-dialog-close>Отмена</button>
        <button type="submit" form="report-form" class="btn btn--primary">Отправить</button>`,
      onMount(el, close) {
        $('#report-form', el).addEventListener('submit', async (event) => {
          event.preventDefault();
          const reason = new FormData(event.target).get('reason');
          try {
            await api(`/pins/${encodeURIComponent(pinId)}/reports`, { method: 'POST', body: { reason } });
            close();
            toast('Спасибо. Модератор проверит метку', { type: 'success' });
          } catch (error) {
            toast(error.message || 'Не получилось отправить жалобу', { type: 'danger' });
          }
        });
      },
    });
  }

  /* ── Делегированные data-action ── */

  // Эти действия уже имеют инлайновые Alpine-обработчики на самих элементах.
  const INLINE_ACTIONS = ['toggle-panel', 'mobile-menu', 'hide-intro', 'toggle-picker', 'cancel-create'];

  document.addEventListener('click', (event) => {
    const el = event.target.closest('[data-action]');
    if (!el) return;
    const { action, id } = el.dataset;
    if (INLINE_ACTIONS.includes(action)) return;
    if (action === 'filters' && el.hasAttribute('hx-get')) return; // кнопка на карте открывает вкладку фильтров через htmx
    if (el.tagName === 'A' && el.hasAttribute('hx-get')) return;

    switch (action) {
      case 'login':
        event.preventDefault();
        openAuthDialog().then((user) => {
          if (user) location.reload();
        });
        break;
      case 'logout':
        event.preventDefault();
        api('/users/logout', { method: 'POST' })
          .catch(() => {})
          .finally(() => {
            storage.remove(CSRF_KEY);
            toast('Вы вышли');
            location.reload();
          });
        break;
      case 'rename':
        openRenameDialog();
        break;
      case 'share':
        event.preventDefault();
        shareLink({ title: el.dataset.shareTitle || document.title, text: el.dataset.shareText || '', url: el.dataset.shareUrl || location.href });
        break;
      case 'ics':
        downloadIcs(el.dataset);
        break;
      case 'photo': {
        const article = el.closest('article[data-photos]');
        if (!article) break;
        let photos;
        try {
          photos = JSON.parse(article.dataset.photos);
        } catch {
          photos = [];
        }
        if (!photos.length) break;
        const gallery = el.closest('.preview__gallery, .detail__gallery, .gallery');
        const imgs = gallery ? $$('img', gallery) : [];
        const img = el.querySelector('img') || el.closest('[data-photo-index]');
        let start = 0;
        if (img && imgs.length) start = imgs.indexOf(img);
        if (start < 0) start = 0;
        openLightbox(photos, start, $('.detail__title', article)?.textContent?.trim() || document.title);
        break;
      }
      case 'route-toggle':
        toggleRoute(id, el.dataset.title || '');
        if (window.Alpine) window.Alpine.store('route').ids = storage.get(ROUTE_KEY, []);
        break;
      case 'route-clear': {
        clearRoute();
        if (window.Alpine) window.Alpine.store('route').ids = [];
        const params = currentParams();
        if (params.get('panel') === 'route') {
          params.delete('panel');
          const qs = params.toString();
          panelSwap(`/fragments/map-panel?${qs}`, `/?${qs}`);
        }
        break;
      }
      case 'filters':
        openFiltersDialog({ scenario: el.dataset.scenario || currentParams().get('s') || 'city' });
        break;
      case 'select': {
        const stage = document.getElementById('map-stage');
        const mode = stage?.dataset.stageMode || 'items';
        if (mode === 'pins') selectPin(id);
        else selectItem(id);
        break;
      }
      case 'pin-going':
        requireAuth('Войдите, чтобы отметиться «Я приду» — автор увидит, сколько человек собирается.').then(async (ok) => {
          if (!ok) return;
          const joined = el.getAttribute('aria-pressed') === 'true';
          try {
            await api(`/pins/${encodeURIComponent(id)}/going`, { method: joined ? 'DELETE' : 'PUT' });
            toast(joined ? 'Отметку сняли' : 'Отметили: вы идёте. Автор увидит это');
            await refreshPins();
            htmx.ajax('GET', `/fragments/pin/${encodeURIComponent(id)}`, { target: '#map-panel-inner' });
          } catch (error) {
            toast(error.message || 'Не получилось отметиться', { type: 'danger' });
          }
        });
        break;
      case 'delete-pin':
        confirmDialog({ title: 'Снять метку?', text: 'Она сразу исчезнет с карты у всех.', confirmLabel: 'Снять', danger: true }).then(async (ok) => {
          if (!ok) return;
          try {
            await api(`/pins/${encodeURIComponent(id)}`, { method: 'DELETE' });
            toast('Метка снята');
            const params = currentParams();
            params.delete('pin');
            const qs = params.toString();
            panelSwap(`/fragments/pins${qs ? `?${qs}` : ''}`, `/now${qs ? `?${qs}` : ''}`);
            await refreshPins();
          } catch (error) {
            toast(error.message || 'Не получилось снять метку', { type: 'danger' });
          }
        });
        break;
      case 'report':
        openReportDialog(id);
        break;
      case 'create':
        nowCreateComponent()?.start();
        break;
      case 'zoom-in':
        window.placesMap?.zoomIn();
        break;
      case 'zoom-out':
        window.placesMap?.zoomOut();
        break;
      case 'locate': {
        const fallback = () => {
          window.placesMap?.showMe([131.8855, 43.1155]);
          toast('Геолокация недоступна — показываем центр Владивостока');
        };
        if (!navigator.geolocation) {
          fallback();
          break;
        }
        navigator.geolocation.getCurrentPosition(
          ({ coords }) => {
            const point = [coords.longitude, coords.latitude];
            const inBounds = point[0] > 130.5 && point[0] < 139 && point[1] > 42 && point[1] < 49;
            if (inBounds) window.placesMap?.showMe(point);
            else {
              window.placesMap?.showMe([131.8855, 43.1155]);
              toast('Вы далеко от Приморья — показываем центр Владивостока');
            }
          },
          fallback,
          { timeout: 6000, maximumAge: 60000 },
        );
        break;
      }
      case 'retry-map':
        window.__remountMap?.();
        break;
      case 'clear-search': {
        event.preventDefault();
        const params = currentParams();
        params.delete('q');
        const qs = params.toString();
        panelSwap(`/fragments/map-panel?${qs}`, `/?${qs}`);
        break;
      }
      default:
    }
  });

  /* ── Поиск и фильтры: делегированные обработчики (URL меняет JS, свопает htmx) ── */

  const searchDebounces = new WeakMap();

  document.addEventListener('input', (event) => {
    const form = event.target.closest('form[data-role="map-search"], form[data-role="catalog-search"]');
    if (!form) return;
    let entry = searchDebounces.get(form);
    if (!entry) {
      entry = { timer: null, value: form.querySelector('input[name="q"]')?.value || '' };
      searchDebounces.set(form, entry);
    }
    clearTimeout(entry.timer);
    entry.timer = setTimeout(() => {
      const input = form.querySelector('input[name="q"]');
      const value = (input?.value || '').trim();
      if (value === entry.value.trim()) return;
      entry.value = value;
      const params = currentParams();
      if (value) params.set('q', value);
      else params.delete('q');
      const qs = params.toString();
      const isMap = form.dataset.role === 'map-search';
      window.__focusSearch = true;
      panelSwap(
        `${isMap ? '/fragments/map-panel' : '/fragments/catalog-results'}?${qs}`,
        `${isMap ? '/' : '/catalog'}?${qs}`,
        isMap ? '#map-panel-inner' : '#catalog-results',
      );
    }, 250);
  });

  document.addEventListener('submit', (event) => {
    const form = event.target.closest('form[data-role="map-search"], form[data-role="catalog-search"]');
    if (form) event.preventDefault();
  });

  document.addEventListener('change', (event) => {
    const sort = event.target.closest('[data-role="catalog-sort"]');
    if (sort) {
      const params = currentParams();
      if (sort.value) params.set('sort', sort.value);
      else params.delete('sort');
      const qs = params.toString();
      panelSwap(`/fragments/catalog-results?${qs}`, `/catalog?${qs}`, '#catalog-results');
      return;
    }

    const filtersForm = event.target.closest('form[data-role="map-filters"]');
    if (!filtersForm) return;
    const scenario = filtersForm.dataset.scenario;
    const params = currentParams();
    FILTER_IDS.forEach((key) => params.delete(key));
    const data = new FormData(filtersForm);
    for (const [key, value] of data.entries()) {
      if (value === '' || value === null) continue;
      // single-радио с пустым значением означает «любое» — пропускаем
      if (!params.getAll(key).includes(value)) params.append(key, value);
    }
    // чекбоксы-тумблеры приходят как "on"
    for (const filter of FILTERS[scenario] || []) {
      if (filter.type === 'toggle') {
        const on = data.get(filter.id) === 'on';
        params.delete(filter.id);
        if (on) params.set(filter.id, '1');
      }
    }
    const qs = params.toString();
    panelSwap(`/fragments/map-panel?${qs}`, `/?${qs}`);
  });

  /* ── После htmx-свопа ── */

  function focusSearchIfNeeded() {
    if (!window.__focusSearch) return;
    window.__focusSearch = false;
    const input = document.querySelector('#map-panel-inner input[name="q"], #catalog-results input[name="q"]');
    if (input) {
      input.focus();
      const end = input.value.length;
      input.setSelectionRange(end, end);
    }
  }

  document.body.addEventListener('htmx:afterSwap', (event) => {
    const target = event.detail.target;
    if (!(target instanceof HTMLElement)) return;

    if (target.id === 'map-panel-inner') {
      focusSearchIfNeeded();
      layoutSlots();

      let url = event.detail.xhr?.responseURL || location.href;
      let params;
      try {
        params = new URL(url, location.origin).searchParams;
      } catch {
        params = currentParams();
      }

      if (params.get('panel') === 'route') {
        routeState.notify(); // скрыть плашку маршрута, пока открыт редактор
      } else if (params.get('place')) {
        window.placesMap?.select(params.get('place'));
      } else {
        const stage = document.getElementById('map-stage');
        if (stage?.dataset.stageMode === 'pins') {
          refreshPins();
        } else {
          const hasQuery = Boolean(params.get('q'));
          const hasFilters = FILTER_IDS.some((key) => params.get(key));
          refreshMapItems({ fit: hasQuery || hasFilters });
        }
      }

      // свежие счётчики по типам меток
      if (document.getElementById('map-stage')?.dataset.stageMode === 'pins') {
        api('/pins').then(updateKindCounts).catch(() => {});
      }
      routeState.notify();
      return;
    }

    if (target.id === 'catalog-results') {
      focusSearchIfNeeded();
    }
  });

  /* ── Сцена карты: создание, шторка, слоты, отступы ── */

  const CITY_CENTER = [131.8855, 43.1155];

  function layoutSlots() {
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
  function updatePadding() {
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

  /** Нижняя шторка панели на мобильном: peek → half → full (порт ui/sheet.js). */
  function bindSheet(panel) {
    const handle = panel.querySelector('.map-panel__handle');
    if (!handle) return { set() {}, get: () => 'peek' };
    const STATES = ['peek', 'half', 'full'];
    const LABELS = { peek: 'Развернуть панель', half: 'Развернуть панель на весь экран', full: 'Свернуть панель' };
    let current = panel.dataset.sheet || 'peek';
    let drag = null;
    let suppressClick = false;

    const offsets = () => {
      const height = panel.getBoundingClientRect().height;
      const peek = parseFloat(getComputedStyle(panel.parentElement).getPropertyValue('--sheet-peek')) * 16 || 140;
      return { peek: height - peek, half: height * 0.45, full: 0 };
    };

    const set = (state) => {
      current = state;
      panel.dataset.sheet = state;
      panel.style.removeProperty('--sheet-offset');
      handle.setAttribute('aria-expanded', String(state !== 'peek'));
      handle.setAttribute('aria-label', LABELS[state]);
      setTimeout(updatePadding, 340);
    };

    const start = (event) => {
      if (!isMobileLayout() || event.button > 0) return;
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
      const sorted = STATES.map((state) => ({ state, distance: Math.abs(o[state] - position) })).sort((a, b) => a.distance - b.distance);
      let target = sorted[0].state;
      if (Math.abs(dy) > 40 && target === current) {
        const index = STATES.indexOf(current) + (dy < 0 ? 1 : -1);
        target = STATES[Math.max(0, Math.min(STATES.length - 1, index))];
      }
      set(target);
    };

    handle.addEventListener('pointerdown', start);
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', end);
    window.addEventListener('pointercancel', end);

    handle.addEventListener('click', () => {
      if (suppressClick) return;
      const index = STATES.indexOf(current);
      set(current === 'full' ? 'peek' : STATES[index + 1]);
    });

    set(current);
    return { set, get: () => current };
  }

  function initStage() {
    const stage = document.getElementById('map-stage');
    if (!stage) return;
    const canvas = stage.querySelector('#map');
    const error = stage.querySelector('[data-error]');
    const mode = stage.dataset.stageMode || 'items';

    const showError = (reason) => {
      error.hidden = false;
      error.innerHTML = `
        <div class="empty">
          <span class="empty__icon">${icon('wifi-off', 'icon--lg')}</span>
          <h2 class="empty__title">Карта не загрузилась</h2>
          <p class="empty__text">${reason === 'timeout' ? 'Сервер карты отвечает слишком долго.' : 'Похоже, пропало соединение.'} Список мест работает — откройте его или попробуйте ещё раз.</p>
          <div class="empty__actions">
            <button type="button" class="btn btn--primary" data-action="retry-map">${icon('refresh-cw')}Повторить</button>
          </div>
        </div>`;
    };

    const syncMap = async () => {
      if (!window.placesMap) return;
      window.placesMap.setMode(mode);
      if (mode === 'pins') await refreshPins();
      else await refreshMapItems({ fit: false });
      paintRouteOnMap();
    };

    const mount = () => {
      try {
        const controller = window.createPlacesMap(canvas, { onError: showError });
        window.placesMap = controller;
        const readyCheck = setInterval(() => {
          if (controller.isReady()) {
            clearInterval(readyCheck);
            error.hidden = true;
            updatePadding();
          }
        }, 300);
        setTimeout(() => clearInterval(readyCheck), 15000);
        syncMap();
      } catch (err) {
        console.error('Не удалось создать карту', err);
        showError('crash');
      }
    };

    mount();
    window.__remountMap = () => {
      try {
        window.placesMap?.destroy?.();
      } catch {
        /* уже уничтожена */
      }
      error.hidden = true;
      mount();
    };

    // Выбор маркера: превью в панели (если не идёт выбор точки для метки).
    stage.addEventListener('places:select', (event) => {
      const now = nowCreateComponent();
      if (now && now.step !== 'list') return;
      if (mode === 'pins') selectPin(event.detail.id);
      else selectItem(event.detail.id);
    });

    // Наведение на маркер подсвечивает карточку в списке и наоборот.
    stage.addEventListener('places:hover', (event) => {
      $$('.pin-card[data-id], .result-row[data-id]').forEach((card) =>
        card.classList.toggle('is-hovered', card.dataset.id === event.detail.id),
      );
    });
    document.addEventListener('mouseover', (event) => {
      const card = event.target.closest('.pin-card[data-id], .result-row[data-id]');
      if (card) window.placesMap?.hover(card.dataset.id);
    });

    bindSheet(stage.querySelector('#map-panel'));
    layoutSlots();

    const mobileQuery = window.matchMedia('(max-width: 47.99rem)');
    mobileQuery.addEventListener('change', () => {
      layoutSlots();
      window.placesMap?.resize();
    });

    new ResizeObserver(() => {
      if (!stage.hidden) window.placesMap?.resize();
    }).observe(stage);

    const top = stage.querySelector('[data-top]');
    if (top) {
      new ResizeObserver(() => {
        if (isMobileLayout()) stage.style.setProperty('--sheet-top', `${Math.ceil(top.getBoundingClientRect().height) + 20}px`);
      }).observe(top);
    }

    window.addEventListener(
      'resize',
      debounce(() => {
        layoutSlots();
        window.placesMap?.resize();
      }, 150),
    );

    // Escape закрывает превью метки.
    document.addEventListener('keydown', (event) => {
      if (event.key !== 'Escape') return;
      if (document.querySelector('dialog[open]')) return;
      if (mode !== 'pins') return;
      const pin = currentParams().get('pin');
      if (!pin) return;
      const params = currentParams();
      params.delete('pin');
      const qs = params.toString();
      panelSwap(`/fragments/pins${qs ? `?${qs}` : ''}`, `/now${qs ? `?${qs}` : ''}`);
    });

    // Автообновление меток раз в минуту (только режим «Сейчас»).
    setInterval(() => {
      if (mode !== 'pins') return;
      if (document.hidden || document.querySelector('dialog[open]')) return;
      const now = nowCreateComponent();
      if (now && now.step !== 'list') return;
      refreshPins();
      const pin = currentParams().get('pin');
      if (pin) htmx.ajax('GET', `/fragments/pin/${encodeURIComponent(pin)}`, { target: '#map-panel-inner' });
      else htmx.ajax('GET', '/fragments/pins', { target: '#map-panel-inner' });
    }, 60000);
  }

  /* ── Мини-карта на странице места ── */

  function initMiniMaps() {
    $$('[data-mini-map]').forEach((node) => {
      if (!window.maplibregl) return;
      const lat = Number(node.dataset.lat);
      const lon = Number(node.dataset.lon);
      if (!lat || !lon) return;
      try {
        const scenario = node.dataset.scenario || 'city';
        const map = new window.maplibregl.Map({
          container: node,
          style: '/map/style.json',
          center: [lon, lat],
          zoom: scenario === 'city' ? 14 : 10.5,
          interactive: false,
          attributionControl: { compact: true },
        });
        const el = document.createElement('div');
        el.className = 'marker marker--default';
        el.setAttribute('aria-hidden', 'true');
        new window.maplibregl.Marker({ element: el, anchor: 'bottom' }).setLngLat([lon, lat]).addTo(map);
      } catch (error) {
        console.warn('Мини-карта недоступна', error);
        node.hidden = true;
      }
    });
  }

  /* ── Старт ── */

  document.addEventListener('DOMContentLoaded', () => {
    initStage();
    initMiniMaps();

    // Правила: ?section=moderation прокручивает к разделу модерации.
    if (location.pathname === '/rules') {
      const section = currentParams().get('section');
      if (section) {
        const target = document.getElementById(section);
        if (target) setTimeout(() => target.scrollIntoView({ behavior: 'smooth', block: 'start' }), 100);
      }
    }
  });
})();
