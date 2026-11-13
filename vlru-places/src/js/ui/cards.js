/**
 * Рендереры карточек. Чистые функции: данные → SafeHtml. Никакой логики событий внутри.
 * Разметка соответствует компонентам из src/styles/components/card.css и design-system.html.
 */

import { html, attr } from '../core/dom.js';
import { icon } from './icon.js';
import { categoryOf, PIN_KIND_BY_ID, DIFFICULTY } from '../data/taxonomy.js';
import { formatDistance, formatEventWhen, dateTileParts, pinTiming, initials, avatarStyle, countLabel } from '../core/format.js';

export const itemUrl = (item) => `#/${item.type === 'event' ? 'event' : 'place'}/${item.id}`;

export const scenarioKey = (item) => (item.type === 'event' ? 'timed' : item.scenario);

/** Фото или заглушка с иконкой категории. size: 'sm' — превью 640 px, 'lg' — 1400 px. */
export function media(item, { size = 'sm', index = 0, ratio = null, badges = null, eager = false } = {}) {
  const photo = item.photos?.[index];
  const style = ratio ? `--media-ratio:${ratio}` : null;
  if (!photo) {
    return html`
      <div class="media media--placeholder" data-scenario="${scenarioKey(item)}"${attr('style', style)}>
        ${icon(categoryOf(item).icon || 'image')} ${badges ? html`<div class="media__badges">${badges}</div>` : ''}
      </div>
    `;
  }
  return html`
    <div class="media"${attr('style', style)}>
      <img
        src="${size === 'lg' ? photo.src : photo.srcSm || photo.src}"
        alt="${photo.alt || item.title}"
        width="${photo.width || 640}"
        height="${photo.height || 480}"
        loading="${eager ? 'eager' : 'lazy'}"
        decoding="async"
      />
      ${badges ? html`<div class="media__badges">${badges}</div>` : ''}
    </div>
  `;
}

export function dateTile(iso) {
  const { day, month } = dateTileParts(iso);
  return html`<span class="date-tile"><span class="date-tile__day">${day}</span><span class="date-tile__month">${month}</span></span>`;
}

/** Короткие факты: цена, расстояние, сложность / дата. */
function facts(item, { compact = false } = {}) {
  const list = [];
  if (item.type === 'event') {
    list.push(html`<span>${icon('clock')}${formatEventWhen(item.startsAt).split(' · ')[1]}</span>`);
    list.push(html`<span>${icon('ticket')}${item.price?.label}</span>`);
  } else {
    if (item.price) list.push(html`<span>${icon('wallet')}${item.price.free ? 'Бесплатно' : item.price.label}</span>`);
    if (item.difficulty && item.scenario === 'nature') list.push(html`<span>${icon('mountain')}${DIFFICULTY[item.difficulty]}</span>`);
    if (!compact || item.scenario !== 'city') list.push(html`<span>${icon('navigation')}${formatDistance(item.distanceKm)} от центра</span>`);
  }
  return list;
}

/** Строка мета-информации: категория · район / дата. */
function meta(item, className) {
  const category = categoryOf(item);
  return html`
    <span class="${className}__category" data-scenario="${scenarioKey(item)}">${category.label}</span>
    <span class="sep" aria-hidden="true"></span>
    <span>${item.type === 'event' ? formatEventWhen(item.startsAt).split(' · ')[0] : item.district}</span>
  `;
}

/** Карточка справочника. layout: 'grid' | 'row'. */
export function placeCard(item, { layout = 'grid', inRoute = false, eager = false } = {}) {
  const badges =
    item.type === 'event'
      ? dateTile(item.startsAt)
      : item.featured
        ? html`<span class="badge badge--overlay">${icon('star')}Стоит увидеть</span>`
        : null;
  const routeButton = html`
    <button
      type="button"
      class="icon-btn icon-btn--floating icon-btn--sm icon-btn--round"
      data-action="route-toggle"
      data-id="${item.id}"
      aria-pressed="${inRoute}"
      aria-label="${inRoute ? 'Убрать из маршрута' : 'Добавить в маршрут'}: ${item.title}"
      title="${inRoute ? 'Убрать из маршрута' : 'Добавить в маршрут'}"
    >
      ${icon(inRoute ? 'check' : 'route', 'icon--sm')}
    </button>
  `;
  return html`
    <article class="place-card${layout === 'row' ? ' place-card--row' : ''}">
      <div class="place-card__media">
        ${media(item, { badges, eager })}
        <div class="place-card__action">${routeButton}</div>
      </div>
      <div class="place-card__body">
        <div class="place-card__meta">${meta(item, 'place-card')}</div>
        <h3 class="place-card__title">
          <a class="place-card__link" href="${itemUrl(item)}">${item.title}</a>
        </h3>
        <p class="place-card__text">${item.short}</p>
        <div class="place-card__facts">${facts(item)}</div>
      </div>
    </article>
  `;
}

export function placeCardSkeleton() {
  return html`
    <div class="place-card" aria-hidden="true">
      <div class="media skeleton"></div>
      <div class="place-card__body">
        <div class="skeleton skeleton--text" style="width: 40%"></div>
        <div class="skeleton skeleton--text" style="width: 85%; height: 1.125rem"></div>
        <div class="skeleton skeleton--text" style="width: 70%"></div>
      </div>
    </div>
  `;
}

/** Строка результата в панели карты. */
export function resultRow(item, { current = false, inRoute = false } = {}) {
  return html`
    <li class="result-row" data-id="${item.id}"${attr('aria-current', current ? 'true' : null)}>
      <button type="button" class="result-row__open" data-action="select" data-id="${item.id}" aria-label="${item.title}, ${categoryOf(item).label}"></button>
      ${media(item)}
      <div class="result-row__body">
        <span class="result-row__title">${item.title}</span>
        <span class="result-row__meta">${meta(item, 'result-row')}</span>
        <span class="result-row__facts">${facts(item, { compact: true })}</span>
      </div>
      <div class="result-row__aside">
        <button
          type="button"
          class="icon-btn icon-btn--sm"
          data-action="route-toggle"
          data-id="${item.id}"
          aria-pressed="${inRoute}"
          aria-label="${inRoute ? 'Убрать из маршрута' : 'Добавить в маршрут'}: ${item.title}"
          title="${inRoute ? 'Убрать из маршрута' : 'Добавить в маршрут'}"
        >
          ${icon(inRoute ? 'check' : 'route', 'icon--sm')}
        </button>
      </div>
    </li>
  `;
}

export function resultRowSkeleton() {
  return html`
    <li class="result-row" aria-hidden="true">
      <div class="media skeleton"></div>
      <div class="result-row__body">
        <div class="skeleton skeleton--text" style="width: 70%"></div>
        <div class="skeleton skeleton--text" style="width: 45%"></div>
        <div class="skeleton skeleton--text" style="width: 55%"></div>
      </div>
    </li>
  `;
}

export function avatar(name, size = 'sm') {
  return html`<span class="avatar avatar--${size}" style="${avatarStyle(name)}" aria-hidden="true">${initials(name)}</span>`;
}

/** Демо-участники для аватаров: детерминированно по id метки. */
const NAMES = ['Аня', 'Дима', 'Света', 'Кирилл', 'Маша', 'Олег', 'Лена', 'Ваня', 'Катя', 'Саша', 'Рома', 'Юля'];
export function participantNames(pin, count = 3) {
  const seed = [...pin.id].reduce((sum, ch) => sum + ch.charCodeAt(0), 0);
  return Array.from({ length: Math.min(count, pin.goingTotal) }, (_, i) => NAMES[(seed + i * 5) % NAMES.length]);
}

export function pinTimeBadge(pin) {
  const timing = pinTiming(pin.startsAt, pin.endsAt);
  return html`
    <span class="badge badge--timed">
      ${timing.state === 'live' ? html`<span class="live-dot" aria-hidden="true"></span>` : icon('clock')} ${timing.label}
    </span>
  `;
}

export function pinMeter(pin) {
  if (!pin.need) {
    return html`<div class="meter"><div class="meter__label"><span>Идут <strong>${pin.goingTotal}</strong></span><span>Без ограничений</span></div></div>`;
  }
  const total = pin.goingTotal + pin.need;
  const left = Math.max(0, pin.need - (pin.joined && !pin.mine ? 1 : 0));
  const percent = Math.min(100, Math.round((pin.goingTotal / total) * 100));
  return html`
    <div class="meter">
      <div class="meter__label">
        <span>Идут <strong>${pin.goingTotal}</strong></span>
        <span>${left ? `нужно ещё ${countLabel(left, ['человек', 'человека', 'человек'])}` : 'Набрали'}</span>
      </div>
      <div class="meter__track" role="progressbar" aria-label="Набор участников" aria-valuemin="0" aria-valuemax="${total}" aria-valuenow="${pin.goingTotal}">
        <div class="meter__fill" style="width: ${percent}%"></div>
      </div>
    </div>
  `;
}

/** Карточка быстрой метки. */
export function pinCard(pin, { current = false } = {}) {
  const kind = PIN_KIND_BY_ID[pin.kind] || PIN_KIND_BY_ID.other;
  return html`
    <li class="pin-card${pin.mine ? ' pin-card--mine' : ''}" data-id="${pin.id}"${attr('aria-current', current ? 'true' : null)}>
      <div class="pin-card__head">
        ${avatar(pin.author.name)}
        <span class="pin-card__author">
          <strong>${pin.mine ? 'Вы' : pin.author.name}</strong>
          <span>${kind.label}</span>
        </span>
        <span class="pin-card__time">${pinTimeBadge(pin)}</span>
      </div>
      <div>
        <button type="button" class="pin-card__open" data-action="select" data-id="${pin.id}">
          <span class="pin-card__title">${pin.title}</span>
        </button>
        ${pin.note ? html`<p class="pin-card__note">${pin.note}</p>` : ''}
      </div>
      <span class="pin-card__place">${icon('map-pin')}${pin.placeLabel}</span>
      <div class="pin-card__foot">
        ${pinMeter(pin)}
        ${pin.mine
          ? html`<span class="badge badge--inverse">Ваша метка</span>`
          : html`<button type="button" class="btn btn--sm btn--accent-soft" data-action="pin-going" data-id="${pin.id}" aria-pressed="${pin.joined}">
              ${icon(pin.joined ? 'check' : 'plus', 'icon--sm')}${pin.joined ? 'Вы идёте' : 'Я приду'}
            </button>`}
      </div>
    </li>
  `;
}

export function collectionCard(collection, { count, cover, pressed }) {
  return html`
    <button type="button" class="collection-card" data-action="collection" data-id="${collection.id}" aria-pressed="${pressed}">
      ${cover ? media(cover, { ratio: '1' }) : html`<span class="media media--placeholder"></span>`}
      <span>
        <span class="collection-card__title">${collection.title}</span>
        <span class="collection-card__count">${countLabel(count, ['объект', 'объекта', 'объектов'])}</span>
      </span>
    </button>
  `;
}
