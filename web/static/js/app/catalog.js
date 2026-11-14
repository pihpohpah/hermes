/* Диалог фильтров каталога и делегированные обработчики поиска/сортировки/
 * фильтров/htmx-afterSwap. URL меняет JS, свопает htmx. */
import { $, $$, countLabel, currentParams, debounce, escapeHtml } from './util.js';
import { api } from './api.js';
import { openDialog } from './ui.js';
import { FILTERS, FILTER_IDS, SCENARIO_LABELS } from './taxonomy.js';
import { layoutSlots } from './layout.js';
import { panelSwap, refreshMapItems, refreshPins, updateKindCounts, mapItems } from './nav.js';
import { routeState } from './route.js';

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

export function openFiltersDialog({ scenario }) {
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

export function bindCatalogListeners() {
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
}
