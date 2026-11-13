/**
 * Диалог расширенных фильтров сценария. Набор полей берётся из FILTERS (taxonomy.js).
 * Кнопка применения показывает число результатов «вживую», пока пользователь меняет условия.
 */

import { html, $, debounce } from '../core/dom.js';
import { icon } from './icon.js';
import { openDialog } from './dialog.js';
import { FILTERS, SCENARIO_BY_ID } from '../data/taxonomy.js';
import { countLabel } from '../core/format.js';

function field(filter, value) {
  if (filter.type === 'toggle') {
    return html`
      <label class="switch filter-field">
        <span>
          <span class="filter-field__label">${filter.label}</span>
          ${filter.hint ? html`<span class="filter-field__hint">${filter.hint}</span>` : ''}
        </span>
        <input type="checkbox" name="${filter.id}" ${value ? 'checked' : ''} />
      </label>
    `;
  }
  const multi = filter.type === 'multi';
  const selected = multi ? value || [] : value || '';
  return html`
    <fieldset class="fieldset filter-field">
      <legend class="filter-field__label">${filter.label}</legend>
      <div class="chip-row chip-row--wrap">
        ${filter.options.map(
          (option) => html`
            <label class="chip chip--lg">
              <input type="${multi ? 'checkbox' : 'radio'}" name="${filter.id}" value="${option.value}" ${(multi ? selected.includes(option.value) : selected === option.value) ? 'checked' : ''} />
              ${option.label}
            </label>
          `,
        )}
      </div>
    </fieldset>
  `;
}

function readForm(form, scenario) {
  const result = {};
  for (const filter of FILTERS[scenario]) {
    if (filter.type === 'toggle') result[filter.id] = form.elements[filter.id].checked;
    else if (filter.type === 'single') result[filter.id] = form.querySelector(`input[name="${filter.id}"]:checked`)?.value || '';
    else result[filter.id] = Array.from(form.querySelectorAll(`input[name="${filter.id}"]:checked`)).map((input) => input.value);
  }
  return result;
}

/**
 * @param {object} options
 * @param {'city'|'nature'|'sea'} options.scenario
 * @param {object} options.value — текущие фильтры
 * @param {(filters) => Promise<number>} options.count — сколько результатов даст набор фильтров
 * @param {(filters) => void} options.onApply
 */
export function openFiltersDialog({ scenario, value = {}, count, onApply }) {
  const filters = FILTERS[scenario];
  openDialog({
    title: `Фильтры: ${SCENARIO_BY_ID[scenario].label.toLowerCase()}`,
    body: html`<form class="filters-form" id="filters-form">${filters.map((filter) => field(filter, value[filter.id]))}</form>`,
    footer: html`
      <button type="button" class="btn btn--ghost" data-action="reset">Сбросить</button>
      <button type="submit" form="filters-form" class="btn btn--primary" data-apply>Показать</button>
    `,
    onMount(el, close) {
      const form = $('form', el);
      const apply = $('[data-apply]', el);

      const refresh = debounce(async () => {
        const total = await count(readForm(form, scenario));
        apply.textContent = total ? `Показать ${countLabel(total, ['результат', 'результата', 'результатов'])}` : 'Ничего не найдено';
        apply.disabled = total === 0;
      }, 120);

      form.addEventListener('change', refresh);
      refresh();

      $('[data-action="reset"]', el).addEventListener('click', () => {
        form.reset();
        form.querySelectorAll('input').forEach((input) => {
          input.checked = input.type === 'radio' && input.value === '';
        });
        refresh();
      });

      form.addEventListener('submit', (event) => {
        event.preventDefault();
        onApply(readForm(form, scenario));
        close();
      });
    },
  });
}

/** Чипы активных фильтров для строки над результатами. */
export function activeFilterChips(scenario, value = {}) {
  const chips = [];
  for (const filter of FILTERS[scenario] || []) {
    const current = value[filter.id];
    if (filter.type === 'toggle' && current) chips.push({ id: filter.id, label: filter.label });
    if (filter.type === 'single' && current) chips.push({ id: filter.id, label: filter.options.find((o) => o.value === current)?.label });
    if (filter.type === 'multi' && current?.length) {
      current.forEach((v) => chips.push({ id: filter.id, value: v, label: filter.options.find((o) => o.value === v)?.label }));
    }
  }
  return chips.map(
    (chip) => html`
      <button type="button" class="chip chip--removable" data-action="remove-filter" data-filter="${chip.id}" data-value="${chip.value || ''}" aria-label="Убрать фильтр: ${chip.label}">
        ${chip.label}${icon('x')}
      </button>
    `,
  );
}

export function removeFilter(value, id, item) {
  const next = { ...value };
  if (Array.isArray(next[id])) next[id] = next[id].filter((v) => v !== item);
  else next[id] = typeof next[id] === 'boolean' ? false : '';
  return next;
}
