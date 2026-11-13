(function () {
  const { icon, hydrate } = window.VL_ICONS;
  const items = window.VL_DATA.items;
  const app = document.getElementById('app');
  const scenarios = {
    city: { label: 'Город', icon: 'building' },
    nature: { label: 'Природа', icon: 'tree' },
    sea: { label: 'Море', icon: 'wave' },
  };
  const state = {
    scenario: 'city',
    search: '',
    categories: new Set(),
    tags: new Set(),
    selected: null,
    panelCollapsed: false,
    mobileView: 'map',
    sort: 'popular',
    view: 'grid',
    visible: 9,
    formStep: 1,
    formData: {},
    correcting: null,
  };
  let map = null;
  let currentRoute = '';
  let searchTimer = null;

  const escapeHTML = (value) => String(value ?? '').replace(/[&<>"']/g, (character) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;',
  }[character]));

  function scenarioTabs() {
    return `<div class="scenario-tabs" role="group" aria-label="Выберите сценарий">${Object.entries(scenarios).map(([id, scenario]) => `
      <button class="scenario-tab" type="button" data-action="scenario" data-scenario="${id}" aria-pressed="${state.scenario === id}">${icon(scenario.icon, 'icon--sm')}${scenario.label}</button>`).join('')}
    </div>`;
  }

  function searchField(context) {
    return `<div class="search-field">
      ${icon('search')}
      <label class="sr-only" for="search-${context}">Поиск по местам и событиям</label>
      <input id="search-${context}" type="search" placeholder="Название, категория или район" autocomplete="off" value="${escapeHTML(state.search)}" data-search="${context}">
      ${state.search ? `<button class="icon-button search-field__clear" type="button" data-action="clear-search" aria-label="Очистить поиск">${icon('close', 'icon--sm')}</button>` : ''}
    </div>`;
  }

  function categoriesForScenario() {
    const categories = new Map();
    items.filter((item) => item.scenario === state.scenario).forEach((item) => categories.set(item.categoryId, item.category));
    return [...categories.entries()].map(([id, label]) => ({ id, label }));
  }

  function getFilteredItems() {
    const query = state.search.trim().toLocaleLowerCase('ru');
    const filtered = items.filter((item) => {
      if (item.scenario !== state.scenario) return false;
      if (state.categories.size && !state.categories.has(item.categoryId)) return false;
      if (state.tags.size && ![...state.tags].every((tag) => item.tags.includes(tag))) return false;
      if (query) {
        const haystack = [item.title, item.category, item.district, item.short, ...item.tags].join(' ').toLocaleLowerCase('ru');
        if (!haystack.includes(query)) return false;
      }
      return true;
    });
    return filtered.sort((a, b) => state.sort === 'name' ? a.title.localeCompare(b.title, 'ru') : b.popular - a.popular);
  }

  function badge(item, extraClass) {
    const label = item.kind === 'event' ? item.date : item.category;
    return `<span class="badge badge--${item.kind === 'event' ? 'event' : item.scenario}${extraClass ? ` ${extraClass}` : ''}">${item.kind === 'event' ? icon('calendar', 'icon--sm') : ''}${escapeHTML(label)}</span>`;
  }

  function placeCard(item, variant) {
    const compact = variant === 'compact';
    const list = variant === 'list';
    const metaSecondary = item.kind === 'event' ? `${item.date}, ${item.time}` : (item.price || item.hours);
    return `<article class="place-card${compact ? ' place-card--compact' : ''}${list ? ' place-card--list' : ''}${state.selected === item.id ? ' is-selected' : ''}" ${compact ? `data-select-id="${item.id}"` : ''}>
      <a class="place-card__image-link" href="#/item/${item.id}" tabindex="${compact ? '-1' : '0'}" aria-hidden="${compact ? 'true' : 'false'}">
        <img class="place-card__image" src="${item.image}" alt="Иллюстрация: ${escapeHTML(item.title)}" loading="lazy">
        ${!compact ? badge(item, 'place-card__image-badge') : ''}
      </a>
      <div class="place-card__body">
        ${compact ? badge(item) : ''}
        <h3 class="place-card__heading"><a href="#/item/${item.id}">${escapeHTML(item.title)}</a></h3>
        <div class="place-card__meta">
          <span>${icon('location', 'icon--sm')}${escapeHTML(item.district)}</span>
          ${metaSecondary ? `<span>${icon(item.kind === 'event' ? 'clock' : 'ticket', 'icon--sm')}${escapeHTML(metaSecondary)}</span>` : ''}
        </div>
        ${!compact ? `<p class="place-card__description">${escapeHTML(item.short)}</p>` : ''}
        <div class="place-card__footer">
          <span class="caption">${item.kind === 'event' ? escapeHTML(item.age) : escapeHTML(item.category)}</span>
          <a class="text-link" href="#/item/${item.id}">Подробнее${icon('arrowRight', 'icon--sm')}</a>
        </div>
      </div>
    </article>`;
  }

  function emptyState(title, text) {
    return `<div class="empty-state">
      <div class="empty-state__icon">${icon('search')}</div>
      <h2>${title || 'Ничего не нашли'}</h2>
      <p>${text || 'Попробуйте убрать часть фильтров или изменить поисковый запрос.'}</p>
      <button class="button button--secondary" type="button" data-action="reset-filters">Сбросить фильтры</button>
    </div>`;
  }

  function activeFilterCount() {
    return state.categories.size + state.tags.size;
  }

  function filterButton(extraClass) {
    const count = activeFilterCount();
    return `<button class="button button--secondary ${extraClass || ''}" type="button" data-action="open-filters">${icon('filter')}Фильтры${count ? `<span class="badge badge--city">${count}</span>` : ''}</button>`;
  }

  function activeFiltersMarkup() {
    const chips = [];
    const categories = categoriesForScenario();
    state.categories.forEach((id) => {
      const category = categories.find((entry) => entry.id === id);
      if (category) chips.push(`<button class="chip is-active" type="button" data-action="remove-category" data-category="${id}">${escapeHTML(category.label)}${icon('close')}</button>`);
    });
    state.tags.forEach((tag) => chips.push(`<button class="chip is-active" type="button" data-action="remove-tag" data-tag="${escapeHTML(tag)}">${escapeHTML(tag)}${icon('close')}</button>`));
    if (state.search) chips.push(`<button class="chip is-active" type="button" data-action="clear-search">«${escapeHTML(state.search)}»${icon('close')}</button>`);
    if (!chips.length) return '';
    return `<div class="active-filters" aria-label="Активные фильтры">${chips.join('')}<button class="text-link" type="button" data-action="reset-filters">Сбросить всё</button></div>`;
  }

  function syncSearchFields() {
    app.querySelectorAll('[data-search]').forEach((input) => {
      if (input.value !== state.search) input.value = state.search;
      const field = input.closest('.search-field');
      const clear = field.querySelector('.search-field__clear');
      if (state.search && !clear) {
        input.insertAdjacentHTML('afterend', `<button class="icon-button search-field__clear" type="button" data-action="clear-search" aria-label="Очистить поиск">${icon('close', 'icon--sm')}</button>`);
      } else if (!state.search && clear) {
        clear.remove();
      }
    });
    const reset = app.querySelector('.map-panel [data-action="reset-filters"]');
    if (reset) reset.disabled = !activeFilterCount() && !state.search;
  }

  function renderMap(params) {
    if (map) {
      map.destroy();
      map = null;
    }
    const requested = params.get('place');
    if (requested) {
      const item = items.find((entry) => entry.id === requested);
      if (item) {
        state.scenario = item.scenario;
        state.selected = item.id;
        state.categories.clear();
        state.tags.clear();
        state.search = '';
      }
    }
    const visibleItems = getFilteredItems();
    app.innerHTML = `<section class="map-page${state.panelCollapsed ? ' is-panel-collapsed' : ''}${state.mobileView === 'list' ? ' is-mobile-list' : ''}" aria-label="Карта мест и активностей">
      <aside class="map-panel" aria-label="Результаты поиска">
        <div class="map-panel__top">
          <div class="map-panel__heading">
            <h1>Исследуйте город</h1>
            <button class="icon-button" type="button" data-action="toggle-panel" aria-label="Свернуть панель результатов">${icon('collapse')}</button>
          </div>
          ${scenarioTabs()}
          ${searchField('map')}
          <div class="map-panel__filter-row">
            ${filterButton()}
            <button class="button button--ghost" type="button" data-action="reset-filters" ${!activeFilterCount() && !state.search ? 'disabled' : ''}>Сбросить</button>
          </div>
        </div>
        <div class="map-results__header"><span><strong data-result-count>${visibleItems.length}</strong> ${plural(visibleItems.length, ['результат', 'результата', 'результатов'])}</span><span>по рекомендованности</span></div>
        <div class="map-results" data-map-results>${visibleItems.length ? visibleItems.map((item) => placeCard(item, 'compact')).join('') : emptyState()}</div>
        <div class="map-panel__footer"><span>Не нашли нужное место?</span><a href="#/add">Добавить его</a></div>
      </aside>
      <div class="map-canvas">
        <div class="map-canvas__top">
          <div class="map-canvas__mobile-tabs">${scenarioTabs()}</div>
          <div class="map-canvas__mobile-search">${searchField('mobile-map')}${filterButton()}</div>
          <div class="map-context">${icon('navigation', 'icon--sm')}<span><strong data-result-count>${visibleItems.length}</strong> точек в подборке · перетаскивайте карту</span></div>
        </div>
        ${state.panelCollapsed ? `<button class="button button--secondary map-open-panel" type="button" data-action="toggle-panel">${icon('expand')}Показать список</button>` : ''}
        <div class="map-viewport" tabindex="0" role="region" aria-label="Интерактивная схема Владивостока. Перетаскивайте карту, масштабируйте колесом или кнопками плюс и минус."></div>
        <div class="map-controls" aria-label="Управление масштабом">
          <button class="icon-button" type="button" data-action="zoom-in" aria-label="Увеличить карту">${icon('plus')}</button>
          <button class="icon-button" type="button" data-action="zoom-out" aria-label="Уменьшить карту"><span aria-hidden="true">−</span></button>
          <button class="icon-button" type="button" data-action="locate" aria-label="Вернуться к центру Владивостока">${icon('locate')}</button>
          <span class="map-zoom-label" data-zoom-label>100%</span>
        </div>
        <div class="map-legend" aria-label="Обозначения на карте"><span><i></i>Город</span><span><i class="nature"></i>Природа</span><span><i class="sea"></i>Море</span><span><i class="event"></i>События</span></div>
        <div data-map-preview>${state.selected ? mapPreview(items.find((item) => item.id === state.selected)) : ''}</div>
      </div>
      <div class="map-mobile-switch" role="group" aria-label="Вид результатов">
        <button type="button" data-action="mobile-view" data-view="map" aria-pressed="${state.mobileView === 'map'}">${icon('map', 'icon--sm')}Карта</button>
        <button type="button" data-action="mobile-view" data-view="list" aria-pressed="${state.mobileView === 'list'}">${icon('list', 'icon--sm')}Список</button>
      </div>
    </section>`;

    map = new window.VLMap(app.querySelector('.map-viewport'), {
      onSelect: selectPlace,
      onZoom: (zoom) => {
        const label = app.querySelector('[data-zoom-label]');
        if (label) label.textContent = `${zoom}%`;
      },
    });
    map.setItems(visibleItems, state.selected);
    if (requested && state.selected) {
      requestAnimationFrame(() => requestAnimationFrame(() => {
        const item = items.find((entry) => entry.id === state.selected);
        if (map && item) map.centerOn(item, 1.65);
      }));
    }
    document.title = 'Карта мест и активностей — VL.ru';
  }

  function mapPreview(item) {
    if (!item) return '';
    return `<article class="map-preview">
      <img class="map-preview__image" src="${item.image}" alt="Иллюстрация: ${escapeHTML(item.title)}">
      <div class="map-preview__content">
        ${badge(item)}
        <h2>${escapeHTML(item.title)}</h2>
        <p>${escapeHTML(item.district)} · ${escapeHTML(item.kind === 'event' ? `${item.date}, ${item.time}` : item.price)}</p>
        <a class="text-link" href="#/item/${item.id}">Подробнее${icon('arrowRight', 'icon--sm')}</a>
      </div>
      <button class="icon-button map-preview__close" type="button" data-action="close-preview" aria-label="Закрыть карточку">${icon('close', 'icon--sm')}</button>
    </article>`;
  }

  function selectPlace(id) {
    const item = items.find((entry) => entry.id === id);
    if (!item) return;
    state.selected = id;
    if (map) map.setSelected(id);
    app.querySelectorAll('[data-select-id]').forEach((card) => card.classList.toggle('is-selected', card.dataset.selectId === id));
    const preview = app.querySelector('[data-map-preview]');
    if (preview) preview.innerHTML = mapPreview(item);
    if (window.innerWidth < 768 && state.mobileView === 'list') {
      state.mobileView = 'map';
      updateMobileView();
    }
  }

  function updateMapData() {
    if (!map) return;
    syncSearchFields();
    const filtered = getFilteredItems();
    if (state.selected && !filtered.some((item) => item.id === state.selected)) state.selected = null;
    map.setItems(filtered, state.selected);
    const results = app.querySelector('[data-map-results]');
    if (results) results.innerHTML = filtered.length ? filtered.map((item) => placeCard(item, 'compact')).join('') : emptyState();
    app.querySelectorAll('[data-result-count]').forEach((node) => { node.textContent = filtered.length; });
    const preview = app.querySelector('[data-map-preview]');
    if (preview) preview.innerHTML = state.selected ? mapPreview(items.find((item) => item.id === state.selected)) : '';
    app.querySelectorAll('[data-action="scenario"]').forEach((button) => button.setAttribute('aria-pressed', String(button.dataset.scenario === state.scenario)));
    app.querySelectorAll('[data-action="open-filters"]').forEach((button) => {
      const count = activeFilterCount();
      const countNode = button.querySelector('.badge');
      if (count && countNode) countNode.textContent = count;
      else if (count) button.insertAdjacentHTML('beforeend', `<span class="badge badge--city">${count}</span>`);
      else if (countNode) countNode.remove();
    });
  }

  function updateMobileView() {
    const page = app.querySelector('.map-page');
    if (page) page.classList.toggle('is-mobile-list', state.mobileView === 'list');
    app.querySelectorAll('[data-action="mobile-view"]').forEach((button) => button.setAttribute('aria-pressed', String(button.dataset.view === state.mobileView)));
  }

  function renderCatalog() {
    const filtered = getFilteredItems();
    const categories = categoriesForScenario();
    app.innerHTML = `<section class="page catalog-page">
      <div class="container">
        <div class="page-heading catalog-heading">
          <div><p class="eyebrow">Единый справочник Владивостока</p><h1>Места и активности</h1><p>Выберите сценарий — от камерных выставок до дальних мысов.</p></div>
          <a class="button button--secondary" href="#/add">${icon('plus')}Добавить место</a>
        </div>
        <div class="catalog-controls">
          <div class="catalog-controls__main">
            ${searchField('catalog')}
            ${scenarioTabs()}
            ${filterButton('button--filter-main')}
          </div>
          <div class="catalog-controls__bottom">
            <div class="chip-list chip-list--scroll" aria-label="Быстрый выбор категории">
              <button class="chip" type="button" data-action="all-categories" aria-pressed="${!state.categories.size}">Все категории</button>
              ${categories.map((category) => `<button class="chip" type="button" data-action="quick-category" data-category="${category.id}" aria-pressed="${state.categories.has(category.id)}">${escapeHTML(category.label)}</button>`).join('')}
            </div>
            <label class="sr-only" for="catalog-sort">Сортировка</label>
            <select class="select" id="catalog-sort" data-sort style="width:auto;min-height:2.5rem;padding-block:.5rem">
              <option value="popular" ${state.sort === 'popular' ? 'selected' : ''}>Сначала рекомендованные</option>
              <option value="name" ${state.sort === 'name' ? 'selected' : ''}>По алфавиту</option>
            </select>
          </div>
        </div>
        <div data-active-filters>${activeFiltersMarkup()}</div>
        <div class="catalog-summary"><span>Найдено <strong data-catalog-count>${filtered.length}</strong> ${plural(filtered.length, ['объект', 'объекта', 'объектов'])}</span>
          <div class="view-switch" role="group" aria-label="Вид списка">
            <button class="icon-button ${state.view === 'grid' ? 'is-active' : ''}" type="button" data-action="catalog-view" data-view="grid" aria-label="Показать сеткой" aria-pressed="${state.view === 'grid'}">${icon('grid')}</button>
            <button class="icon-button ${state.view === 'list' ? 'is-active' : ''}" type="button" data-action="catalog-view" data-view="list" aria-label="Показать списком" aria-pressed="${state.view === 'list'}">${icon('list')}</button>
          </div>
        </div>
        <div data-catalog-results></div>
      </div>
    </section>`;
    updateCatalogResults();
    document.title = 'Справочник мест и активностей — VL.ru';
  }

  function updateCatalogResults() {
    const container = app.querySelector('[data-catalog-results]');
    if (!container) return;
    syncSearchFields();
    const filtered = getFilteredItems();
    const visibleItems = filtered.slice(0, state.visible);
    container.innerHTML = `${filtered.length ? `<div class="catalog-grid${state.view === 'list' ? ' is-list' : ''}" data-catalog-grid>${visibleItems.map((item) => placeCard(item, state.view === 'list' ? 'list' : '')).join('')}</div>` : emptyState()}
      ${filtered.length > state.visible ? `<div class="catalog-more"><button class="button button--secondary" type="button" data-action="show-more">Показать ещё ${Math.min(6, filtered.length - state.visible)}${icon('chevronDown')}</button></div>` : ''}`;
    const count = app.querySelector('[data-catalog-count]');
    if (count) count.textContent = filtered.length;
    const active = app.querySelector('[data-active-filters]');
    if (active) active.innerHTML = activeFiltersMarkup();
  }

  function renderDetail(id) {
    const item = items.find((entry) => entry.id === id);
    if (!item) {
      renderNotFound();
      return;
    }
    const related = items.filter((entry) => entry.id !== item.id && entry.scenario === item.scenario).sort((a, b) => b.popular - a.popular).slice(0, 3);
    const gallery = [item, ...related].slice(0, 3);
    const practical = item.scenario === 'nature' || item.scenario === 'sea' ? `<div class="notice notice--warning">${icon('warning')}<div><strong>Проверьте условия перед поездкой</strong><br>Погода на побережье меняется быстро. Оценивайте маршрут по своим силам и не оставляйте мусор.</div></div>` : '';
    app.innerHTML = `<article class="page detail-page">
      <div class="container">
        <ol class="breadcrumbs" aria-label="Хлебные крошки"><li><a href="#/catalog">Справочник</a></li><li><a href="#/catalog" data-action="detail-scenario" data-scenario="${item.scenario}">${scenarios[item.scenario].label}</a></li><li aria-current="page">${escapeHTML(item.title)}</li></ol>
        <header class="detail-header">
          <div class="detail-header__title">${badge(item)}<h1>${escapeHTML(item.title)}</h1><div class="detail-header__meta"><span>${icon('location', 'icon--sm')}${escapeHTML(item.district)}</span><span>${icon(item.kind === 'event' ? 'calendar' : 'clock', 'icon--sm')}${escapeHTML(item.kind === 'event' ? `${item.date}, ${item.time}` : item.hours)}</span></div></div>
          <div class="detail-header__actions"><a class="button button--secondary" href="#/add?correct=${item.id}">${icon('edit')}Исправить</a><a class="button button--primary" href="#/map?place=${item.id}">${icon('map')}На карте</a></div>
        </header>
        <div class="detail-gallery" aria-label="Фотографии места">
          <img class="detail-gallery__main" src="${gallery[0].image}" alt="Иллюстрация: ${escapeHTML(item.title)}">
          <div class="detail-gallery__side">${gallery.slice(1).map((imageItem) => `<img src="${imageItem.image}" alt="Атмосфера сценария «${scenarios[item.scenario].label}»" loading="lazy">`).join('')}</div>
        </div>
        <div class="detail-layout">
          <div class="detail-content">
            <section class="detail-section"><h2>${item.kind === 'event' ? 'О событии' : 'О месте'}</h2><p>${escapeHTML(item.description)}</p></section>
            ${item.features.length ? `<section class="detail-section"><h2>Что важно знать</h2><ul class="feature-list">${item.features.map((feature) => `<li>${icon('check', 'icon--sm')}${escapeHTML(feature)}</li>`).join('')}</ul></section>` : ''}
            ${item.access ? `<section class="detail-section"><h2>Как добраться</h2><p>${escapeHTML(item.access)}</p></section>` : ''}
            ${practical}
            <div class="detail-source"><span>${escapeHTML(item.source)} · ${escapeHTML(item.updated)}</span><a href="#/add?correct=${item.id}">Сообщить об изменениях</a></div>
          </div>
          <aside class="detail-aside" aria-label="Практическая информация">
            <section class="info-card"><h2>Информация</h2><dl class="info-list">
              ${infoRow('location', 'Адрес или ориентир', item.address)}
              ${item.kind === 'event' ? infoRow('calendar', 'Дата и время', `${item.date} · ${item.time}`) : infoRow('clock', 'Время посещения', item.hours)}
              ${infoRow('ticket', item.kind === 'event' ? 'Билеты' : 'Стоимость', item.price)}
              ${infoRow('clock', 'Длительность', item.duration)}
              ${item.kind === 'event' ? infoRow('users', 'Возраст', item.age) : infoRow('calendar', 'Сезон', item.season)}
            </dl></section>
            <section class="info-card">
              <div class="mini-map" aria-hidden="true"><svg viewBox="0 0 360 176"><rect width="360" height="176" fill="var(--color-map-water)"/><path d="M-20 0h245c12 30-4 43-18 58-20 20-9 50-42 57-35 7-48-24-77-12-38 16-64 9-108 2Z" fill="var(--color-map-land)"/><path d="M244 118c33-22 77-16 102 12 17 20 23 36 20 46H225c-7-22-1-42 19-58Z" fill="var(--color-map-land)"/><path d="M20 55c65 9 114-6 173 19M187 0c-5 38 5 66 2 105M207 111l72 37" fill="none" stroke="white" stroke-width="4" stroke-linecap="round"/></svg><span class="mini-map__pin">${icon('location')}</span></div>
              <h2>${escapeHTML(item.district)}</h2><p class="caption">Точное положение и ближайшие места — на общей карте.</p><a class="button button--secondary button--wide info-card__map-link" href="#/map?place=${item.id}">Открыть на карте${icon('arrowRight', 'icon--sm')}</a>
            </section>
          </aside>
        </div>
        <section class="related-section"><h2>Похожие места</h2><div class="catalog-grid">${related.map((entry) => placeCard(entry)).join('')}</div></section>
      </div>
    </article>`;
    document.title = `${item.title} — VL.ru Места`;
  }

  function infoRow(iconName, label, value) {
    if (!value) return '';
    return `<div>${icon(iconName)}<div><dt>${escapeHTML(label)}</dt><dd>${escapeHTML(value)}</dd></div></div>`;
  }

  function renderForm(params) {
    const correctionId = params.get('correct');
    if (correctionId && state.correcting !== correctionId) {
      const item = items.find((entry) => entry.id === correctionId);
      if (item) {
        state.correcting = correctionId;
        state.formStep = 1;
        state.formData = { kind: item.kind, scenario: item.scenario, category: item.categoryId, title: item.title, address: item.address, description: item.description, price: item.price, source: '', contact: '' };
      }
    } else if (!correctionId && state.correcting) {
      state.correcting = null;
      state.formStep = 1;
      state.formData = {};
    }
    const correctingItem = items.find((entry) => entry.id === state.correcting);
    const data = state.formData;
    const selectedScenario = data.scenario || state.scenario || 'city';
    const categoryOptions = [...new Map(items.filter((item) => item.scenario === selectedScenario).map((item) => [item.categoryId, item.category])).entries()];
    const steps = ['Что добавляем', 'Подробности', 'Проверка и отправка'];
    app.innerHTML = `<section class="page form-page"><div class="container">
      <ol class="breadcrumbs" aria-label="Хлебные крошки"><li><a href="#/map">Карта</a></li><li aria-current="page">${correctingItem ? 'Исправление информации' : 'Добавление места'}</li></ol>
      <div class="form-page__heading"><h1>${correctingItem ? 'Исправить информацию' : 'Добавить место или событие'}</h1><p>${correctingItem ? `Предложите уточнение для карточки «${escapeHTML(correctingItem.title)}». Редакция проверит изменения перед публикацией.` : 'Знаете камерную площадку, тропу или событие, которых нет в справочнике? Поделитесь — редакция проверит информацию и добавит её на карту.'}</p></div>
      <ol class="form-steps" aria-label="Шаги формы">${steps.map((label, index) => {
        const step = index + 1;
        return `<li class="form-step${state.formStep === step ? ' is-active' : ''}${state.formStep > step ? ' is-complete' : ''}" ${state.formStep === step ? 'aria-current="step"' : ''}><span class="form-step__number">${state.formStep > step ? icon('check', 'icon--sm') : step}</span><span class="form-step__text">${label}</span></li>`;
      }).join('')}</ol>
      <form class="contribution-form" id="contribution-form" novalidate>
        <div class="contribution-form__body">${formStepMarkup(state.formStep, data, categoryOptions)}</div>
        <div class="contribution-form__footer">
          ${state.formStep > 1 ? `<button class="button button--ghost" type="button" data-action="previous-step">${icon('chevronLeft')}Назад</button>` : `<a class="button button--ghost" href="${correctingItem ? `#/item/${correctingItem.id}` : '#/map'}">Отмена</a>`}
          ${state.formStep < 3 ? `<button class="button button--primary" type="button" data-action="next-step">Продолжить${icon('arrowRight')}</button>` : `<button class="button button--primary" type="submit">${icon('check')}Отправить на проверку</button>`}
        </div>
      </form>
      <p class="form-privacy" style="max-width:48rem;margin-top:1rem">Отправляя информацию, вы помогаете поддерживать справочник актуальным. Контакт нужен только для уточнения деталей и не публикуется.</p>
    </div></section>`;
    document.title = `${correctingItem ? 'Исправление информации' : 'Добавить место'} — VL.ru`;
  }

  function formStepMarkup(step, data, categoryOptions) {
    if (step === 1) {
      return `<h2 class="contribution-form__section-title">Что вы хотите добавить?</h2><p class="contribution-form__section-description">Выберите тип и сценарий. Категория поможет правильно показать объект в фильтрах.</p>
        <fieldset class="fieldset"><legend>Тип информации</legend><div class="choice-grid choice-grid--two">
          ${choiceCard('kind', 'place', 'Место', 'Постоянная локация или площадка', 'location', data.kind !== 'event')}
          ${choiceCard('kind', 'event', 'Событие', 'Активность с датой и временем', 'calendar', data.kind === 'event')}
        </div></fieldset>
        <fieldset class="fieldset"><legend>Сценарий</legend><div class="choice-grid">
          ${Object.entries(scenarios).map(([id, scenario]) => choiceCard('scenario', id, scenario.label, { city: 'Улицы, культура, события', nature: 'Сопки, парки и тропы', sea: 'Пляжи, бухты и прогулки' }[id], scenario.icon, (data.scenario || state.scenario) === id)).join('')}
        </div></fieldset>
        <div class="field"><label class="field__label" for="form-category">Категория</label><select class="select" id="form-category" name="category" required><option value="">Выберите категорию</option>${categoryOptions.map(([id, label]) => `<option value="${id}" ${data.category === id ? 'selected' : ''}>${escapeHTML(label)}</option>`).join('')}</select><p class="field__hint">Если подходящей категории нет, выберите ближайшую и уточните в описании.</p></div>`;
    }
    if (step === 2) {
      return `<h2 class="contribution-form__section-title">Расскажите об этом месте</h2><p class="contribution-form__section-description">Пишите конкретно и по делу: чем место интересно и что важно знать перед визитом.</p>
        <div class="form-grid">
          <div class="field field--full"><label class="field__label" for="form-title">Название</label><input class="input" id="form-title" name="title" value="${escapeHTML(data.title)}" placeholder="Например, Смотровая площадка на сопке" required></div>
          <div class="field field--full"><label class="field__label" for="form-address">Адрес или ориентир</label><input class="input" id="form-address" name="address" value="${escapeHTML(data.address)}" placeholder="Район, улица или понятный ориентир" required></div>
          <div class="field field--full"><label class="field__label" for="form-description">Описание</label><textarea class="textarea" id="form-description" name="description" placeholder="Что здесь можно делать? Кому подойдёт? Есть ли ограничения?" minlength="20" required>${escapeHTML(data.description)}</textarea><p class="field__hint">Минимум 20 символов. Не копируйте рекламные тексты.</p></div>
          <div class="field"><label class="field__label" for="form-hours">Время работы <span class="optional">— необязательно</span></label><input class="input" id="form-hours" name="hours" value="${escapeHTML(data.hours)}" placeholder="Ежедневно, 10:00–20:00"></div>
          <div class="field"><label class="field__label" for="form-price">Стоимость <span class="optional">— необязательно</span></label><input class="input" id="form-price" name="price" value="${escapeHTML(data.price)}" placeholder="Бесплатно или от 500 ₽"></div>
        </div>`;
    }
    return `<h2 class="contribution-form__section-title">Добавьте подтверждение</h2><p class="contribution-form__section-description">Фотографии и ссылка помогут редакции быстрее проверить информацию. Отправка в прототипе демонстрационная.</p>
      <label class="upload-field"><input type="file" id="form-files" accept="image/jpeg,image/png,image/webp" multiple><span class="upload-field__content">${icon('upload', 'icon--lg')}<strong>Выберите фотографии</strong><span>JPG, PNG или WebP · до 5 файлов</span></span></label><div class="upload-preview" data-upload-preview></div>
      <div class="field"><label class="field__label" for="form-source">Ссылка на источник <span class="optional">— необязательно</span></label><div class="search-field">${icon('link')}<input class="input" style="padding-left:2.625rem" id="form-source" name="source" type="url" value="${escapeHTML(data.source)}" placeholder="https://"></div><p class="field__hint">Официальная страница, публикация организатора или другой проверяемый источник.</p></div>
      <div class="field"><label class="field__label" for="form-contact">Контакт для уточнений <span class="optional">— необязательно</span></label><input class="input" id="form-contact" name="contact" value="${escapeHTML(data.contact)}" placeholder="Почта или ник в Telegram"><p class="field__hint">Не будет показан другим пользователям.</p></div>
      <div class="notice">${icon('info')}<div>Перед публикацией редакция проверит местоположение, описание и практическую информацию.</div></div>`;
  }

  function choiceCard(name, value, title, description, iconName, checked) {
    return `<label class="choice-card"><input type="radio" name="${name}" value="${value}" ${checked ? 'checked' : ''}><span class="choice-card__icon">${icon(iconName)}</span><span class="choice-card__text"><strong>${title}</strong><small>${description}</small></span></label>`;
  }

  function captureForm() {
    const form = document.getElementById('contribution-form');
    if (!form) return;
    const formData = new FormData(form);
    ['kind', 'scenario', 'category', 'title', 'address', 'description', 'hours', 'price', 'source', 'contact'].forEach((key) => {
      if (formData.has(key)) state.formData[key] = String(formData.get(key));
    });
  }

  function validateStep() {
    const form = document.getElementById('contribution-form');
    form.querySelectorAll('[aria-invalid]').forEach((field) => field.removeAttribute('aria-invalid'));
    form.querySelectorAll('.field__error').forEach((error) => error.remove());
    const errors = [];
    if (state.formStep === 1) {
      const category = form.elements.category;
      if (!category.value) errors.push([category, 'Выберите категорию']);
    }
    if (state.formStep === 2) {
      const title = form.elements.title;
      const address = form.elements.address;
      const description = form.elements.description;
      if (title.value.trim().length < 2) errors.push([title, 'Укажите название места']);
      if (address.value.trim().length < 3) errors.push([address, 'Укажите адрес или понятный ориентир']);
      if (description.value.trim().length < 20) errors.push([description, 'Добавьте описание длиной не менее 20 символов']);
    }
    errors.forEach(([field, message]) => {
      field.setAttribute('aria-invalid', 'true');
      field.closest('.field').insertAdjacentHTML('beforeend', `<p class="field__error">${icon('warning', 'icon--sm')}${message}</p>`);
    });
    if (errors.length) {
      errors[0][0].focus();
      return false;
    }
    return true;
  }

  function renderSuccess() {
    app.innerHTML = `<section class="success-page"><div class="success-card"><div class="success-card__icon">${icon('checkCircle')}</div><p class="eyebrow">Спасибо за вклад</p><h1>Информация отправлена</h1><p>Мы проверим детали перед публикацией. Если понадобится уточнение, используем только указанный вами контакт.</p><div class="success-card__actions"><a class="button button--primary" href="#/map">Вернуться на карту</a><a class="button button--secondary" href="#/add">Добавить ещё</a></div></div></section>`;
    state.formStep = 1;
    state.formData = {};
    state.correcting = null;
    document.title = 'Информация отправлена — VL.ru';
  }

  function renderNotFound() {
    app.innerHTML = `<section class="not-found"><div class="not-found__content"><div class="not-found__code">404</div><h1>Такой точки на карте нет</h1><p>Возможно, ссылка устарела или в адресе есть ошибка. Вернитесь на карту или продолжите поиск в справочнике.</p><div class="not-found__actions"><a class="button button--primary" href="#/map">${icon('map')}Открыть карту</a><a class="button button--secondary" href="#/catalog">Перейти в справочник</a></div></div></section>`;
    document.title = 'Страница не найдена — VL.ru Места';
  }

  function openFilters() {
    const categories = categoriesForScenario();
    const availableTags = state.scenario === 'city' ? ['Бесплатно', 'С детьми', 'Можно с собакой', 'Требуется регистрация'] : ['Бесплатно', 'С детьми', 'Можно с собакой', 'Есть парковка'];
    const dialog = document.createElement('dialog');
    dialog.className = 'dialog filter-dialog';
    dialog.innerHTML = `<div class="dialog__header"><h2>Фильтры</h2><button class="icon-button" type="button" data-dialog-close aria-label="Закрыть фильтры">${icon('close')}</button></div>
      <div class="dialog__body">
        <fieldset class="fieldset filter-group"><legend class="filter-group__title">Категории</legend>${categories.map((category) => `<label class="check"><input type="checkbox" name="category" value="${category.id}" ${state.categories.has(category.id) ? 'checked' : ''}>${escapeHTML(category.label)}</label>`).join('')}</fieldset>
        <fieldset class="fieldset filter-group"><legend class="filter-group__title">Условия</legend>${availableTags.map((tag) => `<label class="check"><input type="checkbox" name="tag" value="${escapeHTML(tag)}" ${state.tags.has(tag) ? 'checked' : ''}>${escapeHTML(tag)}</label>`).join('')}</fieldset>
      </div>
      <div class="dialog__footer"><button class="button button--ghost" type="button" data-dialog-reset>Сбросить</button><button class="button button--primary" type="button" data-dialog-apply>Показать места</button></div>`;
    document.body.appendChild(dialog);
    dialog.showModal();
    dialog.querySelector('[data-dialog-close]').addEventListener('click', () => dialog.close());
    dialog.querySelector('[data-dialog-reset]').addEventListener('click', () => dialog.querySelectorAll('input[type="checkbox"]').forEach((input) => { input.checked = false; }));
    dialog.querySelector('[data-dialog-apply]').addEventListener('click', () => {
      state.categories = new Set([...dialog.querySelectorAll('input[name="category"]:checked')].map((input) => input.value));
      state.tags = new Set([...dialog.querySelectorAll('input[name="tag"]:checked')].map((input) => input.value));
      state.visible = 9;
      dialog.close();
      if (currentRoute === 'map') renderMap(new URLSearchParams());
      else renderCatalog();
    });
    dialog.addEventListener('click', (event) => {
      const rect = dialog.getBoundingClientRect();
      if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) dialog.close();
    });
    dialog.addEventListener('close', () => dialog.remove(), { once: true });
  }

  function showToast(message) {
    const region = document.querySelector('.toast-region');
    const toast = document.createElement('div');
    toast.className = 'toast';
    toast.innerHTML = `${icon('checkCircle')}<span>${escapeHTML(message)}</span>`;
    region.appendChild(toast);
    setTimeout(() => toast.remove(), 3500);
  }

  function plural(number, words) {
    const n = Math.abs(number) % 100;
    const n1 = n % 10;
    if (n > 10 && n < 20) return words[2];
    if (n1 > 1 && n1 < 5) return words[1];
    if (n1 === 1) return words[0];
    return words[2];
  }

  function changeScenario(scenario) {
    if (!scenarios[scenario]) return;
    state.scenario = scenario;
    state.categories.clear();
    state.tags.clear();
    state.selected = null;
    state.visible = 9;
    if (currentRoute === 'map') renderMap(new URLSearchParams());
    else renderCatalog();
  }

  function resetFilters() {
    state.search = '';
    state.categories.clear();
    state.tags.clear();
    state.selected = null;
    state.visible = 9;
    if (currentRoute === 'map') renderMap(new URLSearchParams());
    else renderCatalog();
  }

  app.addEventListener('click', (event) => {
    const actionNode = event.target.closest('[data-action]');
    if (!actionNode) {
      const card = event.target.closest('[data-select-id]');
      if (card && !event.target.closest('a')) selectPlace(card.dataset.selectId);
      return;
    }
    const action = actionNode.dataset.action;
    if (action === 'scenario' || action === 'detail-scenario') {
      if (action === 'detail-scenario') return;
      changeScenario(actionNode.dataset.scenario);
    } else if (action === 'open-filters') {
      openFilters();
    } else if (action === 'reset-filters') {
      resetFilters();
    } else if (action === 'clear-search') {
      state.search = '';
      state.visible = 9;
      if (currentRoute === 'map') renderMap(new URLSearchParams());
      else renderCatalog();
    } else if (action === 'quick-category') {
      const id = actionNode.dataset.category;
      state.categories.has(id) ? state.categories.delete(id) : state.categories.add(id);
      state.visible = 9;
      renderCatalog();
    } else if (action === 'all-categories') {
      state.categories.clear();
      renderCatalog();
    } else if (action === 'remove-category') {
      state.categories.delete(actionNode.dataset.category);
      renderCatalog();
    } else if (action === 'remove-tag') {
      state.tags.delete(actionNode.dataset.tag);
      renderCatalog();
    } else if (action === 'catalog-view') {
      state.view = actionNode.dataset.view;
      app.querySelectorAll('[data-action="catalog-view"]').forEach((button) => {
        const active = button.dataset.view === state.view;
        button.classList.toggle('is-active', active);
        button.setAttribute('aria-pressed', String(active));
      });
      updateCatalogResults();
    } else if (action === 'show-more') {
      state.visible += 6;
      updateCatalogResults();
    } else if (action === 'toggle-panel') {
      state.panelCollapsed = !state.panelCollapsed;
      renderMap(new URLSearchParams());
    } else if (action === 'zoom-in' && map) {
      map.zoomIn();
    } else if (action === 'zoom-out' && map) {
      map.zoomOut();
    } else if (action === 'locate' && map) {
      const central = getFilteredItems()[0];
      if (central) map.centerOn(central, 1.35);
      else map.fit();
      showToast('Карта возвращена к основной подборке');
    } else if (action === 'close-preview') {
      state.selected = null;
      if (map) map.setSelected(null);
      actionNode.closest('[data-map-preview]').innerHTML = '';
      app.querySelectorAll('[data-select-id]').forEach((card) => card.classList.remove('is-selected'));
    } else if (action === 'mobile-view') {
      state.mobileView = actionNode.dataset.view;
      updateMobileView();
    } else if (action === 'next-step') {
      captureForm();
      if (validateStep()) {
        state.formStep += 1;
        renderForm(new URLSearchParams(state.correcting ? `correct=${state.correcting}` : ''));
        window.scrollTo({ top: 0, behavior: 'smooth' });
      }
    } else if (action === 'previous-step') {
      captureForm();
      state.formStep = Math.max(1, state.formStep - 1);
      renderForm(new URLSearchParams(state.correcting ? `correct=${state.correcting}` : ''));
    }
  });

  app.addEventListener('input', (event) => {
    if (event.target.matches('[data-search]')) {
      state.search = event.target.value;
      state.visible = 9;
      clearTimeout(searchTimer);
      searchTimer = setTimeout(() => {
        if (currentRoute === 'map') updateMapData();
        else updateCatalogResults();
      }, 120);
    }
    if (event.target.closest('#contribution-form') && event.target.hasAttribute('aria-invalid')) {
      event.target.removeAttribute('aria-invalid');
      const error = event.target.closest('.field')?.querySelector('.field__error');
      if (error) error.remove();
    }
  });

  app.addEventListener('change', (event) => {
    if (event.target.matches('[data-sort]')) {
      state.sort = event.target.value;
      state.visible = 9;
      updateCatalogResults();
    }
    if (event.target.id === 'form-files') {
      const preview = app.querySelector('[data-upload-preview]');
      const files = [...event.target.files].slice(0, 5);
      preview.innerHTML = files.map((file) => `<span>${icon('image', 'icon--sm')}${escapeHTML(file.name)}</span>`).join('');
      if (files.length) showToast(`Выбрано файлов: ${files.length}. Загрузка демонстрационная.`);
    }
    if (event.target.name === 'scenario' && event.target.closest('#contribution-form')) {
      captureForm();
      state.formData.category = '';
      state.formData.scenario = event.target.value;
      renderForm(new URLSearchParams(state.correcting ? `correct=${state.correcting}` : ''));
    }
  });

  app.addEventListener('submit', (event) => {
    if (event.target.id !== 'contribution-form') return;
    event.preventDefault();
    captureForm();
    if (!validateStep()) return;
    window.location.hash = '#/success';
  });

  function parseRoute() {
    const raw = window.location.hash.replace(/^#/, '') || '/map';
    const [path, query = ''] = raw.split('?');
    return { path, params: new URLSearchParams(query) };
  }

  function renderRoute() {
    const { path, params } = parseRoute();
    if (map) {
      map.destroy();
      map = null;
    }
    document.querySelectorAll('dialog').forEach((dialog) => dialog.close());
    const parts = path.split('/').filter(Boolean);
    currentRoute = parts[0] || 'map';
    const isMap = currentRoute === 'map';
    document.body.classList.toggle('is-map-route', isMap);
    document.querySelectorAll('[data-nav]').forEach((link) => {
      const active = link.dataset.nav === currentRoute || (currentRoute === 'item' && link.dataset.nav === 'catalog');
      link.classList.toggle('is-active', active);
      if (active) link.setAttribute('aria-current', 'page');
      else link.removeAttribute('aria-current');
    });
    closeMobileMenu();

    if (currentRoute === 'map') renderMap(params);
    else if (currentRoute === 'catalog') renderCatalog();
    else if (currentRoute === 'item' && parts[1]) renderDetail(parts[1]);
    else if (currentRoute === 'add') renderForm(params);
    else if (currentRoute === 'success') renderSuccess();
    else renderNotFound();

    if (!isMap) window.scrollTo(0, 0);
    hydrate(app);
  }

  const menuButton = document.querySelector('.mobile-menu-button');
  const mobileMenu = document.getElementById('mobile-menu');
  function closeMobileMenu() {
    mobileMenu.hidden = true;
    menuButton.setAttribute('aria-expanded', 'false');
    menuButton.setAttribute('aria-label', 'Открыть меню');
  }
  menuButton.addEventListener('click', () => {
    const open = mobileMenu.hidden;
    mobileMenu.hidden = !open;
    menuButton.setAttribute('aria-expanded', String(open));
    menuButton.setAttribute('aria-label', open ? 'Закрыть меню' : 'Открыть меню');
  });
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape' && !mobileMenu.hidden) {
      closeMobileMenu();
      menuButton.focus();
    }
  });
  document.addEventListener('click', (event) => {
    if (!event.target.closest('.site-header') && !mobileMenu.hidden) closeMobileMenu();
  });
  window.addEventListener('hashchange', renderRoute);
  if (!window.location.hash) window.location.hash = '#/map';
  else renderRoute();
})();
