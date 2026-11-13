/**
 * Добавление места, события или правки (#/add) и экран успеха (#/add/success).
 *
 * Три шага: «Что» → «Подробности» → «Отправка». Шаг хранится в URL (step=1..3), поэтому работает
 * кнопка «Назад» браузера. Черновик держится в памяти модуля до отправки или ухода со страницы.
 * Предложение уходит редакции; публикации без проверки нет.
 *
 * Параметры: type (place | event | fix), fix (id исправляемого объекта), scenario.
 * Без scenario новый черновик наследует глобальный режим; дальше раздел независим от шапки.
 */

import { html, render, $, $$, on, createDisposer } from '../core/dom.js';
import { store } from '../core/store.js';
import { navigate, setQuery } from '../core/router.js';
import { icon } from '../ui/icon.js';
import { toast } from '../ui/toast.js';
import { SCENARIOS, SCENARIO_BY_ID, CATEGORIES, EVENT_CATEGORIES, categoryOf } from '../data/taxonomy.js';
import { getItem } from '../api/places.js';
import { submitProposal } from '../api/submissions.js';
import { openAuthDialog } from '../ui/auth-dialog.js';
import { media } from '../ui/cards.js';
import { footer } from '../ui/footer.js';
import { CITY_CENTER } from '../core/geo.js';
import { Map as MapLibreMap } from '../../../vendor/maplibre-gl/maplibre-gl.mjs';
import { buildStyle } from '../map/style.js';

const STEPS = ['Что', 'Подробности', 'Отправка'];
const MAX_PHOTOS = 5;
const DESCRIPTION_MIN = 30;

const TYPES = [
  { id: 'place', title: 'Место', text: 'Постоянная точка: бухта, кофейня, смотровая', icon: 'map-pin' },
  { id: 'event', title: 'Событие', text: 'С датой: концерт, поход, маркет', icon: 'calendar' },
  { id: 'fix', title: 'Правка', text: 'Неточность в существующей карточке', icon: 'pencil' },
];

const FIX_REASONS = ['Закрылось навсегда', 'Неверный адрес или точка', 'Изменились часы', 'Изменилась цена', 'Ошибка в описании', 'Плохие фото', 'Другое'];

let draft = null;

function emptyDraft(query) {
  return {
    type: TYPES.some((t) => t.id === query.type) ? query.type : query.fix ? 'fix' : 'place',
    scenario: SCENARIO_BY_ID[query.scenario] ? query.scenario : store.get().scenario,
    category: '',
    fixId: query.fix || '',
    fixReasons: [],
    title: '',
    address: '',
    coords: null,
    date: '',
    time: '',
    description: '',
    hours: '',
    price: '',
    link: '',
    photos: [],
    contact: '',
    consent: false,
  };
}

function setError(form, name, message) {
  const control = form.elements[name];
  const node = control instanceof RadioNodeList ? control[0] : control;
  const errorId = node?.getAttribute('aria-describedby')?.split(' ')[0];
  const error = errorId ? $(`#${errorId}`, form) : null;
  if (node && !(control instanceof RadioNodeList)) node.setAttribute('aria-invalid', message ? 'true' : 'false');
  if (error) error.textContent = message || '';
  return message ? node : null;
}

export const addView = {
  title: 'Добавить',
  section: null,

  mount(target, ctx) {
    const disposer = createDisposer();
    let fixItem = null;
    let pickerMap = null;
    let step = 1;

    if (!draft || (ctx.query.fix && draft.fixId !== ctx.query.fix) || (!ctx.query.step && !ctx.query.fix)) {
      draft?.photos.forEach((photo) => URL.revokeObjectURL(photo.url));
      draft = emptyDraft(ctx.query);
    }
    let mounted = true;

    function categoryOptions() {
      if (draft.type === 'event') return Object.entries(EVENT_CATEGORIES).filter(([id]) => id !== 'party').map(([id, c]) => ({ id, ...c }));
      return CATEGORIES[draft.scenario];
    }

    function stepper() {
      return html`
        <ol class="steps" aria-label="Шаги">
          ${STEPS.map(
            (label, i) => html`
              <li class="steps__item${i + 1 < step ? ' is-done' : ''}"${i + 1 === step ? html` aria-current="step"` : ''}>
                <span class="steps__bar"></span><span>${i + 1}. ${label}</span>
              </li>
            `,
          )}
        </ol>
      `;
    }

    /* ── Шаг 1 ── */

    function stepWhat() {
      return html`
        <fieldset class="fieldset">
          <legend>Что хотите добавить</legend>
          <div class="choice-group">
            ${TYPES.map(
              (t) => html`
                <label class="choice">
                  <input type="radio" name="type" value="${t.id}" ${draft.type === t.id ? 'checked' : ''} />
                  <span class="choice__icon">${icon(t.icon)}</span>
                  <span><span class="choice__title">${t.title}</span><span class="choice__text">${t.text}</span></span>
                </label>
              `,
            )}
          </div>
        </fieldset>

        ${draft.type === 'fix'
          ? html`
              ${fixItem
                ? html`
                    <div class="fix-target">
                      ${media(fixItem, { ratio: '1' })}
                      <div><span class="detail__muted">Исправляем</span><strong>${fixItem.title}</strong><span class="detail__muted">${categoryOf(fixItem).label} · ${fixItem.district}</span></div>
                    </div>
                  `
                : html`<div class="alert alert--neutral">${icon('info')}<span>Откройте карточку места и нажмите «Предложить правку» — тогда мы поймём, что именно исправить. Или опишите ниже, о каком месте речь.</span></div>`}
              <fieldset class="fieldset">
                <legend>Что не так</legend>
                <div class="chip-row chip-row--wrap">
                  ${FIX_REASONS.map((reason) => html`<label class="chip chip--lg"><input type="checkbox" name="fixReasons" value="${reason}" ${draft.fixReasons.includes(reason) ? 'checked' : ''} aria-describedby="err-fixReasons" />${reason}</label>`)}
                </div>
                <p class="field__error" id="err-fixReasons" role="alert"></p>
              </fieldset>
            `
          : html`
              <fieldset class="fieldset">
                <legend>Раздел</legend>
                <div class="segmented segmented--lg" role="radiogroup">
                  ${SCENARIOS.map((s) => html`<label class="segmented__item" data-scenario="${s.id}"><input type="radio" name="scenario" value="${s.id}" ${draft.scenario === s.id ? 'checked' : ''} />${icon(s.icon)}${s.label}</label>`)}
                </div>
              </fieldset>
              <fieldset class="fieldset">
                <legend>Категория</legend>
                <div class="chip-row chip-row--wrap">
                  ${categoryOptions().map((c) => html`<label class="chip chip--lg"><input type="radio" name="category" value="${c.id}" ${draft.category === c.id ? 'checked' : ''} aria-describedby="err-category" />${icon(c.icon)}${c.label}</label>`)}
                </div>
                <p class="field__error" id="err-category" role="alert"></p>
              </fieldset>
            `}
      `;
    }

    /* ── Шаг 2 ── */

    function photosField() {
      return html`
        <div class="field">
          <span class="field__label" id="photos-label">Фото <span class="optional">— до ${MAX_PHOTOS}, необязательно</span></span>
          <div class="upload" role="group" aria-labelledby="photos-label">
            ${draft.photos.map(
              (photo, index) => html`
                <div class="upload__item">
                  <img src="${photo.url}" alt="Фото ${index + 1}: ${photo.file.name}" />
                  <button type="button" class="icon-btn icon-btn--sm icon-btn--floating icon-btn--round" data-action="remove-photo" data-index="${index}" aria-label="Удалить фото ${photo.file.name}">${icon('x')}</button>
                </div>
              `,
            )}
            ${draft.photos.length < MAX_PHOTOS
              ? html`<label class="upload__drop" data-drop>${icon('image-plus', 'icon--lg')}Добавить<input type="file" name="photos" accept="image/*" multiple /></label>`
              : ''}
          </div>
          <p class="field__hint">Свои снимки или с разрешения автора. JPG, PNG или HEIC до 10 МБ.</p>
        </div>
      `;
    }

    function stepDetails() {
      if (draft.type === 'fix') {
        return html`
          ${fixItem
            ? ''
            : html`
                <div class="field">
                  <label class="field__label" for="f-title">Какое место или событие</label>
                  <input class="input" id="f-title" name="title" value="${draft.title}" maxlength="80" aria-describedby="err-title" />
                  <p class="field__error" id="err-title" role="alert"></p>
                </div>
              `}
          <div class="field">
            <label class="field__label" for="f-description">Как правильно</label>
            <textarea class="textarea" id="f-description" name="description" maxlength="1000" placeholder="Например: с ноября работает до 18:00, вход теперь со двора" aria-describedby="err-description f-description-count">${draft.description}</textarea>
            <p class="field__error" id="err-description" role="alert"></p>
            <span class="field__counter" id="f-description-count">${draft.description.length} / 1000</span>
          </div>
          <div class="field">
            <label class="field__label" for="f-link">Источник <span class="optional">— необязательно</span></label>
            <input class="input" id="f-link" name="link" type="url" inputmode="url" value="${draft.link}" placeholder="Ссылка на сайт, соцсеть или новость" />
          </div>
          ${photosField()}
        `;
      }

      const isEvent = draft.type === 'event';
      return html`
        <div class="field">
          <label class="field__label" for="f-title">Название</label>
          <input class="input" id="f-title" name="title" value="${draft.title}" maxlength="80" placeholder="${isEvent ? 'Например, джем-сейшн в «Заре»' : 'Например, бухта Шамора'}" aria-describedby="err-title" required />
          <p class="field__error" id="err-title" role="alert"></p>
        </div>

        ${isEvent
          ? html`
              <div class="form-row">
                <div class="field">
                  <label class="field__label" for="f-date">Дата</label>
                  <input class="input" id="f-date" name="date" type="date" value="${draft.date}" aria-describedby="err-date" required />
                  <p class="field__error" id="err-date" role="alert"></p>
                </div>
                <div class="field">
                  <label class="field__label" for="f-time">Начало</label>
                  <input class="input" id="f-time" name="time" type="time" value="${draft.time}" aria-describedby="err-time" required />
                  <p class="field__error" id="err-time" role="alert"></p>
                </div>
              </div>
            `
          : ''}

        <div class="field">
          <label class="field__label" for="f-address">${isEvent ? 'Где проходит' : 'Адрес или ориентир'}</label>
          <div class="input-with-action">
            <input class="input" id="f-address" name="address" value="${draft.address}" maxlength="120" placeholder="Улица и дом или понятный ориентир" aria-describedby="err-address" required />
            <button type="button" class="btn btn--outline" data-action="toggle-picker" aria-expanded="${Boolean(draft.coords)}" aria-controls="add-map">${icon('map-pin')}${draft.coords ? 'Точка выбрана' : 'На карте'}</button>
          </div>
          <p class="field__error" id="err-address" role="alert"></p>
          <div class="add-map" id="add-map" ${draft.coords ? '' : 'hidden'}>
            <div class="add-map__canvas" data-picker-map></div>
            <div class="map-picker"><div class="map-picker__pin"><svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" stroke="#fff" stroke-width="1.5" d="M12 22s7-6.2 7-12a7 7 0 1 0-14 0c0 5.8 7 12 7 12Z"/><circle cx="12" cy="10" r="2.6" fill="#fff"/></svg><span class="map-picker__shadow"></span></div></div>
            <p class="add-map__hint">${icon('info', 'icon--sm')}Двигайте карту, чтобы метка встала на место</p>
          </div>
        </div>

        <div class="field">
          <label class="field__label" for="f-description">Описание</label>
          <textarea class="textarea" id="f-description" name="description" maxlength="1000" placeholder="Чем интересно, когда лучше идти, что важно знать заранее" aria-describedby="err-description f-description-count" required>${draft.description}</textarea>
          <p class="field__error" id="err-description" role="alert"></p>
          <span class="field__counter" id="f-description-count">${draft.description.length} / 1000</span>
        </div>

        <div class="form-row">
          ${isEvent
            ? ''
            : html`
                <div class="field">
                  <label class="field__label" for="f-hours">Часы работы <span class="optional">— необязательно</span></label>
                  <input class="input" id="f-hours" name="hours" value="${draft.hours}" placeholder="Круглосуточно" />
                </div>
              `}
          <div class="field">
            <label class="field__label" for="f-price">Стоимость <span class="optional">— необязательно</span></label>
            <input class="input" id="f-price" name="price" value="${draft.price}" placeholder="Бесплатно или от 500 ₽" />
          </div>
        </div>

        <div class="field">
          <label class="field__label" for="f-link">Сайт или соцсеть <span class="optional">— необязательно</span></label>
          <input class="input" id="f-link" name="link" type="url" inputmode="url" value="${draft.link}" placeholder="https://" />
        </div>

        ${photosField()}
      `;
    }

    /* ── Шаг 3 ── */

    function summary() {
      const type = TYPES.find((t) => t.id === draft.type);
      const rows = [
        ['Что', type.title],
        draft.type !== 'fix' ? ['Раздел', SCENARIO_BY_ID[draft.scenario].label] : null,
        draft.type !== 'fix' ? ['Категория', categoryOptions().find((c) => c.id === draft.category)?.label] : null,
        draft.type === 'fix' ? ['Объект', fixItem?.title || draft.title] : ['Название', draft.title],
        draft.type === 'fix' ? ['Что не так', draft.fixReasons.join(', ')] : ['Где', draft.address],
        draft.date ? ['Когда', `${new Date(`${draft.date}T${draft.time || '00:00'}`).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' })}, ${draft.time}`] : null,
        ['Фото', draft.photos.length ? `${draft.photos.length} шт.` : 'Нет'],
      ].filter(Boolean);
      return html`
        <div class="summary-card">
          <div class="summary-card__head">
            <h3>Проверьте перед отправкой</h3>
            <button type="button" class="link-btn" data-action="goto" data-step="1">Изменить</button>
          </div>
          <dl class="summary-card__list">${rows.map(([label, value]) => html`<div><dt>${label}</dt><dd>${value || '—'}</dd></div>`)}</dl>
          ${draft.description ? html`<p class="summary-card__text">${draft.description}</p>` : ''}
        </div>
      `;
    }

    function stepSend() {
      const { user } = store.get();
      return html`
        ${summary()}
        ${user
          ? html`<div class="alert alert--success">${icon('circle-check')}<span>Вы вошли как <strong>${user.name}</strong>. Статус проверки появится в профиле, а если понадобится уточнение — напишем в SMS.</span></div>`
          : html`
              <div class="field">
                <label class="field__label" for="f-contact">Как с вами связаться</label>
                <input class="input" id="f-contact" name="contact" value="${draft.contact}" autocomplete="email" placeholder="Почта или телефон" aria-describedby="err-contact f-contact-hint" required />
                <p class="field__error" id="err-contact" role="alert"></p>
                <p class="field__hint" id="f-contact-hint">Только для вопросов редакции, публиковать не будем. <button type="button" class="link-btn" data-action="login">Или войдите</button> — тогда статус будет в профиле.</p>
              </div>
            `}
        <label class="check">
          <input type="checkbox" name="consent" ${draft.consent ? 'checked' : ''} aria-describedby="err-consent" />
          <span class="check__text">Информация достоверна, а фото можно публиковать<small>Редакция может сократить текст и поправить детали</small></span>
        </label>
        <p class="field__error" id="err-consent" role="alert"></p>
      `;
    }

    /* ── Отрисовка ── */

    function paint() {
      const titles = {
        place: 'Добавить место',
        event: 'Добавить событие',
        fix: 'Предложить правку',
      };
      render(
        target,
        html`
          <div class="page add">
            <div class="container add__layout">
              <div class="add__main">
                <header class="page__head">
                  <h1 class="page__title">${titles[draft.type]}</h1>
                  <p class="page__lead">${draft.type === 'fix' ? 'Расскажите, что изменилось, — редакция проверит и обновит карточку.' : 'Расскажите о том, что знаете сами. Редакция проверит и опубликует — обычно за один-два дня.'}</p>
                </header>
                ${stepper()}
                <form class="add__form" novalidate data-step="${step}">
                  <h2 class="add__step-title" tabindex="-1">${STEPS[step - 1]}</h2>
                  ${step === 1 ? stepWhat() : step === 2 ? stepDetails() : stepSend()}
                  <div class="add__nav">
                    ${step > 1 ? html`<button type="button" class="btn btn--ghost btn--lg" data-action="goto" data-step="${step - 1}">${icon('arrow-left')}Назад</button>` : html`<a class="btn btn--ghost btn--lg" href="#/map">Отмена</a>`}
                    <button type="submit" class="btn btn--primary btn--lg">${step < 3 ? html`Дальше${icon('arrow-right')}` : 'Отправить на проверку'}</button>
                  </div>
                </form>
              </div>
              <aside class="add__aside" aria-label="Подсказки">
                <div class="tips">
                  <h2 class="tips__title">${icon('info')}Как попасть в справочник</h2>
                  <ul class="tips__list" role="list">
                    <li>Пишите о том, где были сами: так описание будет точным.</li>
                    <li>Практичные детали важнее эпитетов: как добраться, когда лучше, что взять.</li>
                    <li>Не добавляйте частную территорию и места, куда закрыт доступ.</li>
                    <li>Реклама и платные размещения сюда не попадают.</li>
                  </ul>
                  <a href="#/rules?section=moderation">Как мы проверяем места</a>
                </div>
              </aside>
            </div>
          </div>
          ${footer()}
        `,
      );
      if (step === 2 && draft.coords) mountPicker();
    }

    /* ── Карта выбора точки ── */

    function mountPicker() {
      const wrap = $('#add-map', target);
      if (!wrap) return;
      wrap.hidden = false;
      if (pickerMap) return;
      const start = draft.coords || fixItem?.coords || CITY_CENTER;
      try {
        pickerMap = new MapLibreMap({
          container: $('[data-picker-map]', wrap),
          style: buildStyle(),
          center: start,
          zoom: 13,
          attributionControl: { compact: true },
          dragRotate: false,
        });
        pickerMap.on('moveend', () => {
          draft.coords = pickerMap.getCenter().toArray();
        });
        draft.coords = start;
      } catch (error) {
        console.warn('Карта выбора точки недоступна', error);
        wrap.hidden = true;
        toast('Карта недоступна — укажите адрес текстом', { type: 'danger' });
      }
    }

    function destroyPicker() {
      pickerMap?.remove();
      pickerMap = null;
    }

    /* ── Чтение и проверка ── */

    function read(form) {
      const value = (name) => (form.elements[name] ? form.elements[name].value : undefined);
      if (step === 1) {
        draft.type = form.querySelector('input[name="type"]:checked')?.value || draft.type;
        draft.scenario = form.querySelector('input[name="scenario"]:checked')?.value || draft.scenario;
        draft.category = form.querySelector('input[name="category"]:checked')?.value || '';
        draft.fixReasons = $$('input[name="fixReasons"]:checked', form).map((input) => input.value);
      }
      if (step === 2) {
        ['title', 'address', 'date', 'time', 'description', 'hours', 'price', 'link'].forEach((name) => {
          if (value(name) !== undefined) draft[name] = value(name);
        });
      }
      if (step === 3) {
        if (form.elements.contact) draft.contact = form.elements.contact.value;
        draft.consent = form.elements.consent.checked;
      }
    }

    function validate(form) {
      const problems = [];
      const check = (name, message) => problems.push(setError(form, name, message));
      if (step === 1) {
        if (draft.type === 'fix') check('fixReasons', draft.fixReasons.length ? '' : 'Отметьте хотя бы один пункт');
        else check('category', draft.category ? '' : 'Выберите категорию');
      }
      if (step === 2) {
        if (form.elements.title) check('title', draft.title.trim().length >= 3 ? '' : 'Напишите название — хотя бы три буквы');
        if (form.elements.address) check('address', draft.address.trim().length >= 3 || draft.coords ? '' : 'Укажите адрес, ориентир или точку на карте');
        if (form.elements.date) check('date', draft.date ? '' : 'Выберите дату');
        if (form.elements.time) check('time', draft.time ? '' : 'Укажите время начала');
        const min = draft.type === 'fix' ? 10 : DESCRIPTION_MIN;
        check('description', draft.description.trim().length >= min ? '' : `Добавьте пару предложений — хотя бы ${min} символов`);
      }
      if (step === 3) {
        if (form.elements.contact) {
          const contact = draft.contact.trim();
          const ok = /\S+@\S+\.\S+/.test(contact) || contact.replace(/\D/g, '').length >= 10;
          check('contact', ok ? '' : 'Укажите почту или телефон, чтобы редакция могла уточнить детали');
        }
        check('consent', draft.consent ? '' : 'Подтвердите, что информация достоверна');
      }
      const first = problems.find(Boolean);
      first?.focus();
      return !first;
    }

    async function submit(form) {
      const button = $('button[type="submit"]', form);
      button.classList.add('is-loading');
      try {
        const submission = await submitProposal({
          ...draft,
          photos: draft.photos.map((p) => p.file),
          fixTitle: fixItem?.title,
        });
        draft.photos.forEach((p) => URL.revokeObjectURL(p.url));
        draft = null;
        navigate('/add/success', { id: submission.id, type: submission.type });
      } catch (error) {
        console.error('Не удалось отправить предложение', error);
        toast('Не получилось отправить. Проверьте соединение и попробуйте ещё раз', { type: 'danger' });
      } finally {
        button.classList.remove('is-loading');
      }
    }

    /* ── События ── */

    disposer.add(
      on(target, 'submit', 'form.add__form', (event, form) => {
        event.preventDefault();
        read(form);
        if (!validate(form)) return;
        if (step < 3) setQuery({ step: step + 1 });
        else submit(form);
      }),
    );

    disposer.add(
      on(target, 'change', 'input[name="type"], input[name="scenario"]', (event, input) => {
        const form = input.form;
        read(form);
        if (input.name === 'scenario' || input.name === 'type') draft.category = '';
        if (draft.type === 'fix' && !fixItem) setQuery({ type: 'fix' });
        else paint();
        $(`input[name="${input.name}"][value="${input.value}"]`, target)?.focus();
      }),
    );

    disposer.add(
      on(target, 'input', '#f-description', (event, textarea) => {
        $('#f-description-count', target).textContent = `${textarea.value.length} / 1000`;
      }),
    );

    disposer.add(
      on(target, 'change', 'input[name="photos"]', (event, input) => {
        const form = input.form;
        read(form);
        const files = Array.from(input.files).filter((file) => file.type.startsWith('image/'));
        const room = MAX_PHOTOS - draft.photos.length;
        if (files.length > room) toast(`Можно добавить ещё ${room}. Лишние фото не загрузили`, { type: 'danger' });
        files.slice(0, room).forEach((file) => draft.photos.push({ file, url: URL.createObjectURL(file) }));
        destroyPicker();
        paint();
      }),
    );

    disposer.add(
      on(target, 'click', '[data-action]', (event, el) => {
        const { action } = el.dataset;
        const form = $('form.add__form', target);
        if (action === 'goto') {
          read(form);
          setQuery({ step: Number(el.dataset.step) === 1 ? '' : el.dataset.step });
        }
        if (action === 'remove-photo') {
          read(form);
          const [removed] = draft.photos.splice(Number(el.dataset.index), 1);
          URL.revokeObjectURL(removed.url);
          destroyPicker();
          paint();
        }
        if (action === 'toggle-picker') {
          const wrap = $('#add-map', target);
          if (wrap.hidden) {
            mountPicker();
            el.setAttribute('aria-expanded', 'true');
            render(el, html`${icon('map-pin')}Точка выбрана`);
          } else {
            wrap.hidden = true;
            draft.coords = null;
            destroyPicker();
            el.setAttribute('aria-expanded', 'false');
            render(el, html`${icon('map-pin')}На карте`);
          }
        }
        if (action === 'login') {
          read(form);
          openAuthDialog({ reason: 'Войдите, чтобы следить за статусом предложения в профиле.' }).then((user) => {
            if (user) paint();
          });
        }
      }),
    );

    disposer.add(
      on(target, 'dragover', '[data-drop]', (event, drop) => {
        event.preventDefault();
        drop.classList.add('is-dragover');
      }),
    );
    disposer.add(on(target, 'dragleave', '[data-drop]', (event, drop) => drop.classList.remove('is-dragover')));
    disposer.add(
      on(target, 'drop', '[data-drop]', (event, drop) => {
        event.preventDefault();
        const input = $('input[type="file"]', drop);
        input.files = event.dataTransfer.files;
        input.dispatchEvent(new Event('change', { bubbles: true }));
      }),
    );

    let firstApply = true;

    function apply(query) {
      const requested = Math.min(3, Math.max(1, Number(query.step) || 1));
      // Нельзя перепрыгнуть шаг через адресную строку
      step = requested > 1 && !(draft.category || draft.fixReasons.length) ? 1 : requested;
      if (step === 3 && !draft.description) step = 2;
      destroyPicker();
      paint();
      // При переходе между шагами фокус уходит на заголовок шага, при первом открытии — остаётся на заголовке страницы
      if (!firstApply) $('.add__step-title', target)?.focus();
      firstApply = false;
      window.scrollTo(0, 0);
    }

    if (draft.fixId) {
      getItem(draft.fixId)
        .then((item) => {
          if (!mounted) return;
          fixItem = item;
          draft.scenario = item.scenario;
          apply(ctx.query);
        })
        .catch((error) => {
          if (!mounted) return;
          console.warn('Исправляемый объект не найден, форма открыта как новая правка', error);
          draft.fixId = '';
          apply(ctx.query);
        });
    } else {
      apply(ctx.query);
    }

    return {
      update(next) {
        apply(next.query);
      },
      unmount() {
        mounted = false;
        disposer.dispose();
        destroyPicker();
      },
    };
  },
};

export const successView = {
  title: 'Отправлено',
  section: null,
  mount(target, ctx) {
    const { user } = store.get();
    const isFix = ctx.query.type === 'fix';
    render(
      target,
      html`
        <div class="page container">
          <div class="state-page">
            <span class="state-page__icon">${icon('circle-check', 'icon--xl')}</span>
            <h1 class="state-page__title">${isFix ? 'Спасибо за правку' : 'Спасибо, отправили на проверку'}</h1>
            <p class="state-page__text">
              Редакция посмотрит в течение одного-двух рабочих дней.
              ${user ? 'Статус появится в профиле, в разделе «Мои предложения».' : 'Если понадобится уточнение, напишем на указанный контакт.'}
            </p>
            <div class="state-page__actions">
              <a class="btn btn--primary btn--lg" href="#/map">${icon('map')}Вернуться на карту</a>
              ${user ? html`<a class="btn btn--outline btn--lg" href="#/profile?tab=submissions">Мои предложения</a>` : html`<a class="btn btn--outline btn--lg" href="#/add">Добавить ещё</a>`}
            </div>
            <p class="state-page__note">Это прототип: данные никуда не отправляются.</p>
          </div>
        </div>
        ${footer()}
      `,
    );
  },
};
