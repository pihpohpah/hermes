/**
 * Маршрут между точками: плавающая плашка над картой и редактор в панели (#/map?panel=route).
 * Состав маршрута хранится в store.route, режим — в store.routeMode.
 */

import { html, render, on } from '../core/dom.js';
import { store } from '../core/store.js';
import { icon } from '../ui/icon.js';
import { getItem } from '../api/places.js';
import { buildRoute, ROUTE_MODES } from '../map/routing.js';
import { formatDistance, formatDuration, countLabel } from '../core/format.js';
import { moveStop, clearRoute } from '../ui/route-store.js';
import { setQuery } from '../core/router.js';
import { itemUrl } from '../ui/cards.js';

export function createRouteController(stage) {
  let stops = [];
  let result = null;
  let computing = false;
  let visible = false;
  let editorTarget = null;
  let version = 0;

  async function recompute() {
    const current = ++version;
    const { route, routeMode } = store.get();
    computing = route.length >= 2;
    paint();
    const loaded = await Promise.all(route.map((id) => getItem(id).catch(() => null)));
    if (current !== version) return;
    stops = loaded.filter(Boolean);
    if (stops.length !== route.length) store.set({ route: stops.map((s) => s.id) });

    result = stops.length >= 2 ? await buildRoute(stops, routeMode) : null;
    if (current !== version) return;
    computing = false;

    if (visible) stage.map.setRoute({ stops, geometry: result?.geometry || null, approximate: result?.approximate || false });
    paint();
  }

  function modeSwitch() {
    const { routeMode } = store.get();
    return html`
      <div class="segmented" role="group" aria-label="Способ передвижения">
        ${ROUTE_MODES.map(
          (mode) => html`
            <button type="button" class="segmented__item" aria-pressed="${routeMode === mode.id}" data-action="route-mode" data-mode="${mode.id}">
              ${icon(mode.icon)}${mode.label}
            </button>
          `,
        )}
      </div>
    `;
  }

  function summaryText() {
    if (stops.length < 2) return 'Добавьте ещё одну точку';
    if (computing || !result) return 'Считаем маршрут…';
    const mode = store.get().routeMode === 'car' ? 'на машине' : 'пешком';
    return `${formatDistance(result.km)} · ${formatDuration(result.minutes)} ${mode}${result.approximate ? ', примерно' : ''}`;
  }

  function paintBar() {
    const bar = stage.els.routeBar;
    const show = visible && stops.length > 0 && !editorTarget;
    bar.hidden = !show;
    stage.els.root.classList.toggle('has-route', show);
    if (!show) return;
    render(
      bar,
      html`
        <div class="route-bar__card" role="region" aria-label="Маршрут">
          <span class="route-bar__icon">${icon('route')}</span>
          <div class="route-bar__info">
            <strong>Маршрут · ${countLabel(stops.length, ['точка', 'точки', 'точек'])}</strong>
            <span>${summaryText()}</span>
          </div>
          ${stops.length >= 2 ? modeSwitch() : ''}
          <div class="route-bar__actions">
            <button type="button" class="btn btn--sm btn--outline" data-action="route-open">${icon('pencil', 'icon--sm')}Изменить</button>
            <button type="button" class="icon-btn icon-btn--sm" data-action="route-clear" aria-label="Очистить маршрут" title="Очистить маршрут">${icon('x')}</button>
          </div>
        </div>
      `,
    );
  }

  function yandexUrl() {
    const points = stops.map((s) => `${s.coords[1]},${s.coords[0]}`).join('~');
    return `https://yandex.ru/maps/?rtext=${points}&rtt=${store.get().routeMode === 'car' ? 'auto' : 'pd'}`;
  }

  function paintEditor() {
    if (!editorTarget) return;
    if (!stops.length) {
      render(
        editorTarget,
        html`
          <div class="preview">
            <div class="preview__bar">
              <button type="button" class="btn btn--sm btn--ghost" data-action="route-close">${icon('arrow-left', 'icon--sm')}К списку</button>
            </div>
            <div class="empty">
              <span class="empty__icon">${icon('route', 'icon--lg')}</span>
              <h2 class="empty__title">Маршрут пока пуст</h2>
              <p class="empty__text">Нажмите ${icon('route', 'icon--xs')} у места в списке или на его странице — точка появится здесь. Можно собрать до 6 точек.</p>
            </div>
          </div>
        `,
      );
      return;
    }
    const legs = result?.legs || [];
    render(
      editorTarget,
      html`
        <div class="preview">
          <div class="preview__bar">
            <button type="button" class="btn btn--sm btn--ghost" data-action="route-close">${icon('arrow-left', 'icon--sm')}К списку</button>
            <button type="button" class="btn btn--sm btn--danger" data-action="route-clear">${icon('trash-2', 'icon--sm')}Очистить</button>
          </div>
          <div>
            <h2 class="preview__title">Маршрут</h2>
            <p class="preview__meta">Порядок точек можно менять стрелками</p>
          </div>
          ${stops.length >= 2 ? modeSwitch() : ''}
          ${stops.length >= 2
            ? html`
                <dl class="route-summary">
                  <div class="route-summary__item"><dt>Расстояние</dt><dd>${computing || !result ? '…' : formatDistance(result.km)}</dd></div>
                  <div class="route-summary__item"><dt>В пути</dt><dd>${computing || !result ? '…' : formatDuration(result.minutes)}</dd></div>
                </dl>
              `
            : ''}
          ${result?.approximate
            ? html`<div class="alert alert--warning">${icon('triangle-alert')}<span>Сервис маршрутов не ответил, поэтому расстояние посчитано по прямой с поправкой. Точный путь — в навигаторе.</span></div>`
            : ''}
          <ol class="route-list" role="list">
            ${stops.map(
              (stop, index) => html`
                <li class="route-stop">
                  <span class="route-stop__num" aria-hidden="true">${index + 1}</span>
                  <div>
                    <a class="route-stop__title" href="${itemUrl(stop)}">${stop.title}</a>
                    ${legs[index] ? html`<div class="route-stop__leg">→ ${formatDistance(legs[index].km)}, ${formatDuration(legs[index].minutes)}</div>` : ''}
                  </div>
                  <div class="route-stop__actions">
                    <button type="button" class="icon-btn icon-btn--sm" data-action="route-up" data-id="${stop.id}" aria-label="Выше: ${stop.title}" ${index === 0 ? 'disabled' : ''}>${icon('chevron-up')}</button>
                    <button type="button" class="icon-btn icon-btn--sm" data-action="route-down" data-id="${stop.id}" aria-label="Ниже: ${stop.title}" ${index === stops.length - 1 ? 'disabled' : ''}>${icon('chevron-down')}</button>
                    <button type="button" class="icon-btn icon-btn--sm" data-action="route-toggle" data-id="${stop.id}" aria-label="Убрать из маршрута: ${stop.title}">${icon('x')}</button>
                  </div>
                </li>
              `,
            )}
          </ol>
          ${stops.length >= 2
            ? html`<a class="btn btn--outline btn--block" href="${yandexUrl()}" target="_blank" rel="noopener">${icon('external-link')}Открыть в Яндекс Картах</a>`
            : html`<div class="alert alert--neutral">${icon('info')}<span>Добавьте ещё хотя бы одну точку, чтобы построить путь.</span></div>`}
        </div>
      `,
    );
  }

  /** Постоянный live-регион: панель и редактор перерисовываются, а озвучивается только итог. */
  const announcer = document.createElement('p');
  announcer.className = 'visually-hidden';
  announcer.setAttribute('aria-live', 'polite');
  stage.els.root.append(announcer);

  function paint() {
    paintBar();
    paintEditor();
    if (visible && stops.length) announcer.textContent = `Маршрут, ${countLabel(stops.length, ['точка', 'точки', 'точек'])}: ${summaryText()}`;
  }

  const handleClick = (event, el) => {
    const { action, id, mode } = el.dataset;
    if (action === 'route-mode') store.set({ routeMode: mode });
    if (action === 'route-open') setQuery({ panel: 'route', place: '' });
    if (action === 'route-close') setQuery({ panel: '' });
    if (action === 'route-clear') {
      clearRoute();
      setQuery({ panel: '' });
    }
    if (action === 'route-up') moveStop(id, -1);
    if (action === 'route-down') moveStop(id, 1);
  };

  on(stage.els.routeBar, 'click', '[data-action]', handleClick);
  on(stage.els.body, 'click', '[data-action^="route-"]:not([data-action="route-toggle"])', handleClick);

  store.subscribe(recompute, ['route', 'routeMode']);
  recompute();

  return {
    setVisible(value) {
      visible = value;
      paintBar();
      stage.map.setRoute(value ? { stops, geometry: result?.geometry || null, approximate: result?.approximate } : null);
    },
    openEditor(target) {
      editorTarget = target;
      paint();
      if (stops.length) stage.map.fitTo(stops.map((s) => s.coords));
    },
    closeEditor() {
      editorTarget = null;
      paintBar();
    },
  };
}

