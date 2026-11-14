/* Сцена карты: создание контроллера, нижняя шторка, слоты, отступы,
 * автоподгрузка; мини-карта на странице места. */
import { $$, debounce, icon, currentParams, isMobileLayout } from './util.js';
import { panelSwap, refreshMapItems, refreshPins, selectItem, selectPin } from './nav.js';
import { nowCreateComponent } from './pins.js';
import { paintRouteOnMap } from './route.js';
import { layoutSlots, updatePadding } from './layout.js';
import { createMiniMap, createPlacesMap, ensureMapVendor } from './map.js';

/* Глобальные обработчики сцены вешаются один раз за жизнь страницы:
 * при hx-boost-переходах body подменяется без перезагрузки, поэтому
 * состояние читаем из живого DOM, а не из замыкания конкретной сцены. */
let stageGlobalsBound = false;

function bindStageGlobals() {
  if (stageGlobalsBound) return;
  stageGlobalsBound = true;

  // Наведение на карточку подсвечивает маркер на карте.
  document.addEventListener('mouseover', (event) => {
    const card = event.target.closest('.pin-card[data-id], .result-row[data-id]');
    if (card) window.placesMap?.hover(card.dataset.id);
  });

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
    const stage = document.getElementById('map-stage');
    if (!stage || stage.dataset.stageMode !== 'pins') return;
    const pin = currentParams().get('pin');
    if (!pin) return;
    const params = currentParams();
    params.delete('pin');
    const qs = params.toString();
    panelSwap(`/fragments/pins${qs ? `?${qs}` : ''}`, `/now${qs ? `?${qs}` : ''}`);
  });

  // Автообновление меток раз в минуту (только режим «Сейчас»).
  setInterval(() => {
    const stage = document.getElementById('map-stage');
    if (!stage || stage.dataset.stageMode !== 'pins') return;
    if (document.hidden || document.querySelector('dialog[open]')) return;
    const now = nowCreateComponent();
    if (now && now.step !== 'list') return;
    refreshPins();
    const pin = currentParams().get('pin');
    if (pin) htmx.ajax('GET', `/fragments/pin/${encodeURIComponent(pin)}`, { target: '#map-panel-inner' });
    else htmx.ajax('GET', '/fragments/pins', { target: '#map-panel-inner' });
  }, 60000);
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

export function initStage() {
  const stage = document.getElementById('map-stage');
  if (!stage) return;
  const canvas = stage.querySelector('#map');
  const error = stage.querySelector('[data-error]');
  const mode = stage.dataset.stageMode || 'items';
  const scenario = stage.dataset.scenario || (mode === 'pins' ? 'now' : 'city');

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
      const controller = createPlacesMap(canvas, { onError: showError, scenario });
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

  // Вендор карты при буст-переходе с другой страницы может быть ещё не загружен
  // (head при свопе не перечитывается) — догружаем перед созданием сцены.
  ensureMapVendor().then(mount, () => showError('crash'));
  window.__remountMap = () => {
    try {
      window.placesMap?.destroy?.();
    } catch {
      /* уже уничтожена */
    }
    error.hidden = true;
    mount();
  };

  bindStageGlobals();

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
}

/* ── Мини-карта на странице места ── */

export function initMiniMaps() {
  $$('[data-mini-map]').forEach((node) => {
    if (node.dataset.minimapInit || node.dataset.minimapPending) return;
    if (!window.maplibregl) {
      node.dataset.minimapPending = '1';
      ensureMapVendor()
        .then(() => {
          delete node.dataset.minimapPending;
          initMiniMaps();
        })
        .catch(() => {
          delete node.dataset.minimapPending;
          node.hidden = true;
        });
      return;
    }
    node.dataset.minimapInit = '1';
    const lat = Number(node.dataset.lat);
    const lon = Number(node.dataset.lon);
    if (!lat || !lon) return;
    const scenario = node.dataset.scenario || 'city';
    createMiniMap(node, { center: [lon, lat], zoom: scenario === 'city' ? 14 : 10.5, scenario }).catch((error) => {
      console.warn('Мини-карта недоступна', error);
      node.hidden = true;
    });
  });
}
