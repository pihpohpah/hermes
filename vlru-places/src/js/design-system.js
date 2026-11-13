/**
 * Живые части design-system.html: палитра и контраст читаются из CSS-токенов, иконки — из спрайта,
 * карточки рисуются продуктовыми рендерерами на демо-данных.
 */

import { html, render, $, $$ } from './core/dom.js';
import { placeCard, resultRow, pinCard, collectionCard, routeCard } from './ui/cards.js';
import { listItems, getItem } from './api/places.js';
import { listPins } from './api/pins.js';
import { COLLECTIONS } from './data/taxonomy.js';
import { openDialog } from './ui/dialog.js';
import { toast } from './ui/toast.js';
import { openAuthDialog } from './ui/auth-dialog.js';
import { openLuckyDialog } from './ui/lucky-dialog.js';

const css = getComputedStyle(document.documentElement);
const token = (name) => css.getPropertyValue(name).trim();

const PALETTE = {
  Нейтральные: ['0', '25', '50', '100', '150', '200', '300', '400', '500', '600', '700', '800', '900'].map((n) => `--gray-${n}`),
  Оранжевый: ['50', '100', '200', '300', '400', '500', '600', '700', '800', '900'].map((n) => `--orange-${n}`),
  Фиолетовый: ['50', '100', '200', '300', '500', '600', '700'].map((n) => `--violet-${n}`),
  'Синий, зелёный, красный, жёлтый': ['--blue-50', '--blue-500', '--green-50', '--green-500', '--red-50', '--red-500', '--yellow-50', '--yellow-500'],
};

const ROLES = [
  ['--color-bg', 'Фон страницы'],
  ['--color-bg-subtle', 'Фон подвала, подсказок, сводок'],
  ['--color-bg-muted', 'Поля, нейтральные кнопки, заглушки'],
  ['--color-border', 'Обводки и разделители'],
  ['--color-text', 'Основной текст'],
  ['--color-text-secondary', 'Вторичный текст, описания'],
  ['--color-text-tertiary', 'Мета, подсказки'],
  ['--color-primary', 'Главные действия, выбранное, маршрут'],
  ['--color-primary-soft', 'Мягкий фон действий'],
  ['--color-primary-text', 'Оранжевый текст на белом'],
  ['--color-brand', 'Бренд: маркеры, цифра 404'],
  ['--color-accent', 'Время: события, метки «Сейчас»'],
  ['--color-focus', 'Кольцо фокуса'],
  ['--color-link', 'Ссылки'],
  ['--color-city', 'Сценарий «Город»'],
  ['--color-nature', 'Сценарий «Природа»'],
  ['--color-sea', 'Сценарий «Море»'],
  ['--color-success', 'Успех'],
  ['--color-danger', 'Ошибка'],
  ['--color-warning', 'Предупреждение'],
];

const CONTRAST_PAIRS = [
  ['--color-text', '--color-bg', 'Основной текст'],
  ['--color-text-secondary', '--color-bg', 'Вторичный текст'],
  ['--color-text-tertiary', '--color-bg', 'Мета'],
  ['--color-text-tertiary', '--color-bg-muted', 'Плейсхолдер в поле'],
  ['--color-on-primary', '--color-primary', 'Текст на оранжевой кнопке'],
  ['--color-on-primary', '--color-primary-hover', 'То же при наведении'],
  ['--color-primary-text', '--color-primary-soft', 'Вторичная кнопка'],
  ['--color-on-accent', '--color-accent', 'Текст на фиолетовой кнопке'],
  ['--color-timed-text', '--color-timed-soft', 'Бейдж времени'],
  ['--color-link', '--color-bg', 'Ссылка'],
  ['--color-nature-text', '--color-nature-soft', 'Бейдж «Природа»'],
  ['--color-sea-text', '--color-sea-soft', 'Бейдж «Море»'],
  ['--color-danger', '--color-bg', 'Текст ошибки'],
];

function luminance(hex) {
  const value = hex.replace('#', '');
  const full = value.length === 3 ? [...value].map((c) => c + c).join('') : value;
  const [r, g, b] = [0, 2, 4].map((i) => parseInt(full.slice(i, i + 2), 16) / 255).map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(a, b) {
  const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (l1 + 0.05) / (l2 + 0.05);
}

/** Значение токена, раскрывая ссылки var(--x). */
function resolve(name) {
  let value = token(name);
  for (let i = 0; i < 5 && value.startsWith('var('); i += 1) value = token(value.slice(4, -1).trim());
  return value;
}

function swatch(name, label = name.replace('--', '')) {
  const value = resolve(name);
  return html`
    <div class="ds-swatch">
      <div class="ds-swatch__color" style="background: var(${name})"></div>
      <div class="ds-swatch__info"><strong>${label}</strong><code>${value}</code></div>
    </div>
  `;
}

function paintColors() {
  render(
    $('[data-ds="palette"]'),
    Object.entries(PALETTE).map(([title, names]) => html`<p class="ds-label" style="margin-top: var(--space-4)">${title}</p><div class="ds-scale">${names.map((n) => swatch(n))}</div>`),
  );

  render(
    $('[data-ds="roles"]'),
    html`
      <thead><tr><th>Токен</th><th>Значение</th><th>Роль</th></tr></thead>
      <tbody>${ROLES.map(([name, role]) => html`<tr><td><span class="ds-dot" style="background: var(${name})"></span><code>${name}</code></td><td><code>${resolve(name)}</code></td><td>${role}</td></tr>`)}</tbody>
    `,
  );

  render(
    $('[data-ds="contrast"]'),
    html`
      <thead><tr><th>Пара</th><th>Пример</th><th>Контраст</th><th>Норма AA</th></tr></thead>
      <tbody>
        ${CONTRAST_PAIRS.map(([fg, bg, label]) => {
          const ratio = contrast(resolve(fg), resolve(bg));
          const verdict = ratio >= 4.5 ? html`<span class="badge badge--success">текст</span>` : ratio >= 3 ? html`<span class="badge badge--warning">UI и крупный текст</span>` : html`<span class="badge badge--danger">не для текста</span>`;
          return html`<tr><td>${label}<br /><code>${fg} / ${bg}</code></td><td><span style="padding: 0.25rem 0.5rem; border-radius: 6px; background: var(${bg}); color: var(${fg}); font-weight: 600">Аа Места</span></td><td class="tabular">${ratio.toFixed(2)}:1</td><td>${verdict}</td></tr>`;
        })}
      </tbody>
    `,
  );

  render(
    $('[data-ds="map-colors"]'),
    ['--map-land', '--map-water', '--map-park', '--map-wood', '--map-residential', '--map-building', '--map-road-casing', '--map-motorway', '--map-label', '--map-water-label'].map((n) => swatch(n)),
  );
}

function paintSpacing() {
  const steps = ['1', '2', '3', '4', '5', '6', '8', '10', '12', '16'];
  render(
    $('[data-ds="spacing"]'),
    steps.map((n) => html`<div class="ds-space"><code class="ds-label" style="width: 6rem; margin: 0">--space-${n}</code><span class="ds-space__bar" style="width: var(--space-${n})"></span><span>${parseFloat(token(`--space-${n}`)) * 16} px</span></div>`),
  );
}

async function paintIcons() {
  const text = await fetch('assets/icons.svg').then((r) => r.text());
  const names = [...text.matchAll(/id="i-([a-z0-9-]+)"/g)].map((m) => m[1]).sort();
  render(
    $('[data-ds="icons"]'),
    names.map((name) => html`<div class="ds-icon"><svg class="icon icon--lg" aria-hidden="true"><use href="assets/icons.svg#i-${name}"></use></svg><span>${name}</span></div>`),
  );
}

async function paintCards() {
  const [{ items: city }, { items: events }, { items: sea }] = await Promise.all([
    listItems({ scenario: 'city', type: 'place', latency: 0 }),
    listItems({ scenario: 'all', type: 'event', sort: 'date', latency: 0 }),
    listItems({ scenario: 'sea', type: 'place', latency: 0 }),
  ]);
  render($('[data-ds="place-cards"]'), [city[0], events[0], sea[0], city[3]].filter(Boolean).map((item, i) => placeCard(item, { inRoute: i === 2 })));
  render($('[data-ds="rows"]'), [events[0], city[1], sea[1]].filter(Boolean).map((item, i) => resultRow(item, { current: i === 1, inRoute: i === 2 })));

  const pins = await listPins();
  render($('[data-ds="pins"]'), html`${pins.slice(0, 2).map((pin) => html`<ul role="list" style="padding: 0; margin: 0; list-style: none">${pinCard(pin)}</ul>`)}`);

  const covers = await Promise.all(COLLECTIONS.map((c) => getItem(c.cover).catch(() => null)));
  render($('[data-ds="collections"]'), COLLECTIONS.map((c, i) => collectionCard(c, { count: 12 - i * 2, cover: covers[i], pressed: i === 1 })));

  render(
    $('[data-ds="recommendations"]'),
    html`
      ${placeCard(sea[1], { reason: 'Вы искали «закат»' })}
      ${placeCard(city[2], { reason: 'Как «Маяк Токаревского», где вы были' })}
      <div style="grid-column: 1 / -1; max-width: 32rem">${routeCard({ scenario: 'city', title: 'Прогулка по городу', reason: 'Собрали из того, что вам близко: закат, история', stops: city.slice(0, 3) })}</div>
    `,
  );
}

function bindDemos() {
  document.addEventListener('click', (event) => {
    const trigger = event.target.closest('[data-ds-open]');
    if (trigger) {
      const kind = trigger.dataset.dsOpen;
      if (kind === 'dialog') {
        openDialog({
          title: 'Фильтры: природа',
          body: html`<p>Так выглядит окно: заголовок, содержимое с прокруткой и закреплённые кнопки внизу. Закрывается по Escape, крестику и клику по фону.</p>`,
          footer: html`<button type="button" class="btn btn--ghost" data-dialog-close>Сбросить</button><button type="button" class="btn btn--primary" data-dialog-close>Показать 12 результатов</button>`,
        });
      }
      if (kind === 'toast') toast('Ссылка скопирована', { type: 'success', action: { label: 'Отменить', onClick: () => {} } });
      if (kind === 'auth') openAuthDialog({ reason: 'Пример окна входа — тот же компонент, что в продукте.' });
      if (kind === 'lucky') openLuckyDialog();
      return;
    }
    // Витрина: кнопки внутри примеров не должны никуда вести
    if (event.target.closest('.ds-stage a, .ds-stage [data-action]')) event.preventDefault();
  });

  // Витрина: переключатели внутри примеров работают как в продукте
  $$('.ds-stage .chip[aria-pressed], .ds-stage .segmented__item, .ds-stage .tab').forEach((el) => {
    el.addEventListener('click', () => {
      const attr = ['aria-selected', 'aria-checked', 'aria-pressed'].find((name) => el.hasAttribute(name));
      if (!attr) return;
      const exclusive = !el.classList.contains('chip');
      if (exclusive) el.parentElement.querySelectorAll(`[${attr}]`).forEach((other) => other.setAttribute(attr, 'false'));
      el.setAttribute(attr, exclusive ? 'true' : String(el.getAttribute(attr) !== 'true'));
    });
  });
}

paintColors();
paintSpacing();
paintIcons();
paintCards();
bindDemos();
