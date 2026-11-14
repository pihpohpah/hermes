/* «Мне повезёт»: диалог случайного места, события или маршрута.
 * Бросок делает сервер (GET /api/lucky), клиент анимирует перебор и рисует результат. */
import { api } from './api.js';
import { escapeHtml, icon, countLabel, formatDistance } from './util.js';
import { openDialog, toast } from './ui.js';
import { CATEGORY_LABELS, SCENARIO_LABELS, TYPE_LABELS } from './taxonomy.js';
import { MAX_STOPS, replaceRoute } from './route.js';

const LUCKY_TYPES = [
  { id: 'any', label: 'Что угодно', icon: 'sparkles' },
  { id: 'place', label: 'Место', icon: 'map-pin' },
  { id: 'event', label: 'Событие', icon: 'calendar' },
  { id: 'route', label: 'Маршрут', icon: 'route' },
];

const SCENARIO_OPTIONS = [
  { id: 'any', label: 'Любое', icon: 'compass' },
  { id: 'city', label: SCENARIO_LABELS.city, icon: 'house' },
  { id: 'nature', label: SCENARIO_LABELS.nature, icon: 'tree-pine' },
  { id: 'sea', label: SCENARIO_LABELS.sea, icon: 'shell' },
];

const STOP_OPTIONS = Array.from({ length: MAX_STOPS - 1 }, (_, i) => i + 2);
const SPIN_FRAMES = 10;

const EMPTY_TEXT = {
  event: 'В этом сценарии сейчас нет событий. Выберите другой или «Любое».',
  route: 'Здесь слишком мало мест для маршрута. Выберите другой сценарий.',
};

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const pick = (list) => list[Math.floor(Math.random() * list.length)];
const itemURL = (item) => (item.type === 'event' ? `/event/${item.id}` : `/place/${item.id}`);

function categoryLabel(item) {
  if (item.type === 'event') return TYPE_LABELS.event;
  const byScenario = CATEGORY_LABELS[item.scenario] || {};
  return byScenario[item.category] || SCENARIO_LABELS[item.scenario] || '';
}

function mediaHtml(item) {
  const photo = item.photos?.[0];
  if (photo?.src) {
    return `<div class="media"><img src="${escapeHtml(photo.src_sm || photo.src)}" alt="${escapeHtml(photo.alt || item.title)}" loading="eager" /></div>`;
  }
  const scenarioIcon = { city: 'house', nature: 'tree-pine', sea: 'shell' }[itemScenario(item)] || 'map-pin';
  return `<div class="media media--placeholder" data-scenario="${escapeHtml(itemScenario(item))}">${icon(scenarioIcon)}</div>`;
}

function itemScenario(item) {
  return item.type === 'event' ? 'timed' : item.scenario;
}

function resultHtml(result, requestedType) {
  if (!result) {
    return `
      <div class="lucky__idle">
        <span class="empty__icon">${icon('compass', 'icon--lg')}</span>
        <p>${EMPTY_TEXT[requestedType] || 'Ничего не нашлось. Попробуйте другие условия.'}</p>
      </div>`;
  }
  if (result.type === 'route') {
    const stops = result.stops.map((stop) => stop.item);
    const approxKm = result.stops.slice(1).reduce((sum, stop) => sum + stop.from_previous_km, 0) * 1.25;
    const legs = result.stops.map((stop, index) => `
      <li class="route-stop">
        <span class="route-stop__num" aria-hidden="true">${index + 1}</span>
        <div>
          <a class="route-stop__title" href="${itemURL(stop.item)}">${escapeHtml(stop.item.title)}</a>
          <div class="route-stop__leg">${index ? `+${formatDistance(stop.from_previous_km)}` : 'Старт'}</div>
        </div>
      </li>`).join('');
    return `
      <div class="lucky__route" data-lucky="route">
        <p class="lucky__summary">${icon('route', 'icon--sm')}Маршрут: ${countLabel(stops.length, ['точка', 'точки', 'точек'])} · ≈ ${formatDistance(approxKm)}</p>
        <ol class="route-list" role="list">${legs}</ol>
      </div>`;
  }
  const item = result.item;
  return `
    <div class="lucky__card" data-lucky="item" data-id="${escapeHtml(item.id)}">
      <div class="lucky__spin">
        ${mediaHtml(item)}
        <span><strong>${escapeHtml(item.title)}</strong><span>${escapeHtml(categoryLabel(item))}</span></span>
      </div>
    </div>`;
}

/** Пересчёт подписи кнопки броска: во время перебора — «Бросаем…». */
function paintRollButton(root, { spinning, rolled }) {
  const button = root.querySelector('[data-lucky-roll]');
  if (!button) return;
  button.disabled = spinning;
  button.setAttribute('aria-disabled', String(spinning));
  if (spinning) {
    button.innerHTML = `${icon('dices')}Бросаем…`;
  } else {
    button.innerHTML = rolled ? `${icon('shuffle')}Ещё раз` : `${icon('dices')}Испытать удачу`;
  }
}

/** Кнопки результата: «На карте»/«Открыть» для места, «Построить на карте» для маршрута. */
function paintResultActions(root, result) {
  const slot = root.querySelector('[data-lucky-actions]');
  if (!slot) return;
  if (!result) {
    slot.innerHTML = '';
    return;
  }
  if (result.type === 'route') {
    slot.innerHTML = `<button type="button" class="btn btn--primary" data-lucky-build>${icon('map')}Построить на карте</button>`;
    return;
  }
  const item = result.item;
  slot.innerHTML = `
    <a class="btn btn--outline" href="/?place=${encodeURIComponent(item.id)}">${icon('map')}На карте</a>
    <a class="btn btn--primary" href="${itemURL(item)}">Открыть</a>`;
}

export function openLuckyDialog() {
  const state = {
    type: 'any',
    scenario: 'any',
    stops: 3,
    result: undefined, // undefined — не бросали, null — пусто
    spinning: false,
  };

  const dialog = openDialog({
    title: 'Мне повезёт',
    lead: 'Не знаете, куда пойти? Выберите, что ищете, а остальное решит случай.',
    wide: true,
    className: 'dialog--fixed',
    body: `
      <div class="lucky">
        <div class="lucky__options">
          <fieldset class="fieldset lucky__field">
            <legend>Что ищем</legend>
            <div class="chip-row chip-row--wrap" data-lucky-types></div>
          </fieldset>
          <fieldset class="fieldset lucky__field">
            <legend>Где</legend>
            <div class="chip-row chip-row--wrap" data-lucky-scenarios></div>
          </fieldset>
          <fieldset class="fieldset lucky__field" data-lucky-stops-field hidden>
            <legend>Сколько точек</legend>
            <div class="segmented" role="group" data-lucky-stops></div>
          </fieldset>
        </div>
        <div class="lucky__stage" data-lucky-stage aria-live="polite">
          <div class="lucky__idle">
            <span class="empty__icon">${icon('dices', 'icon--lg')}</span>
            <p>Бросьте кубик — выпадет место, событие или готовый маршрут.</p>
          </div>
        </div>
      </div>`,
    footer: `
      <span data-lucky-actions class="lucky__actions"></span>
      <button type="button" class="btn btn--primary btn--lg" data-lucky-roll>${icon('dices')}Испытать удачу</button>`,
    onMount(el) {
      paintChips(el, state);

      el.addEventListener('click', (event) => {
        const chip = event.target.closest('[data-lucky-set]');
        if (chip) {
          const { luckySet, value } = chip.dataset;
          if (luckySet === 'type') state.type = value;
          if (luckySet === 'scenario') state.scenario = value;
          if (luckySet === 'stops') state.stops = Number(value);
          paintChips(el, state);
          return;
        }
        if (event.target.closest('[data-lucky-roll]')) {
          roll(el, state);
          return;
        }
        if (event.target.closest('[data-lucky-build]')) {
          const ids = (state.result?.stops || []).map((stop) => stop.item.id);
          replaceRoute(ids);
          toast(`Маршрут: ${countLabel(ids.length, ['точка', 'точки', 'точек'])}. Считаем путь…`);
          location.href = '/?panel=route';
        }
      });
    },
  });

  return dialog;
}

function paintChips(root, state) {
  const types = root.querySelector('[data-lucky-types]');
  if (types) {
    types.innerHTML = LUCKY_TYPES.map((option) => `
      <button type="button" class="chip" data-lucky-set="type" data-value="${option.id}" aria-pressed="${option.id === state.type}">
        ${icon(option.icon)}${option.label}
      </button>`).join('');
  }
  const scenarios = root.querySelector('[data-lucky-scenarios]');
  if (scenarios) {
    scenarios.innerHTML = SCENARIO_OPTIONS.map((option) => `
      <button type="button" class="chip" data-lucky-set="scenario" data-value="${option.id}" aria-pressed="${option.id === state.scenario}">
        ${icon(option.icon)}${option.label}
      </button>`).join('');
  }
  const stopsField = root.querySelector('[data-lucky-stops-field]');
  if (stopsField) stopsField.hidden = state.type !== 'route';
  const stops = root.querySelector('[data-lucky-stops]');
  if (stops) {
    stops.innerHTML = STOP_OPTIONS.map((count) => `
      <button type="button" class="segmented__item" data-lucky-set="stops" data-value="${count}" aria-pressed="${count === state.stops}">${count}</button>`).join('');
  }
}

async function roll(root, state) {
  if (state.spinning) return;
  state.spinning = true;
  paintRollButton(root, { spinning: true, rolled: state.result !== undefined });

  const previous = state.result?.type === 'route'
    ? state.result.stops[0]?.item?.id || ''
    : state.result?.item?.id || '';

  const params = new URLSearchParams({ type: state.type, s: state.scenario, stops: String(state.stops) });
  if (previous) params.set('exclude', previous);

  const stage = root.querySelector('[data-lucky-stage]');
  stage.setAttribute('aria-busy', 'true');
  const request = api(`/lucky?${params.toString()}`).catch((error) => {
    console.warn('Не удалось бросить кубик', error);
    toast('Не получилось. Проверьте соединение и попробуйте ещё раз', { type: 'danger' });
    return undefined;
  });

  // Перебор кадрами из того же пула, что и бросок: пока сервер думает,
  // пользователь видит «мельтешение» вариантов.
  const pool = await api(`/map-items?s=${encodeURIComponent(state.scenario)}`)
    .then((data) => (Array.isArray(data) ? data : data?.items) || [])
    .catch(() => []);
  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  const frames = reduced || !pool.length ? 0 : SPIN_FRAMES;
  for (let i = 0; i < frames; i += 1) {
    if (!root.isConnected) return;
    const item = pick(pool);
    stage.innerHTML = `
      <div class="lucky__spin" aria-hidden="true">
        ${mediaHtml(item)}
        <span><strong>${escapeHtml(item.title)}</strong><span>${escapeHtml(categoryLabel(item))}</span></span>
      </div>`;
    paintRollButton(root, { spinning: true, rolled: state.result !== undefined });
    await wait(60 + i * 10);
  }

  const result = await request;
  if (!root.isConnected) return;

  state.spinning = false;
  state.result = result === undefined ? state.result : result;
  stage.setAttribute('aria-busy', 'false');
  stage.innerHTML = resultHtml(state.result, state.type);
  stage.setAttribute('aria-busy', 'false');
  paintRollButton(root, { spinning: false, rolled: true });
  paintResultActions(root, state.result);
  paintChips(root, state);
  stage.scrollIntoView({ block: 'nearest', behavior: reduced ? 'auto' : 'smooth' });
}
