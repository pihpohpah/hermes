/**
 * Справочник (#/catalog): поиск, подборки, категории, фильтры, сортировка, сетка или список.
 *
 * Параметры URL: s (city | nature | sea), q, cat, col (подборка), type, sort, view (grid | list).
 * Без параметра s используется глобальный сценарий из store.
 */

import { html, render, $, on, debounce, createDisposer } from '../core/dom.js';
import { store } from '../core/store.js';
import { setQuery } from '../core/router.js';
import { icon } from '../ui/icon.js';
import { SCENARIO_BY_ID, CATEGORIES, COLLECTIONS, SORTS } from '../data/taxonomy.js';
import { listItems, getItem, getCollectionCounts, countActiveFilters, peekItem } from '../api/places.js';
import { placeCard, placeCardSkeleton, collectionCard } from '../ui/cards.js';
import { openFiltersDialog, activeFilterChips, removeFilter } from '../ui/filters-dialog.js';
import { toggleRoute } from '../ui/route-store.js';
import { countLabel } from '../core/format.js';
import { footer } from '../ui/footer.js';

const PAGE_SIZE = 12;

function readQuery(query) {
  return {
    scenario: SCENARIO_BY_ID[query.s] ? query.s : store.get().scenario,
    q: query.q || '',
    cat: query.cat || '',
    collection: COLLECTIONS.some((c) => c.id === query.col) ? query.col : '',
    type: ['place', 'event'].includes(query.type) ? query.type : 'all',
    sort: SORTS.some((s) => s.id === query.sort) ? query.sort : 'recommended',
    view: query.view === 'list' ? 'list' : 'grid',
  };
}

export const catalogView = {
  title: 'Справочник',
  section: 'catalog',

  mount(target, ctx) {
    const disposer = createDisposer();
    let state = readQuery(ctx.query);
    let items = [];
    let visible = PAGE_SIZE;
    let requestId = 0;
    let collectionCounts = {};
    let collectionCovers = {};

    const filters = () => store.get().filters[state.scenario] || {};
    let mounted = true;

    render(
      target,
      html`
        <div class="page catalog">
          <div class="container">
            <header class="catalog__head">
              <div class="catalog__title-row">
                <div>
                  <h1 class="page__title">Справочник</h1>
                  <p class="page__lead">Места и события Владивостока и Приморья — от музеев и подвальных концертов до маяков и водопадов.</p>
                </div>
                <a class="btn btn--outline catalog__map-link" href="#/map">${icon('map')}На карте</a>
              </div>
              <form class="catalog__search" role="search">
                <div class="input-group">
                  ${icon('search')}
                  <input class="input input--surface catalog__search-input" type="search" name="q" placeholder="Например: маяк, джаз, Русский, бесплатно" aria-label="Поиск по справочнику" autocomplete="off" />
                  <button type="button" class="icon-btn icon-btn--sm input-group__action" data-action="clear-search" aria-label="Очистить поиск" hidden>${icon('x')}</button>
                </div>
              </form>
            </header>

            <section class="catalog__collections" aria-labelledby="collections-title">
              <h2 class="visually-hidden" id="collections-title">Подборки</h2>
              <div class="catalog__collections-row" data-collections></div>
            </section>

            <div class="catalog__toolbar" data-toolbar></div>
            <p class="visually-hidden" aria-live="polite" data-announce></p>
            <div class="chip-row catalog__active" data-active hidden></div>
            <div data-results></div>
          </div>
        </div>
        ${footer()}
      `,
    );

    const searchInput = $('.catalog__search-input', target);
    const els = {
      collections: $('[data-collections]', target),
      toolbar: $('[data-toolbar]', target),
      active: $('[data-active]', target),
      results: $('[data-results]', target),
      announce: $('[data-announce]', target),
      clear: $('[data-action="clear-search"]', target),
    };

    function paintCollections() {
      // Пустые подборки не показываем — кроме выбранной, чтобы её можно было снять
      render(
        els.collections,
        COLLECTIONS.filter((collection) => collectionCounts[collection.id] !== 0 || state.collection === collection.id).map((collection) =>
          collectionCard(collection, {
            count: collectionCounts[collection.id] || 0,
            cover: collectionCovers[collection.id],
            pressed: state.collection === collection.id,
          }),
        ),
      );
    }

    function paintToolbar(loading = false) {
      const categories = CATEGORIES[state.scenario];
      const active = countActiveFilters(filters());
      render(
        els.toolbar,
        html`
          <div class="catalog__toolbar-main">
            <div class="chip-row" role="group" aria-label="Категории">
              <button type="button" class="chip" data-action="category" data-id="" aria-pressed="${!state.cat}">Все категории</button>
              ${categories.map((c) => html`<button type="button" class="chip" data-action="category" data-id="${c.id}" aria-pressed="${state.cat === c.id}">${icon(c.icon)}${c.label}</button>`)}
            </div>
          </div>
          <div class="catalog__toolbar-side">
            <span class="catalog__count" aria-hidden="true">${loading ? 'Ищем…' : countLabel(items.length, ['результат', 'результата', 'результатов'])}</span>
            <button type="button" class="btn btn--sm btn--outline filter-btn" data-action="filters">${icon('sliders-horizontal', 'icon--sm')}Фильтры${active ? html`<span class="filter-btn__count">${active}</span>` : ''}</button>
            <label class="visually-hidden" for="catalog-sort">Сортировка</label>
            <select class="select select--sm" id="catalog-sort" data-action="sort">
              ${SORTS.map((s) => html`<option value="${s.id}" ${state.sort === s.id ? 'selected' : ''}>${s.label}</option>`)}
            </select>
            <div class="segmented catalog__view" role="group" aria-label="Вид">
              <button type="button" class="segmented__item" data-action="view" data-id="grid" aria-pressed="${state.view === 'grid'}" aria-label="Сеткой" title="Сеткой">${icon('layout-grid')}</button>
              <button type="button" class="segmented__item" data-action="view" data-id="list" aria-pressed="${state.view === 'list'}" aria-label="Списком" title="Списком">${icon('list')}</button>
            </div>
          </div>
        `,
      );
      const chips = activeFilterChips(state.scenario, filters());
      els.active.hidden = !chips.length && !state.collection;
      render(
        els.active,
        html`
          ${state.collection
            ? html`<button type="button" class="chip chip--removable" data-action="collection" data-id="${state.collection}" aria-label="Убрать подборку">${COLLECTIONS.find((c) => c.id === state.collection).title}${icon('x')}</button>`
            : ''}
          ${chips}
        `,
      );
    }

    function paintResults() {
      if (!items.length) {
        const hasConditions = state.q || state.cat || state.collection || countActiveFilters(filters()) || state.type !== 'all';
        render(
          els.results,
          html`
            <div class="empty catalog__empty">
              <span class="empty__icon">${icon('search', 'icon--lg')}</span>
              <h2 class="empty__title">Ничего не нашлось</h2>
              <p class="empty__text">${state.q ? html`По запросу «${state.q}» пусто. ` : ''}Попробуйте другое слово или уберите часть условий. Если знаете место, которого здесь нет, расскажите о нём.</p>
              <div class="empty__actions">
                ${hasConditions ? html`<button type="button" class="btn btn--primary" data-action="reset">Сбросить условия</button>` : ''}
                <a class="btn btn--outline" href="#/add">Предложить место</a>
              </div>
            </div>
          `,
        );
        return;
      }
      const { route } = store.get();
      const shown = items.slice(0, visible);
      render(
        els.results,
        html`
          <ul class="catalog__grid${state.view === 'list' ? ' catalog__grid--list' : ''}" role="list">
            ${shown.map((item, i) => html`<li>${placeCard(item, { layout: state.view === 'list' ? 'row' : 'grid', inRoute: route.includes(item.id), eager: i < 4 })}</li>`)}
          </ul>
          ${items.length > visible
            ? html`<div class="catalog__more"><button type="button" class="btn btn--outline btn--lg" data-action="more">Показать ещё ${Math.min(PAGE_SIZE, items.length - visible)}</button><span>Показано ${shown.length} из ${items.length}</span></div>`
            : ''}
        `,
      );
    }

    function paintSkeleton() {
      render(els.results, html`<ul class="catalog__grid" role="list" aria-busy="true">${Array.from({ length: 8 }, () => html`<li>${placeCardSkeleton()}</li>`)}</ul>`);
    }

    async function load() {
      const id = ++requestId;
      visible = PAGE_SIZE;
      paintToolbar(true);
      if (!items.length) paintSkeleton();
      try {
        const response = await listItems({
          scenario: state.scenario,
          type: state.type,
          q: state.q,
          category: state.cat,
          filters: filters(),
          collection: state.collection,
          sort: state.sort,
        });
        if (id !== requestId) return;
        items = response.items;
        paintToolbar();
        paintResults();
        els.announce.textContent = `Найдено: ${countLabel(items.length, ['результат', 'результата', 'результатов'])}`;
      } catch (error) {
        if (id !== requestId) return;
        console.error('Не удалось загрузить справочник', error);
        items = [];
        paintToolbar();
        render(
          els.results,
          html`
            <div class="empty catalog__empty">
              <span class="empty__icon">${icon('wifi-off', 'icon--lg')}</span>
              <h2 class="empty__title">Не получилось загрузить справочник</h2>
              <p class="empty__text">Проверьте соединение и попробуйте ещё раз.</p>
              <div class="empty__actions"><button type="button" class="btn btn--primary" data-action="retry">Повторить</button></div>
            </div>
          `,
        );
      }
    }

    let metaId = 0;

    /** Счётчики подборок; ответ для уже неактуального сценария отбрасывается. */
    async function loadMeta() {
      const id = ++metaId;
      try {
        const [collections, covers] = await Promise.all([
          getCollectionCounts(state.scenario),
          Promise.all(COLLECTIONS.map((c) => getItem(c.cover).catch(() => null))),
        ]);
        if (id !== metaId || !mounted) return;
        collectionCounts = collections;
        collectionCovers = Object.fromEntries(COLLECTIONS.map((c, i) => [c.id, covers[i]]));
        paintCollections();
      } catch (error) {
        console.warn('Не удалось загрузить счётчики справочника', error);
      }
    }

    searchInput.value = state.q;
    els.clear.hidden = !state.q;
    paintToolbar(true);
    paintSkeleton();
    loadMeta();
    load();

    const onSearch = debounce(() => setQuery({ q: searchInput.value.trim() }), 250);
    searchInput.addEventListener('input', () => {
      els.clear.hidden = !searchInput.value;
      onSearch();
    });
    $('.catalog__search', target).addEventListener('submit', (event) => {
      event.preventDefault();
      onSearch.cancel();
      setQuery({ q: searchInput.value.trim() });
    });

    disposer.add(
      on(target, 'click', '[data-action]', (event, el) => {
        const { action, id } = el.dataset;
        switch (action) {
          case 'category':
            setQuery({ cat: id });
            break;
          case 'collection':
            setQuery({ col: state.collection === id ? '' : id });
            break;
          case 'view':
            setQuery({ view: id === 'grid' ? '' : id });
            break;
          case 'clear-search':
            searchInput.value = '';
            els.clear.hidden = true;
            setQuery({ q: '' });
            searchInput.focus();
            break;
          case 'more': {
            const firstNew = visible;
            visible += PAGE_SIZE;
            paintResults();
            els.results.querySelectorAll('.place-card__link')[firstNew]?.focus();
            break;
          }
          case 'filters':
            openFiltersDialog({
              scenario: state.scenario,
              value: filters(),
              count: async (value) => (await listItems({ scenario: state.scenario, q: state.q, category: state.cat, filters: value, collection: state.collection, latency: 0 })).total,
              onApply: (value) => {
                store.set((s) => ({ filters: { ...s.filters, [state.scenario]: value } }));
                load();
              },
            });
            break;
          case 'remove-filter':
            store.set((s) => ({ filters: { ...s.filters, [state.scenario]: removeFilter(filters(), el.dataset.filter, el.dataset.value) } }));
            load();
            break;
          case 'reset': {
            store.set((s) => ({ filters: { ...s.filters, [state.scenario]: {} } }));
            searchInput.value = '';
            const urlChanges = state.q || state.cat || state.collection || state.type !== 'all';
            setQuery({ q: '', cat: '', col: '', type: '' });
            // Если менялись только фильтры в store, URL не изменится и update не вызовется
            if (!urlChanges) load();
            break;
          }
          case 'retry':
            load();
            break;
          case 'route-toggle':
            event.preventDefault();
            toggleRoute(id, peekItem(id)?.title);
            break;
          default:
        }
      }),
    );

    disposer.add(on(target, 'change', '[data-action="sort"]', (event, select) => setQuery({ sort: select.value === 'recommended' ? '' : select.value })));
    disposer.add(store.subscribe(() => paintResults(), ['route']));

    return {
      update(next) {
        const prev = state;
        state = readQuery(next.query);
        if (document.activeElement !== searchInput) searchInput.value = state.q;
        if (prev.scenario !== state.scenario) {
          items = [];
          loadMeta();
        }
        paintCollections();
        if (prev.view !== state.view) {
          paintToolbar();
          paintResults();
          return;
        }
        load();
      },
      unmount() {
        mounted = false;
        disposer.dispose();
        requestId += 1;
      },
    };
  },
};
