/**
 * «Сцена» карты — общий каркас для экранов #/map и #/now.
 *
 * Создаётся один раз и живёт всё время работы приложения: при уходе на другие страницы
 * скрывается, а при возврате карта остаётся в том же положении. Содержимое панели
 * (фильтры, список, превью, маршрут, метки) отдают панели explore.js и now.js.
 *
 * Мобильная раскладка: элементы с data-top-slot переносятся из шапки панели в верхнюю
 * плавающую зону .map-top, а панель становится нижней шторкой (ui/sheet.js).
 */

import { html, render, $, $$, on, isMobileLayout } from '../core/dom.js';
import { icon } from '../ui/icon.js';
import { createMap } from '../map/map.js';
import { bindSheet } from '../ui/sheet.js';
import { toast } from '../ui/toast.js';
import { CITY_CENTER, MAX_BOUNDS } from '../core/geo.js';

let instance = null;

export function getStage() {
  if (!instance) instance = createStage(document.getElementById('map-stage'));
  return instance;
}

function createStage(root) {
  render(
    root,
    html`
      <div class="map-top" data-top></div>
      <button type="button" class="icon-btn icon-btn--floating panel-toggle" data-action="toggle-panel" aria-expanded="true" aria-controls="map-panel" aria-label="Скрыть панель" title="Скрыть панель">
        ${icon('panel-left-close')}
      </button>
      <div data-overlay></div>
      <aside class="map-panel" id="map-panel" data-sheet="peek" aria-label="Фильтры и список мест на карте">
        <button type="button" class="map-panel__handle" aria-controls="map-panel" aria-expanded="false"></button>
        <div class="map-panel__head" data-head></div>
        <div class="map-panel__body" id="map-panel-content" data-body></div>
        <div class="map-panel__foot" data-foot></div>
      </aside>
      <div class="map-controls">
        <div class="map-controls__group map-controls__zoom">
          <button type="button" class="icon-btn" data-action="zoom-in" aria-label="Приблизить" title="Приблизить">${icon('plus')}</button>
          <button type="button" class="icon-btn" data-action="zoom-out" aria-label="Отдалить" title="Отдалить">${icon('minus')}</button>
        </div>
        <div class="map-controls__group">
          <button type="button" class="icon-btn" data-action="locate" aria-label="Где я" title="Где я">${icon('locate-fixed')}</button>
        </div>
      </div>
      <div class="route-bar" data-route-bar hidden></div>
      <div class="map-picker" data-picker hidden>
        <div class="map-picker__pin">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" stroke="#fff" stroke-width="1.5" d="M12 22s7-6.2 7-12a7 7 0 1 0-14 0c0 5.8 7 12 7 12Z"/><circle cx="12" cy="10" r="2.6" fill="#fff"/></svg>
          <span class="map-picker__shadow"></span>
        </div>
      </div>
      <div class="map-canvas" id="map"></div>
      <div class="map-error" data-error hidden></div>
    `,
  );

  const els = {
    root,
    canvas: $('#map', root),
    top: $('[data-top]', root),
    overlay: $('[data-overlay]', root),
    panel: $('#map-panel', root),
    handle: $('.map-panel__handle', root),
    head: $('[data-head]', root),
    body: $('[data-body]', root),
    foot: $('[data-foot]', root),
    routeBar: $('[data-route-bar]', root),
    picker: $('[data-picker]', root),
    error: $('[data-error]', root),
    toggle: $('[data-action="toggle-panel"]', root),
  };

  const handlers = { select: null, hover: null, ready: null, sheet: null };
  /** Последние данные, переданные карте, — чтобы восстановить их после пересоздания карты. */
  const memory = { mode: 'items', items: [], itemsOptions: {}, pins: [], route: null, selected: null };
  let controller = null;

  function mountMap() {
    controller = createMap(els.canvas, {
      onSelect: (id, type) => handlers.select?.(id, type),
      onHover: (id) => handlers.hover?.(id),
      onError: showError,
      onReady: () => {
        els.error.hidden = true;
        updatePadding();
        handlers.ready?.();
      },
    });
    controller.setMode(memory.mode);
    controller.setItems(memory.items, memory.itemsOptions);
    controller.setPins(memory.pins);
    if (memory.route) controller.setRoute(memory.route);
    controller.select(memory.selected);
  }

  function showError(reason) {
    render(
      els.error,
      html`
        <div class="empty">
          <span class="empty__icon">${icon('wifi-off', 'icon--lg')}</span>
          <h2 class="empty__title">Карта не загрузилась</h2>
          <p class="empty__text">
            ${reason === 'timeout' ? 'Сервер карты отвечает слишком долго.' : 'Похоже, пропало соединение.'} Список мест работает — откройте его или попробуйте ещё раз.
          </p>
          <div class="empty__actions">
            <button type="button" class="btn btn--primary" data-action="retry-map">${icon('refresh-cw')}Повторить</button>
            <a class="btn btn--outline" href="#/catalog">Открыть справочник</a>
          </div>
        </div>
      `,
    );
    els.error.hidden = false;
  }

  on(els.error, 'click', '[data-action="retry-map"]', () => {
    els.error.hidden = true;
    controller.destroy();
    mountMap();
  });

  /**
   * Прокси к контроллеру карты, запоминающий состояние. Пока карта не создана (сцена ещё не
   * показывалась), вызовы только обновляют memory — mountMap применит его при создании.
   */
  const map = {
    setMode(mode) {
      memory.mode = mode;
      controller?.setMode(mode);
    },
    setItems(items, options = {}) {
      memory.items = items;
      memory.itemsOptions = options;
      controller?.setItems(items, options);
    },
    setPins(pins) {
      memory.pins = pins;
      controller?.setPins(pins);
    },
    setRoute(route) {
      memory.route = route;
      controller?.setRoute(route || {});
    },
    select(id) {
      memory.selected = id;
      controller?.select(id);
    },
    hover: (id) => controller?.hover(id),
    flyTo: (coords, zoom) => controller?.flyTo(coords, zoom),
    fitTo: (list, options) => controller?.fitTo(list, options),
    resetView: (key) => controller?.resetView(key),
    getCenter: () => controller?.getCenter() ?? CITY_CENTER,
    getZoom: () => controller?.getZoom() ?? 12,
    showMe: (coords) => controller?.showMe(coords),
    isReady: () => Boolean(controller?.isReady()),
  };

  /** Доступ к карте из консоли и автотестов: window.__placesMap.raw — экземпляр MapLibre. */
  Object.defineProperty(window, '__placesMap', { get: () => controller, configurable: true });

  /* ── Раскладка ── */

  /**
   * Отступы карты под панели. Центр карты (и маркер выбора точки) — в середине свободной области,
   * поэтому map.getCenter() при создании метки совпадает с тем, что видит пользователь.
   */
  function updatePadding() {
    if (!controller?.isReady()) return;
    let padding;
    if (isMobileLayout()) {
      const topHeight = els.top.getBoundingClientRect().height;
      const panelTop = els.panel.getBoundingClientRect().top - root.getBoundingClientRect().top;
      const bottom = Math.max(120, root.clientHeight - panelTop);
      padding = { top: topHeight + 24, bottom: Math.min(bottom, root.clientHeight * 0.6) + 16, left: 16, right: 16 };
    } else {
      const collapsed = root.classList.contains('is-panel-collapsed');
      padding = { top: 24, bottom: 24, right: 72, left: collapsed ? 24 : els.panel.offsetWidth + 40 };
    }
    controller.setPadding(padding);
    els.picker.style.inset = `${padding.top}px ${padding.right}px ${padding.bottom}px ${padding.left}px`;
  }

  /** Перенос элементов data-top-slot между шапкой панели (десктоп) и плавающей зоной (мобильный). */
  function layoutSlots() {
    const mobile = isMobileLayout();
    $$('[data-top-slot]', root).forEach((node) => {
      const home = $(`[data-slot-home="${node.dataset.topSlot}"]`, els.head);
      if (mobile && node.parentElement !== els.top) els.top.append(node);
      if (!mobile && home && node.parentElement !== home) home.append(node);
    });
    // Сортировка по порядку в шапке
    if (mobile) {
      const order = $$('[data-slot-home]', els.head).map((home) => home.dataset.slotHome);
      $$('[data-top-slot]', els.top)
        .sort((a, b) => order.indexOf(a.dataset.topSlot) - order.indexOf(b.dataset.topSlot))
        .forEach((node) => els.top.append(node));
    }
    root.style.setProperty('--sheet-top', `${Math.ceil(els.top.getBoundingClientRect().height) + 20}px`);
    requestAnimationFrame(updatePadding);
  }

  const mobileQuery = window.matchMedia('(max-width: 47.99rem)');
  mobileQuery.addEventListener('change', () => {
    layoutSlots();
    controller?.resize();
  });

  new ResizeObserver(() => {
    if (!root.hidden) controller?.resize();
  }).observe(root);

  // Высота верхней зоны меняется при перерисовке чипов — шторка в положении full не должна её перекрывать
  new ResizeObserver(() => {
    if (isMobileLayout()) root.style.setProperty('--sheet-top', `${Math.ceil(els.top.getBoundingClientRect().height) + 20}px`);
  }).observe(els.top);

  const sheet = bindSheet(els.panel, {
    handle: els.handle,
    dragZones: [els.head],
    onChange: (state) => {
      setTimeout(updatePadding, 340);
      handlers.sheet?.(state);
    },
  });

  /* ── Контролы ── */

  on(root, 'click', '[data-action="zoom-in"]', () => controller?.zoomIn());
  on(root, 'click', '[data-action="zoom-out"]', () => controller?.zoomOut());
  on(root, 'click', '[data-action="toggle-panel"]', () => {
    const collapsed = root.classList.toggle('is-panel-collapsed');
    // Скрытая панель не должна оставаться в порядке табуляции и дереве доступности
    els.panel.inert = collapsed;
    els.toggle.setAttribute('aria-expanded', String(!collapsed));
    els.toggle.setAttribute('aria-label', collapsed ? 'Показать панель' : 'Скрыть панель');
    els.toggle.title = collapsed ? 'Показать панель' : 'Скрыть панель';
    render(els.toggle, icon(collapsed ? 'panel-left-open' : 'panel-left-close'));
    setTimeout(updatePadding, 340);
  });

  on(root, 'click', '[data-action="locate"]', () => {
    const fallback = () => {
      map.showMe(CITY_CENTER);
      toast('Геолокация недоступна — показываем центр Владивостока');
    };
    if (!navigator.geolocation) return fallback();
    navigator.geolocation.getCurrentPosition(
      ({ coords }) => {
        const point = [coords.longitude, coords.latitude];
        const [[w, s], [e, n]] = MAX_BOUNDS;
        if (point[0] < w || point[0] > e || point[1] < s || point[1] > n) {
          map.showMe(CITY_CENTER);
          toast('Вы далеко от Приморья — показываем центр Владивостока');
        } else {
          map.showMe(point);
        }
      },
      fallback,
      { timeout: 6000, maximumAge: 60000 },
    );
  });

  return {
    root,
    els,
    map,
    sheet,
    show() {
      root.hidden = false;
      // Карта создаётся только в видимом контейнере: иначе MapLibre запомнит нулевой размер
      if (!controller) mountMap();
      requestAnimationFrame(() => {
        controller.resize();
        layoutSlots();
      });
    },
    hide() {
      root.hidden = true;
    },
    /** Панель регистрирует обработчики событий карты. */
    bind(next) {
      Object.assign(handlers, { select: null, hover: null, ready: null, sheet: null }, next);
    },
    layoutSlots,
    updatePadding,
    setSheet: (state) => sheet.set(state),
    getSheet: () => sheet.get(),
    showPicker(visible) {
      els.picker.hidden = !visible;
    },
  };
}
