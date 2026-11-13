/**
 * Экран «Карта» (#/map): сценарии, поиск, категории, фильтры, список и превью выбранного объекта.
 *
 * Состояние экрана живёт в URL:
 *   s      — сценарий (city | nature | sea); без него используется store.scenario
 *   q      — поиск
 *   cat    — категория
 *   type   — all | place | event
 *   place  — id выбранного объекта (открывает превью)
 *   panel  — route (редактор маршрута)
 * Расширенные фильтры сценария — в store.filters[сценарий]. Локальная вкладка панели
 * переключает встроенную форму фильтров и список без изменения URL.
 */

import { html, render, fragment, $, $$, on, debounce, createDisposer, isMobileLayout } from '../core/dom.js';
import { store } from '../core/store.js';
import { setQuery } from '../core/router.js';
import { icon } from '../ui/icon.js';
import { toast } from '../ui/toast.js';
import { SCENARIO_BY_ID, CATEGORIES, FILTERS, TYPE_FILTERS, ACCESS, DIFFICULTY, BEACH, categoryOf } from '../data/taxonomy.js';
import { listItems, getItem, peekItem, countActiveFilters } from '../api/places.js';
import { countActivePins } from '../api/pins.js';
import { resultRow, resultRowSkeleton, media, itemUrl } from '../ui/cards.js';
import { filterField, readFiltersForm, activeFilterChips, removeFilter } from '../ui/filters-dialog.js';
import { toggleRoute, isInRoute } from '../ui/route-store.js';
import { countLabel, formatDistance, formatEventWhen } from '../core/format.js';
import { createRouteController } from './route-panel.js';
import { shareItem } from '../ui/share.js';

const RESULT_FORMS = {
  all: ['результат', 'результата', 'результатов'],
  place: ['место', 'места', 'мест'],
  event: ['событие', 'события', 'событий'],
};

export function createExplorePanel(stage) {
  const { els, map } = stage;
  const route = createRouteController(stage);
  const disposer = createDisposer();
  let state = {};
  let results = [];
  let typeCounts = { all: 0, place: 0, event: 0 };
  let categoryCounts = {};
  let requestId = 0;
  let slots = null;
  let liveChip = null;
  let activeTab = 'filters';
  /** false после leave(): асинхронные ответы, пришедшие позже, ничего не рисуют. */
  let active = false;

  const filters = () => store.get().filters[state.scenario] || {};

  /* ── Постоянные элементы шапки (переезжают в .map-top на мобильном) ── */

  function createSlots() {
    const search = fragment(html`
      <form class="map-panel__search" role="search" data-top-slot="search">
        <div class="input-group">
          ${icon('search')}
          <input class="input" type="search" name="q" placeholder="Название, район или тег" aria-label="Поиск по местам" autocomplete="off" enterkeyhint="search" />
          <button type="button" class="icon-btn icon-btn--sm input-group__action" data-action="clear-search" aria-label="Очистить поиск" hidden>${icon('x')}</button>
        </div>
        <button type="button" class="icon-btn icon-btn--muted filter-btn" data-action="filters" aria-label="Фильтры" title="Фильтры">
          ${icon('sliders-horizontal')}<span class="filter-btn__count" hidden></span>
        </button>
      </form>
    `);

    const chips = fragment(html`<div class="chip-row chip-row--wrap" role="group" aria-label="Категории" data-top-slot="chips"></div>`);

    const input = $('input', search);
    const onInput = debounce(() => setQuery({ q: input.value.trim(), place: '' }), 250);
    input.addEventListener('input', () => {
      $('[data-action="clear-search"]', search).hidden = !input.value;
      onInput();
    });
    search.addEventListener('submit', (event) => {
      event.preventDefault();
      onInput.cancel();
      setQuery({ q: input.value.trim(), place: '' });
      input.blur();
    });

    return { search, chips, input };
  }

  function paintHeadShell() {
    render(
      els.head,
      html`
        <div class="segmented map-panel__view" role="tablist" aria-label="Содержимое панели" data-view-tabs>
          <button type="button" class="segmented__item" id="map-tab-filters" role="tab" data-action="tab" data-id="filters" aria-controls="map-panel-content" aria-selected="true">Фильтры</button>
          <button type="button" class="segmented__item" id="map-tab-list" role="tab" data-action="tab" data-id="list" aria-controls="map-panel-content" aria-selected="false" tabindex="-1">Список <span class="segmented__count" data-list-count>0</span></button>
        </div>
        <div data-slot-home="search"></div>
        <div data-slot-home="chips"></div>
        <div class="map-panel__toolbar" data-toolbar>
          <span class="map-panel__count" aria-live="polite" data-count></span>
          <div data-types></div>
        </div>
        <div class="chip-row" data-active-filters hidden></div>
      `,
    );
    $('[data-slot-home="search"]', els.head).append(slots.search);
    $('[data-slot-home="chips"]', els.head).append(slots.chips);
    els.head.style.padding = '';
  }

  function paintSlots() {
    if (document.activeElement !== slots.input) slots.input.value = state.q;
    $('[data-action="clear-search"]', slots.search).hidden = !state.q;

    const active = countActiveFilters(filters());
    const badge = $('.filter-btn__count', slots.search);
    badge.hidden = !active;
    badge.textContent = active || '';
    const filterButton = $('[data-action="filters"]', slots.search);
    filterButton.setAttribute('aria-label', active ? `Фильтры, выбрано: ${active}` : 'Фильтры');
    filterButton.setAttribute('aria-pressed', String(activeTab === 'filters'));

    const categories = CATEGORIES[state.scenario];
    render(
      slots.chips,
      html`
        <button type="button" class="chip" data-action="category" data-id="" aria-pressed="${!state.cat}">Все</button>
        ${categories.map(
          (c) => html`
            <button type="button" class="chip" data-action="category" data-id="${c.id}" aria-pressed="${state.cat === c.id}">
              ${icon(c.icon)}${c.label}${categoryCounts[c.id] ? html`<span class="chip__count">${categoryCounts[c.id]}</span>` : ''}
            </button>
          `,
        )}
      `,
    );
  }

  function paintTabs(loading = false) {
    $$('[data-action="tab"]', els.head).forEach((tab) => {
      const selected = tab.dataset.id === activeTab;
      tab.setAttribute('aria-selected', String(selected));
      tab.tabIndex = selected ? 0 : -1;
    });
    $('[data-list-count]', els.head).textContent = loading ? '…' : results.length;
  }

  function paintToolbar(loading = false) {
    const toolbar = $('[data-toolbar]', els.head);
    if (!toolbar) return;
    paintTabs(loading);
    // Счётчик — постоянный live-регион: пересоздавать его нельзя, иначе скринридер не озвучит изменение
    render($('[data-count]', toolbar), loading ? 'Ищем…' : html`<strong>${countLabel(results.length, RESULT_FORMS[state.type])}</strong>`);
    render(
      $('[data-types]', toolbar),
      html`
        <div class="segmented" role="group" aria-label="Тип">
          ${TYPE_FILTERS.map(
            (t) => html`
              <button type="button" class="segmented__item" data-action="type" data-id="${t.id}" aria-pressed="${state.type === t.id}">
                ${t.label}${t.id !== 'all' && typeCounts[t.id] ? html` <span class="segmented__count">${typeCounts[t.id]}</span>` : ''}
              </button>
            `,
          )}
        </div>
      `,
    );
    const activeRow = $('[data-active-filters]', els.head);
    const chips = activeFilterChips(state.scenario, filters());
    activeRow.hidden = activeTab !== 'list' || !chips.length;
    render(activeRow, chips);
  }

  function paintFoot() {
    render(
      els.foot,
      html`<span>Не нашли место?</span><a href="#/add?scenario=${state.scenario}">${icon('plus', 'icon--sm')} Предложить</a>`,
    );
    els.foot.hidden = false;
  }

  /* ── Фильтры и список ── */

  const hasConditions = () => Boolean(countActiveFilters(filters()) || state.q || state.cat || state.type !== 'all');

  function paintLoading() {
    render(els.body, html`<ul class="map-panel__list" role="list" aria-busy="true">${Array.from({ length: 6 }, resultRowSkeleton)}</ul>`);
  }

  function paintEmpty() {
    render(
      els.body,
      html`
        <div class="empty">
          <span class="empty__icon">${icon('search', 'icon--lg')}</span>
          <h2 class="empty__title">Ничего не нашлось</h2>
          <p class="empty__text">
            ${state.q ? html`По запросу «${state.q}» в разделе «${SCENARIO_BY_ID[state.scenario].label}» пусто.` : 'С такими условиями мест нет.'}
            Попробуйте изменить условия или загляните в другой раздел.
          </p>
          <div class="empty__actions">
            ${hasConditions() ? html`<button type="button" class="btn btn--primary" data-action="reset-all">Сбросить всё</button>` : ''}
            <a class="btn btn--outline" href="#/add?scenario=${state.scenario}">Предложить место</a>
          </div>
        </div>
      `,
    );
  }

  function paintList() {
    if (!results.length) return paintEmpty();
    const { route: routeIds } = store.get();
    const events = results.filter((item) => item.type === 'event');
    const places = results.filter((item) => item.type === 'place');
    const row = (item) => resultRow(item, { current: item.id === state.selected, inRoute: routeIds.includes(item.id) });
    const grouped = state.type === 'all' && events.length && places.length;
    render(
      els.body,
      grouped
        ? html`
            <h3 class="map-panel__group-title">Скоро</h3>
            <ul class="map-panel__list" role="list">${events.map(row)}</ul>
            <h3 class="map-panel__group-title">Места</h3>
            <ul class="map-panel__list" role="list">${places.map(row)}</ul>
          `
        : html`<ul class="map-panel__list" role="list">${results.map(row)}</ul>`,
    );
  }

  function paintFilters() {
    const value = filters();
    render(
      els.body,
      html`
        <form class="filters-form map-panel__filters" data-inline-filters>
          <p class="map-panel__filters-note">Места на карте обновляются сразу после выбора.</p>
          ${FILTERS[state.scenario].map((filter) => filterField(filter, value[filter.id]))}
          <button type="button" class="btn btn--outline" data-action="reset-all" ${hasConditions() ? '' : 'disabled'}>Сбросить всё</button>
        </form>
      `,
    );
  }

  function paintBody() {
    if (state.selected || state.panel === 'route') return;
    els.body.setAttribute('role', 'tabpanel');
    els.body.setAttribute('aria-labelledby', `map-tab-${activeTab}`);
    if (activeTab === 'filters') paintFilters();
    else paintList();
  }

  function setActiveTab(tab, { focus = false } = {}) {
    if (!['filters', 'list'].includes(tab)) return;
    activeTab = tab;
    paintSlots();
    paintToolbar();
    paintBody();
    els.body.scrollTop = 0;
    if (isMobileLayout() && stage.getSheet() === 'peek') stage.setSheet('half');
    if (focus) $(`#map-tab-${tab}`, els.head)?.focus();
  }

  /** → true, если ответ актуален и отрисован. */
  async function loadResults({ fit = false } = {}) {
    const id = ++requestId;
    const listVisible = !state.selected && state.panel !== 'route' && activeTab === 'list';
    paintToolbar(true);
    if (listVisible && (!results.length || fit)) paintLoading();
    try {
      const response = await listItems({
        scenario: state.scenario,
        type: state.type,
        q: state.q,
        category: state.cat,
        filters: filters(),
      });
      if (id !== requestId || !active) return false;
      results = response.items;
      typeCounts = response.typeCounts;
      categoryCounts = response.categoryCounts;
      map.setItems(results, { scenario: state.scenario });
      paintSlots();
      paintToolbar();
      if (!state.selected && state.panel !== 'route') paintBody();
      if (fit && results.length && (state.q || state.cat || countActiveFilters(filters()))) {
        map.fitTo(results.map((item) => item.coords), { maxZoom: 14 });
      }
      return true;
    } catch (error) {
      console.error('Не удалось загрузить места', error);
      if (id !== requestId || !active) return false;
      paintToolbar();
      if (!state.selected && state.panel !== 'route') {
        render(
          els.body,
          html`
            <div class="empty">
              <span class="empty__icon">${icon('wifi-off', 'icon--lg')}</span>
              <h2 class="empty__title">Не получилось загрузить места</h2>
              <p class="empty__text">Проверьте соединение и попробуйте ещё раз.</p>
              <div class="empty__actions"><button type="button" class="btn btn--primary" data-action="retry">Повторить</button></div>
            </div>
          `,
        );
      }
      return false;
    }
  }

  /* ── Превью выбранного объекта ── */

  function previewFacts(item) {
    const rows = [];
    if (item.type === 'event') {
      rows.push({ icon: 'calendar', label: 'Когда', value: formatEventWhen(item.startsAt) });
      rows.push({ icon: 'map-pin', label: 'Где', value: item.venue });
      rows.push({ icon: 'ticket', label: 'Вход', value: `${item.price?.label}${item.age ? ` · ${item.age}` : ''}` });
    } else {
      rows.push({ icon: 'map-pin', label: 'Адрес', value: item.address });
      rows.push({ icon: 'clock', label: 'Время', value: item.hours });
      rows.push({ icon: 'wallet', label: 'Стоимость', value: item.price?.label });
      if (item.difficulty) rows.push({ icon: 'mountain', label: 'Сложность', value: DIFFICULTY[item.difficulty] });
      if (item.beach && item.beach !== 'none') rows.push({ icon: 'sun', label: 'Берег', value: BEACH[item.beach] });
    }
    return html`
      <dl class="facts preview__facts">
        ${rows.map((row) => html`<div class="fact">${icon(row.icon)}<div><dt>${row.label}</dt><dd>${row.value}</dd></div></div>`)}
      </dl>
    `;
  }

  function paintPreview(item) {
    const category = categoryOf(item);
    const inRoute = isInRoute(item.id);
    const photos = item.photos?.length ? item.photos : [null];
    els.body.removeAttribute('role');
    els.body.removeAttribute('aria-labelledby');
    render(
      els.body,
      html`
        <article class="preview" aria-labelledby="preview-title">
          <div class="preview__bar">
            <button type="button" class="btn btn--sm btn--ghost" data-action="back">${icon('arrow-left', 'icon--sm')}${activeTab === 'list' ? 'К списку' : 'К фильтрам'}</button>
            <button type="button" class="icon-btn icon-btn--sm" data-action="share" data-id="${item.id}" aria-label="Поделиться" title="Поделиться">${icon('share-2')}</button>
          </div>
          <div class="preview__gallery-wrap">
            <div class="preview__gallery" tabindex="0" role="region" aria-label="Фотографии">
              ${photos.map((_, index) => media(item, { size: 'lg', index, eager: index === 0 }))}
            </div>
            ${photos.length > 1 ? html`<div class="preview__dots" aria-hidden="true">${photos.map((_, i) => html`<span class="${i === 0 ? 'is-active' : ''}"></span>`)}</div>` : ''}
          </div>
          <header>
            <div class="preview__meta">
              <span class="badge badge--${item.type === 'event' ? 'timed' : item.scenario}">${icon(category.icon)}${category.label}</span>
              <span>${item.type === 'event' ? item.district : `${item.district} · ${formatDistance(item.distanceKm)} от центра`}</span>
            </div>
            <h2 class="preview__title" id="preview-title" style="margin-top: var(--space-2)">${item.title}</h2>
          </header>
          <div class="preview__actions">
            <a class="btn btn--primary" href="${itemUrl(item)}">Подробнее</a>
            <button type="button" class="btn btn--outline" data-action="route-toggle" data-id="${item.id}" aria-pressed="${inRoute}">
              ${icon(inRoute ? 'check' : 'route')}${inRoute ? 'В маршруте' : 'В маршрут'}
            </button>
          </div>
          <p class="preview__text">${item.short}</p>
          ${previewFacts(item)}
          ${item.howToGet
            ? html`<div><h3 class="preview-subtitle">Как добраться</h3><p class="preview__text">${item.howToGet}</p>
                <div class="chip-row chip-row--wrap" style="margin-top: var(--space-3)">${(item.access || []).map((a) => html`<span class="badge">${icon(ACCESS[a].icon)}${ACCESS[a].label}</span>`)}</div></div>`
            : ''}
          ${item.photos?.[0] ? html`<p class="preview__credit">Фото: ${item.photos[0].author}, ${item.photos[0].license}</p>` : ''}
        </article>
      `,
    );
    const gallery = $('.preview__gallery', els.body);
    gallery?.addEventListener(
      'scroll',
      debounce(() => {
        const index = Math.round(gallery.scrollLeft / gallery.clientWidth);
        $$('.preview__dots span', els.body).forEach((dot, i) => dot.classList.toggle('is-active', i === index));
      }, 60),
    );
    els.body.scrollTop = 0;
  }

  async function showSelected(id, { fly = true } = {}) {
    let item = peekItem(id);
    if (!item) {
      try {
        item = await getItem(id);
      } catch (error) {
        if (!active) return;
        console.warn('Объект из ссылки не найден', error);
        toast('Этот объект не найден — возможно, ссылка устарела', { type: 'danger' });
        setQuery({ place: '' });
        return;
      }
      if (!active || state.selected !== id) return;
    }
    if (item.scenario !== state.scenario) {
      setQuery({ s: item.scenario, cat: '', place: id });
      return;
    }
    map.select(id);
    els.panel.classList.add('is-detail');
    paintPreview(item);
    els.foot.hidden = true;
    if (isMobileLayout() && stage.getSheet() === 'peek') stage.setSheet('half');
    if (fly) setTimeout(() => map.flyTo(item.coords, Math.max(map.getZoom(), item.scenario === 'city' ? 14 : 11)), 60);
  }

  /* ── Подсказка про «Сейчас» поверх карты ── */

  async function paintLiveChip() {
    liveChip?.remove();
    const count = await countActivePins();
    if (!count || !active) return;
    liveChip?.remove();
    liveChip = fragment(html`
      <a class="chip chip--lg chip--floating map-live-chip" href="#/now">
        <span class="live-dot" aria-hidden="true"></span>Сейчас: ${countLabel(count, ['метка', 'метки', 'меток'])} от жителей
      </a>
    `);
    els.overlay.append(liveChip);
  }

  /* ── События ── */

  function bindEvents() {
    disposer.add(
      on(els.root, 'click', '[data-action]', (event, el) => {
        const { action, id } = el.dataset;
        switch (action) {
          case 'category':
            setQuery({ cat: id || '', place: '' });
            break;
          case 'type':
            setQuery({ type: id === 'all' ? '' : id, place: '' });
            break;
          case 'tab':
            setActiveTab(id);
            break;
          case 'clear-search':
            slots.input.value = '';
            setQuery({ q: '', place: '' });
            slots.input.focus();
            break;
          case 'filters':
            setActiveTab('filters');
            if (state.selected || state.panel === 'route') setQuery({ place: '', panel: '' });
            break;
          case 'remove-filter':
            store.set((s) => ({ filters: { ...s.filters, [state.scenario]: removeFilter(filters(), el.dataset.filter, el.dataset.value) } }));
            loadResults();
            break;
          case 'reset-all': {
            const urlChanges = state.q || state.cat || state.type !== 'all' || state.selected;
            store.set((s) => ({ filters: { ...s.filters, [state.scenario]: {} } }));
            slots.input.value = '';
            if (urlChanges) setQuery({ q: '', cat: '', type: '', place: '' });
            else loadResults({ fit: true });
            break;
          }
          case 'retry':
            loadResults();
            break;
          case 'select':
            setQuery({ place: id, panel: '' });
            break;
          case 'back':
            setQuery({ place: '' });
            break;
          case 'route-toggle': {
            const item = peekItem(id);
            toggleRoute(id, item?.title);
            break;
          }
          case 'share':
            shareItem(peekItem(id));
            break;
          default:
        }
      }),
    );

    disposer.add(
      on(els.root, 'change', '[data-inline-filters]', (event, form) => {
        const value = readFiltersForm(form, state.scenario);
        store.set((s) => ({ filters: { ...s.filters, [state.scenario]: value } }));
        loadResults({ fit: true });
      }),
    );
    disposer.add(on(els.root, 'submit', '[data-inline-filters]', (event) => event.preventDefault()));

    disposer.add(
      on(els.head, 'keydown', '[role="tab"]', (event, tab) => {
        if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
        event.preventDefault();
        const next = event.key === 'Home' ? 'filters' : event.key === 'End' ? 'list' : activeTab === 'filters' ? 'list' : 'filters';
        setActiveTab(next, { focus: true });
      }),
    );

    disposer.add(
      on(els.body, 'mouseover', '.result-row', (event, row) => map.hover(row.dataset.id)),
    );
    disposer.add(
      on(els.body, 'mouseleave', '.map-panel__list', () => map.hover(null), true),
    );

    disposer.add(
      store.subscribe(() => {
        if (state.selected) {
          const item = peekItem(state.selected);
          if (item) paintPreview(item);
        } else if (state.panel !== 'route') {
          paintBody();
        }
      }, ['route']),
    );

    const onKey = (event) => {
      if (event.key === 'Escape' && state.selected && !document.querySelector('dialog[open]')) setQuery({ place: '' });
    };
    document.addEventListener('keydown', onKey);
    disposer.add(() => document.removeEventListener('keydown', onKey));

    stage.bind({
      select: (id) => setQuery({ place: id, panel: '' }),
      hover: (id) => {
        $$('.result-row', els.body).forEach((row) => row.classList.toggle('is-hovered', row.dataset.id === id));
      },
    });
  }

  /* ── Жизненный цикл ── */

  function readQuery(query) {
    return {
      scenario: SCENARIO_BY_ID[query.s] ? query.s : store.get().scenario,
      q: query.q || '',
      cat: query.cat || '',
      type: ['place', 'event'].includes(query.type) ? query.type : 'all',
      selected: query.place || null,
      panel: query.panel === 'route' ? 'route' : 'list',
    };
  }

  const panel = {
    enter(ctx) {
      active = true;
      state = readQuery(ctx.query);
      results = [];
      activeTab = 'filters';
      slots = createSlots();
      paintHeadShell();
      paintSlots();
      paintFoot();
      bindEvents();
      map.setMode('items');
      route.setVisible(true);
      stage.layoutSlots();
      paintLiveChip();
      if (!state.selected) map.resetView(state.scenario);
      loadResults().then((ok) => ok && this.applySelection());
      if (state.panel === 'route') {
        els.panel.classList.add('is-detail');
        els.body.removeAttribute('role');
        els.body.removeAttribute('aria-labelledby');
        route.openEditor(els.body);
      }
    },

    update(ctx) {
      const prev = state;
      state = readQuery(ctx.query);
      const scenarioChanged = prev.scenario !== state.scenario;
      const queryChanged = scenarioChanged || prev.q !== state.q || prev.cat !== state.cat || prev.type !== state.type;

      if (scenarioChanged) {
        results = [];
        map.select(null);
        if (!state.selected) map.resetView(state.scenario);
        paintFoot();
      }
      paintSlots();

      if (state.panel === 'route' && prev.panel !== 'route') {
        map.select(null);
        els.panel.classList.add('is-detail');
        els.body.removeAttribute('role');
        els.body.removeAttribute('aria-labelledby');
        route.openEditor(els.body);
        els.foot.hidden = true;
        if (isMobileLayout()) stage.setSheet('half');
        return;
      }
      if (state.panel !== 'route' && prev.panel === 'route') {
        route.closeEditor();
        if (!state.selected) els.panel.classList.remove('is-detail');
        paintFoot();
        if (!queryChanged && !state.selected) paintBody();
      }

      if (queryChanged) {
        loadResults({ fit: !scenarioChanged }).then((ok) => ok && this.applySelection());
      } else if (prev.selected !== state.selected) {
        this.applySelection();
      }
    },

    applySelection() {
      if (state.panel === 'route') return;
      if (state.selected) {
        showSelected(state.selected);
      } else {
        map.select(null);
        els.panel.classList.remove('is-detail');
        paintFoot();
        paintBody();
        if (isMobileLayout() && activeTab === 'list' && stage.getSheet() === 'half') stage.setSheet('peek');
      }
    },

    leave() {
      active = false;
      disposer.dispose();
      els.panel.classList.remove('is-detail');
      route.closeEditor();
      route.setVisible(false);
      liveChip?.remove();
      liveChip = null;
      els.top.replaceChildren();
      requestId += 1;
    },
  };

  return panel;
}
