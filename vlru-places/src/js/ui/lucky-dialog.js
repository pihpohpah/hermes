/**
 * Окно «Мне повезёт»: выбор типа и сценария, короткая анимация перебора и результат.
 * Открывается из шапки, мобильного меню и вкладки «Для вас» в профиле. Работает без входа.
 */

import { html, render, prefersReducedMotion } from '../core/dom.js';
import { store } from '../core/store.js';
import { icon } from './icon.js';
import { openDialog } from './dialog.js';
import { placeCard, routeStops, media, approxRouteKm } from './cards.js';
import { toggleRoute, isInRoute, replaceRoute } from './route-store.js';
import { rollLucky, getLuckyPool, LUCKY_TYPES } from '../api/lucky.js';
import { SCENARIOS, SCENARIO_BY_ID, categoryOf } from '../data/taxonomy.js';
import { MAX_STOPS } from '../map/routing.js';
import { formatDistance, countLabel } from '../core/format.js';

const SPIN_FRAMES = 10;
const SCENARIO_OPTIONS = [{ id: 'any', label: 'Любое', icon: 'compass' }, ...SCENARIOS];
const STOP_OPTIONS = Array.from({ length: MAX_STOPS - 1 }, (_, i) => i + 2);
const EMPTY_TEXT = {
  event: 'В этом сценарии сейчас нет событий. Выберите другой или «Любое».',
  route: 'Здесь слишком мало мест для маршрута. Выберите другой сценарий.',
};

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const pick = (list) => list[Math.floor(Math.random() * list.length)];

function chips(name, options, current) {
  return html`
    <div class="chip-row chip-row--wrap">
      ${options.map(
        (option) => html`
          <button type="button" class="chip" data-action="${name}" data-value="${option.id}" aria-pressed="${option.id === current}">
            ${icon(option.icon)}${option.label}
          </button>
        `,
      )}
    </div>
  `;
}

function spinFrame(item) {
  return html`
    <div class="lucky__spin" aria-hidden="true">
      ${media(item, { ratio: '1', eager: true })}
      <span><strong>${item.title}</strong><span>${categoryOf(item).label}</span></span>
    </div>
  `;
}

function resultView(result, type) {
  if (!result) {
    return html`
      <div class="lucky__idle">
        <span class="empty__icon">${icon('compass', 'icon--lg')}</span>
        <p>${EMPTY_TEXT[type] || 'Ничего не нашлось. Попробуйте другие условия.'}</p>
      </div>
    `;
  }
  if (result.type === 'route') {
    const stops = result.stops.map((stop) => stop.item);
    return html`
      <div class="lucky__route">
        <p class="lucky__summary">${icon('route', 'icon--sm')}Маршрут: ${countLabel(stops.length, ['точка', 'точки', 'точек'])} · ≈ ${formatDistance(approxRouteKm(stops))}</p>
        ${routeStops(stops, { note: (stop, index) => (index ? `+${formatDistance(result.stops[index].fromPreviousKm)}` : 'Старт') })}
      </div>
    `;
  }
  return html`<div class="lucky__card">${placeCard(result.item, { layout: 'row', eager: true, inRoute: isInRoute(result.item.id) })}</div>`;
}

export function openLuckyDialog() {
  const state = { type: 'any', scenario: store.get().scenario, stops: 3, result: undefined, spinning: false };
  let el;

  function paintStage(content) {
    const stage = el.querySelector('[data-lucky-stage]');
    stage.setAttribute('aria-busy', String(state.spinning));
    render(stage, content);
  }

  function paintFooter() {
    const { result } = state;
    const item = result && result.type !== 'route' ? result.item : null;
    const mapHref = item ? `#/map?s=${item.scenario}&place=${item.id}` : '';
    const itemHref = item ? `#/${item.type}/${item.id}` : '';
    render(
      el.querySelector('.dialog__footer'),
      result === undefined || result === null
        ? html`<button type="button" class="btn btn--primary btn--lg" data-action="lucky-roll" aria-disabled="${state.spinning}">${icon('dices')}Испытать удачу</button>`
        : html`
            <button type="button" class="btn btn--ghost" data-action="lucky-roll" aria-disabled="${state.spinning}">${icon('shuffle')}Ещё раз</button>
            ${item
              ? html`<a class="btn btn--outline" href="${mapHref}">${icon('map')}На карте</a><a class="btn btn--primary" href="${itemHref}">Открыть</a>`
              : html`<button type="button" class="btn btn--primary" data-action="lucky-route">${icon('map')}Построить на карте</button>`}
          `,
    );
  }

  function paint() {
    render(
      el.querySelector('[data-lucky-options]'),
      html`
        <fieldset class="fieldset lucky__field">
          <legend>Что ищем</legend>
          ${chips('lucky-type', LUCKY_TYPES, state.type)}
        </fieldset>
        <fieldset class="fieldset lucky__field">
          <legend>Где</legend>
          ${chips('lucky-scenario', SCENARIO_OPTIONS, state.scenario)}
        </fieldset>
        ${state.type === 'route'
          ? html`
              <fieldset class="fieldset lucky__field">
                <legend>Сколько точек</legend>
                <div class="segmented" role="group">
                  ${STOP_OPTIONS.map((count) => html`<button type="button" class="segmented__item" data-action="lucky-stops" data-value="${count}" aria-pressed="${count === state.stops}">${count}</button>`)}
                </div>
              </fieldset>
            `
          : ''}
      `,
    );
  }

  async function roll() {
    if (state.spinning) return;
    state.spinning = true;
    paintFooter();
    const previous = state.result?.item?.id || '';
    const [pool, result] = await Promise.all([getLuckyPool(state.scenario), rollLucky({ type: state.type, scenario: state.scenario, stops: state.stops, exclude: previous })]);
    const frames = prefersReducedMotion() || !pool.length ? 0 : SPIN_FRAMES;
    for (let i = 0; i < frames; i += 1) {
      if (!el.isConnected) return;
      paintStage(spinFrame(pick(pool)));
      await wait(60 + i * 10);
    }
    if (!el.isConnected) return;
    state.spinning = false;
    state.result = result;
    paintStage(resultView(result, state.type));
    paintFooter();
    el.querySelector('[data-lucky-stage]').scrollIntoView({ block: 'nearest', behavior: prefersReducedMotion() ? 'auto' : 'smooth' });
  }

  openDialog({
    title: 'Мне повезёт',
    lead: 'Не знаете, куда пойти? Выберите, что ищете, а остальное решит случай.',
    wide: true,
    className: 'dialog--fixed',
    body: html`
      <div class="lucky">
        <div class="lucky__options" data-lucky-options></div>
        <div class="lucky__stage" data-lucky-stage aria-live="polite">
          <div class="lucky__idle">
            <span class="empty__icon">${icon('dices', 'icon--lg')}</span>
            <p>Бросьте кубик — выпадет место, событие или готовый маршрут.</p>
          </div>
        </div>
      </div>
    `,
    footer: html`<span></span>`,
    onMount(dialogEl, close) {
      el = dialogEl;
      dialogEl.addEventListener('click', (event) => {
        if (event.target.closest('a[href^="#/"]')) return close();
        const button = event.target.closest('[data-action]');
        if (!button) return;
        const { action, value } = button.dataset;
        if (action === 'lucky-type') state.type = value;
        if (action === 'lucky-scenario') state.scenario = SCENARIO_BY_ID[value] ? value : 'any';
        if (action === 'lucky-stops') state.stops = Number(value);
        if (['lucky-type', 'lucky-scenario', 'lucky-stops'].includes(action)) paint();
        if (action === 'lucky-roll') roll();
        if (action === 'route-toggle') {
          toggleRoute(state.result.item.id, state.result.item.title);
          paintStage(resultView(state.result, state.type));
        }
        if (action === 'lucky-route') {
          close();
          replaceRoute(state.result.stops.map((stop) => stop.item.id));
        }
      });
    },
  });

  paint();
  paintFooter();
}
