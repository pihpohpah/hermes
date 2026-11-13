/**
 * Страница места (#/place/:id) и события (#/event/:id) — один шаблон.
 * Колонка контента + липкая карточка практической информации с мини-картой.
 * На мобильном внизу закреплена панель главных действий.
 */

import { html, render, $, on, createDisposer } from '../core/dom.js';
import { store } from '../core/store.js';
import { icon } from '../ui/icon.js';
import { toast } from '../ui/toast.js';
import { openDialog } from '../ui/dialog.js';
import { getItem, getSimilar, getEventsAt, peekItem } from '../api/places.js';
import { categoryOf, SCENARIO_BY_ID, ACCESS, DIFFICULTY, BEACH } from '../data/taxonomy.js';
import { placeCard, media, dateTile, itemUrl } from '../ui/cards.js';
import { formatDistance, formatEventWhen, formatTime, formatDay } from '../core/format.js';
import { toggleRoute, isInRoute } from '../ui/route-store.js';
import { shareItem } from '../ui/share.js';
import { footer } from '../ui/footer.js';
import { notFoundMarkup } from './not-found.js';
import { Map as MapLibreMap, Marker } from '../../../vendor/maplibre-gl/maplibre-gl.mjs';
import { buildStyle } from '../map/style.js';

function mapLink(item) {
  return `#/map?s=${item.scenario}&place=${item.id}`;
}

function gallery(item) {
  const photos = item.photos || [];
  if (!photos.length) return html`<div class="detail__gallery detail__gallery--empty">${media(item, { ratio: '4 / 1' })}</div>`;
  return html`
    <div class="detail__gallery" data-count="${photos.length}">
      ${photos.map(
        (photo, index) => html`
          <button type="button" class="detail__photo" data-action="photo" data-index="${index}" aria-label="Открыть фото ${index + 1} из ${photos.length}: ${photo.alt}">
            ${media(item, { size: index === 0 ? 'lg' : 'sm', index, eager: index === 0 })}
          </button>
        `,
      )}
      ${photos.length > 1 ? html`<span class="detail__photo-count" aria-hidden="true">${icon('camera', 'icon--sm')}${photos.length}</span>` : ''}
    </div>
  `;
}

function infoRows(item) {
  const rows = [];
  if (item.type === 'event') {
    rows.push(['calendar', 'Когда', `${formatEventWhen(item.startsAt)}${item.endsAt ? `–${formatTime(item.endsAt)}` : ''}`]);
    rows.push(['map-pin', 'Где', html`${item.venue}<br /><span class="detail__muted">${item.address}</span>`]);
    rows.push(['ticket', 'Вход', item.price?.label]);
    rows.push(['users', 'Возраст', item.age]);
    rows.push(['megaphone', 'Организатор', item.organizer]);
  } else {
    rows.push(['map-pin', 'Адрес', item.address]);
    rows.push(['clock', 'Время', item.hours]);
    rows.push(['wallet', 'Стоимость', item.price?.label]);
    rows.push(['hourglass', 'Сколько заложить', item.duration]);
    rows.push(['sun', 'Сезон', item.season]);
    if (item.difficulty) rows.push(['mountain', 'Сложность', DIFFICULTY[item.difficulty]]);
    if (item.beach && item.beach !== 'none') rows.push(['waves', 'Берег', BEACH[item.beach]]);
    if (item.swimSeason) rows.push(['droplets', 'Купание', item.swimSeason]);
  }
  rows.push(['navigation', 'От центра', `${formatDistance(item.distanceKm)} по прямой`]);
  return html`<dl class="facts">${rows.filter((row) => row[2]).map(([i, label, value]) => html`<div class="fact">${icon(i)}<div><dt>${label}</dt><dd>${value}</dd></div></div>`)}</dl>`;
}

function safetyNote(item) {
  if (item.type === 'event' || item.scenario === 'city') return '';
  const text =
    item.scenario === 'sea'
      ? 'Погода на море меняется быстро. Перед выходом проверьте прогноз ветра и волны, а на воду берите жилет.'
      : 'В сопках бывают клещи с апреля по июль, а связь пропадает. Предупредите близких о маршруте и возьмите воду.';
  return html`
    <div class="alert alert--warning">
      ${icon('triangle-alert')}
      <div><strong class="alert__title">Перед поездкой</strong>${text} <a href="https://www.vl.ru/weather" target="_blank" rel="noopener">Погода на VL.ru</a></div>
    </div>
  `;
}

function eventRow(event) {
  return html`
    <li>
      <a class="event-row" href="${itemUrl(event)}">
        ${dateTile(event.startsAt)}
        <span><strong>${event.title}</strong><span>${formatEventWhen(event.startsAt)} · ${event.price?.label}</span></span>
        ${icon('chevron-right')}
      </a>
    </li>
  `;
}

/** Экранирование текста по RFC 5545: обратный слэш, запятая, точка с запятой, переводы строк. */
const icsText = (value = '') => String(value).replace(/[\\,;]/g, (ch) => `\\${ch}`).replace(/\r?\n/g, '\\n');

/** Строки длиннее 75 октетов переносятся с пробелом в начале продолжения. */
function icsFold(line) {
  const bytes = new TextEncoder().encode(line);
  if (bytes.length <= 75) return line;
  const parts = [];
  let current = '';
  for (const ch of line) {
    if (new TextEncoder().encode(current + ch).length > (parts.length ? 74 : 75)) {
      parts.push(current);
      current = '';
    }
    current += ch;
  }
  parts.push(current);
  return parts.join('\r\n ');
}

function icsFor(item) {
  const stamp = (iso) => new Date(iso).toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '');
  const end = item.endsAt || new Date(new Date(item.startsAt).getTime() + 2 * 3600000).toISOString();
  return [
    'BEGIN:VCALENDAR',
    'VERSION:2.0',
    'PRODID:-//VL.ru//Mesta//RU',
    'BEGIN:VEVENT',
    `UID:${item.id}@places.vl.ru`,
    `DTSTAMP:${stamp(new Date().toISOString())}`,
    `DTSTART:${stamp(item.startsAt)}`,
    `DTEND:${stamp(end)}`,
    `SUMMARY:${icsText(item.title)}`,
    `LOCATION:${icsText(`${item.venue}, ${item.address}`)}`,
    `DESCRIPTION:${icsText(item.short)}`,
    'END:VEVENT',
    'END:VCALENDAR',
  ]
    .map(icsFold)
    .join('\r\n');
}

function downloadIcs(item) {
  const blob = new Blob([icsFor(item)], { type: 'text/calendar;charset=utf-8' });
  const link = document.createElement('a');
  link.href = URL.createObjectURL(blob);
  link.download = `${item.id}.ics`;
  link.click();
  setTimeout(() => URL.revokeObjectURL(link.href), 1000);
  toast('Файл календаря скачан', { type: 'success' });
}

function openLightbox(item, start) {
  const photos = item.photos;
  let index = start;
  const body = () => {
    const photo = photos[index];
    return html`
      <figure class="lightbox">
        <img src="${photo.src}" alt="${photo.alt}" />
        <figcaption>
          <span>${photo.alt}</span>
          <span class="detail__muted">Фото: ${photo.author} · <a href="${photo.licenseUrl}" target="_blank" rel="noopener">${photo.license}</a> · <a href="${photo.source}" target="_blank" rel="noopener">источник</a></span>
        </figcaption>
      </figure>
    `;
  };
  const footerHtml = () =>
    photos.length > 1
      ? html`
          <button type="button" class="btn btn--outline" data-step="-1">${icon('chevron-left')}Назад</button>
          <span class="lightbox__counter tabular">${index + 1} из ${photos.length}</span>
          <button type="button" class="btn btn--outline" data-step="1">Дальше${icon('chevron-right')}</button>
        `
      : '';
  const dialog = openDialog({
    title: item.title,
    body: body(),
    footer: footerHtml(),
    wide: true,
    className: 'dialog--lightbox',
    onMount(el) {
      const step = (delta) => {
        index = (index + delta + photos.length) % photos.length;
        dialog.setContent({ body: body(), footer: footerHtml() });
      };
      el.querySelector('.dialog__footer')?.addEventListener('click', (event) => {
        const button = event.target.closest('[data-step]');
        if (button) step(Number(button.dataset.step));
      });
      el.onkeydown = (event) => {
        if (event.key === 'ArrowRight') step(1);
        if (event.key === 'ArrowLeft') step(-1);
      };
    },
  });
}

function createMiniMap(container, item) {
  try {
    const map = new MapLibreMap({
      container,
      style: buildStyle(),
      center: item.coords,
      zoom: item.scenario === 'city' ? 14 : 10.5,
      interactive: false,
      attributionControl: { compact: true },
    });
    const el = document.createElement('div');
    el.className = 'marker';
    el.dataset.scenario = item.type === 'event' ? 'timed' : item.scenario;
    el.innerHTML = `<svg class="icon" aria-hidden="true"><use href="assets/icons.svg#i-${categoryOf(item).icon}"></use></svg>`;
    new Marker({ element: el }).setLngLat(item.coords).addTo(map);
    return map;
  } catch (error) {
    console.warn('Мини-карта недоступна', error);
    container.hidden = true;
    return null;
  }
}

export const detailView = {
  title: 'Место',
  section: 'catalog',

  mount(target, ctx) {
    const disposer = createDisposer();
    let item = null;
    let miniMap = null;
    /** Ответы, пришедшие после ухода со страницы, ничего не рисуют. */
    let mounted = true;

    render(
      target,
      html`
        <div class="page container" aria-busy="true">
          <div class="skeleton skeleton--text" style="width: 12rem"></div>
          <div class="skeleton" style="height: 2.5rem; width: min(30rem, 80%); margin-top: var(--space-4)"></div>
          <div class="skeleton" style="aspect-ratio: 21 / 9; margin-top: var(--space-6); border-radius: var(--radius-lg)"></div>
        </div>
      `,
    );

    function actionButtons({ compact = false } = {}) {
      const inRoute = isInRoute(item.id);
      return html`
        <button type="button" class="btn btn--outline" data-action="route-toggle" aria-pressed="${inRoute}">
          ${icon(inRoute ? 'check' : 'route')}${inRoute ? 'В маршруте' : 'В маршрут'}
        </button>
        <a class="btn btn--primary" href="${mapLink(item)}">${icon('map')}${compact ? 'Карта' : 'На карте'}</a>
      `;
    }

    async function paint() {
      const [similar, eventsHere] = await Promise.all([getSimilar(item), item.type === 'place' ? getEventsAt(item.id) : []]);
      if (!mounted) return;
      const category = categoryOf(item);
      const scenario = SCENARIO_BY_ID[item.scenario];
      const place = item.placeId ? peekItem(item.placeId) : null;
      document.title = `${item.title} — Места VL.ru`;

      render(
        target,
        html`
          <article class="page detail" aria-labelledby="detail-title">
            <div class="container">
              <nav class="breadcrumbs" aria-label="Вы здесь">
                <ol>
                  <li><a href="#/catalog">Справочник</a></li>
                  <li><a href="#/catalog?s=${item.scenario}">${scenario.label}</a></li>
                  <li><a href="#/catalog?s=${item.scenario}${item.type === 'place' ? `&cat=${item.category}` : '&type=event'}">${item.type === 'event' ? 'События' : category.label}</a></li>
                </ol>
              </nav>

              <header class="detail__head">
                <div class="detail__heading">
                  <div class="detail__badges">
                    <span class="badge badge--${item.type === 'event' ? 'timed' : item.scenario}">${icon(category.icon)}${category.label}</span>
                    ${item.featured ? html`<span class="badge">${icon('star')}Стоит увидеть</span>` : ''}
                    ${item.type === 'event' ? html`<span class="badge badge--timed">${icon('calendar')}${formatDay(item.startsAt)}</span>` : ''}
                  </div>
                  <h1 class="detail__title" id="detail-title">${item.title}</h1>
                  <p class="detail__subtitle">
                    ${item.subtitle && item.subtitle !== item.district ? html`<span>${item.subtitle}</span>` : ''}
                    <span>${icon('map-pin', 'icon--sm')}${item.district}</span>
                    ${item.type === 'place' ? html`<span>${icon('clock', 'icon--sm')}${item.hours}</span>` : html`<span>${icon('clock', 'icon--sm')}${formatEventWhen(item.startsAt)}</span>`}
                  </p>
                </div>
                <div class="detail__actions">
                  <button type="button" class="icon-btn icon-btn--muted" data-action="share" aria-label="Поделиться" title="Поделиться">${icon('share-2')}</button>
                  ${actionButtons()}
                </div>
              </header>

              ${gallery(item)}

              <div class="detail__layout">
                <div class="detail__main">
                  <p class="detail__lead">${item.short}</p>
                  <div class="prose">${item.description.map((p) => html`<p>${p}</p>`)}</div>

                  ${item.facts?.length
                    ? html`
                        <section class="detail__section" aria-labelledby="facts-title">
                          <h2 class="detail__section-title" id="facts-title">Что важно знать</h2>
                          <ul class="checklist" role="list">${item.facts.map((fact) => html`<li>${icon('check', 'icon--sm')}${fact}</li>`)}</ul>
                        </section>
                      `
                    : ''}

                  ${item.howToGet
                    ? html`
                        <section class="detail__section" aria-labelledby="route-title">
                          <h2 class="detail__section-title" id="route-title">Как добраться</h2>
                          <p>${item.howToGet}</p>
                          <div class="chip-row chip-row--wrap" style="margin-top: var(--space-3)">
                            ${(item.access || []).map((a) => html`<span class="badge">${icon(ACCESS[a].icon)}${ACCESS[a].label}</span>`)}
                          </div>
                        </section>
                      `
                    : ''}

                  ${place
                    ? html`
                        <section class="detail__section" aria-labelledby="venue-title">
                          <h2 class="detail__section-title" id="venue-title">Где проходит</h2>
                          <a class="event-row" href="${itemUrl(place)}">
                            ${media(place, { ratio: '1' })}
                            <span><strong>${place.title}</strong><span>${categoryOf(place).label} · ${place.district}</span></span>
                            ${icon('chevron-right')}
                          </a>
                        </section>
                      `
                    : ''}

                  ${eventsHere.length
                    ? html`
                        <section class="detail__section" aria-labelledby="events-title">
                          <h2 class="detail__section-title" id="events-title">Скоро здесь</h2>
                          <ul class="event-list" role="list">${eventsHere.map(eventRow)}</ul>
                        </section>
                      `
                    : ''}

                  ${safetyNote(item)}

                  <footer class="detail__meta">
                    ${item.photos?.length
                      ? html`<p>Фото: ${item.photos.map((p, i) => html`${i ? ', ' : ''}<a href="${p.source}" target="_blank" rel="noopener">${p.author}</a> (${p.license})`)}. Wikimedia Commons.</p>`
                      : ''}
                    <p>Описание демонстрационное. Нашли неточность? <a href="#/add?fix=${item.id}">Предложите правку</a> — редакция проверит.</p>
                  </footer>
                </div>

                <aside class="detail__aside" aria-label="Практическая информация">
                  <div class="info-card">
                    ${item.type === 'event' ? html`<div class="info-card__date">${dateTile(item.startsAt)}<div><strong>${formatEventWhen(item.startsAt)}</strong><span>${item.price?.label} · ${item.age}</span></div></div>` : ''}
                    ${infoRows(item)}
                    <div class="info-card__map" data-mini-map></div>
                    <div class="info-card__actions">
                      ${item.type === 'event' ? html`<button type="button" class="btn btn--outline btn--block" data-action="ics">${icon('calendar-plus')}В календарь</button>` : ''}
                      <a class="btn btn--ghost btn--block" href="#/add?fix=${item.id}">${icon('pencil')}Предложить правку</a>
                    </div>
                  </div>
                </aside>
              </div>

              ${similar.length
                ? html`
                    <section class="section" aria-labelledby="similar-title">
                      <div class="section__head">
                        <h2 class="section__title" id="similar-title">Похожие ${item.scenario === 'city' ? 'в городе' : item.scenario === 'nature' ? 'на природе' : 'у моря'}</h2>
                        <a class="btn btn--ghost" href="#/catalog?s=${item.scenario}">Все ${icon('arrow-right', 'icon--sm')}</a>
                      </div>
                      <ul class="catalog__grid catalog__grid--similar" role="list">${similar.map((other) => html`<li>${placeCard(other, { inRoute: isInRoute(other.id) })}</li>`)}</ul>
                    </section>
                  `
                : ''}
            </div>
            <div class="detail__mobile-bar">
              <button type="button" class="icon-btn icon-btn--muted" data-action="share" aria-label="Поделиться">${icon('share-2')}</button>
              ${actionButtons({ compact: true })}
            </div>
          </article>
          ${footer()}
        `,
      );

      miniMap?.remove();
      const mini = $('[data-mini-map]', target);
      miniMap = createMiniMap(mini, item);
    }

    getItem(ctx.params.id)
      .then((loaded) => {
        if (!mounted) return;
        const expectedType = ctx.pattern.startsWith('/event') ? 'event' : 'place';
        if (loaded.type !== expectedType) {
          location.replace(itemUrl(loaded));
          return;
        }
        item = loaded;
        paint();
        window.scrollTo(0, 0);
      })
      .catch((error) => {
        if (!mounted) return;
        if (error.code !== 'not_found') console.error('Не удалось загрузить карточку', error);
        render(target, notFoundMarkup({ title: 'Такого места нет', text: 'Возможно, его убрали или в ссылке опечатка. Поищите в справочнике или на карте.' }));
      });

    disposer.add(
      on(target, 'click', '[data-action]', (event, el) => {
        const { action } = el.dataset;
        if (action === 'route-toggle') {
          const id = el.dataset.id || item.id;
          event.preventDefault();
          toggleRoute(id, peekItem(id)?.title);
        }
        if (action === 'share') shareItem(item);
        if (action === 'ics') downloadIcs(item);
        if (action === 'photo') openLightbox(item, Number(el.dataset.index));
      }),
    );

    disposer.add(
      store.subscribe(() => {
        if (!item) return;
        target.querySelectorAll('[data-action="route-toggle"]').forEach((button) => {
          const id = button.dataset.id || item.id;
          const inRoute = isInRoute(id);
          button.setAttribute('aria-pressed', String(inRoute));
          if (!button.dataset.id) {
            render(button, html`${icon(inRoute ? 'check' : 'route')}${inRoute ? 'В маршруте' : 'В маршрут'}`);
          } else {
            button.setAttribute('aria-label', `${inRoute ? 'Убрать из маршрута' : 'Добавить в маршрут'}: ${peekItem(id)?.title || ''}`);
            render(button, icon(inRoute ? 'check' : 'route', 'icon--sm'));
          }
        });
      }, ['route']),
    );

    return {
      unmount() {
        mounted = false;
        disposer.dispose();
        miniMap?.remove();
      },
    };
  },
};
