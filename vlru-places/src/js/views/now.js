/**
 * Экран «Сейчас» (#/now): быстрые метки от жителей поверх той же карты.
 *
 * Параметры URL:
 *   kind   — фильтр по типу метки
 *   pin    — выбранная метка (превью)
 *   create — шаг создания: place (выбор точки на карте) | details (форма)
 */

import { html, render, fragment, $, $$, on, createDisposer, isMobileLayout, debounce } from '../core/dom.js';
import { store } from '../core/store.js';
import { setQuery } from '../core/router.js';
import { icon } from '../ui/icon.js';
import { toast } from '../ui/toast.js';
import { confirmDialog, openDialog } from '../ui/dialog.js';
import { withAuth } from '../ui/auth-dialog.js';
import { PIN_KINDS, PIN_KIND_BY_ID } from '../data/taxonomy.js';
import { listPins, getPin, setGoing, createPin, deletePin, reportPin } from '../api/pins.js';
import { pinCard, pinMeter, pinTimeBadge, avatar, participantNames } from '../ui/cards.js';
import { countLabel, formatAgo, formatDistance, formatDuration } from '../core/format.js';
import { distanceFromCenter } from '../core/geo.js';
import { shareLink } from '../ui/share.js';
import { rulesBody } from './rules.js';

const INTRO_KEY = 'vlru-places:now-intro-hidden';

const START_OPTIONS = [
  { value: 0, label: 'Сейчас' },
  { value: 15, label: 'Через 15 мин' },
  { value: 30, label: 'Через 30 мин' },
  { value: 60, label: 'Через час' },
  { value: 120, label: 'Через 2 часа' },
];

const DURATION_OPTIONS = [
  { value: 60, label: '1 час' },
  { value: 120, label: '2 часа' },
  { value: 180, label: '3 часа' },
];

const introHidden = () => {
  try {
    return sessionStorage.getItem(INTRO_KEY) === '1';
  } catch (error) {
    console.warn('sessionStorage недоступен', error);
    return false;
  }
};

export function createNowPanel(stage) {
  const { els, map } = stage;
  const disposer = createDisposer();
  let state = {};
  let pins = [];
  let kindsSlot = null;
  let timer = null;
  let draft = null;
  /** false после leave(): поздние ответы не рисуют поверх другого экрана. */
  let active = false;
  let requestId = 0;

  /* ── Шапка ── */

  function paintHead() {
    render(
      els.head,
      html`
        <div class="map-panel__title-row">
          <div>
            <h1 class="map-panel__title now-title"><span class="live-dot" aria-hidden="true"></span>Сейчас</h1>
            <p class="map-panel__subtitle" data-now-count>Метки от жителей</p>
          </div>
          <button type="button" class="btn btn--accent" data-action="create">${icon('plus')}Метка</button>
        </div>
        <div data-slot-home="kinds"></div>
      `,
    );
    kindsSlot = fragment(html`<div class="chip-row" role="group" aria-label="Тип метки" data-top-slot="kinds"></div>`);
    $('[data-slot-home="kinds"]', els.head).append(kindsSlot);
  }

  function paintKinds() {
    const counts = {};
    pins.forEach((pin) => {
      counts[pin.kind] = (counts[pin.kind] || 0) + 1;
    });
    render(
      kindsSlot,
      html`
        <button type="button" class="chip" data-action="kind" data-id="" aria-pressed="${!state.kind}">Все</button>
        ${PIN_KINDS.map(
          (kind) => html`
            <button type="button" class="chip" data-action="kind" data-id="${kind.id}" aria-pressed="${state.kind === kind.id}">
              ${icon(kind.icon)}${kind.label}${counts[kind.id] ? html`<span class="chip__count">${counts[kind.id]}</span>` : ''}
            </button>
          `,
        )}
      `,
    );
  }

  function paintFoot() {
    render(els.foot, html`<span>Метка исчезает, когда встреча закончилась</span><button type="button" class="link-btn" data-action="rules">Правила</button>`);
    els.foot.hidden = false;
  }

  /* ── Список ── */

  function visiblePins() {
    return pins.filter((pin) => !state.kind || pin.kind === state.kind);
  }

  function paintList() {
    const list = visiblePins();
    const count = $('[data-now-count]', els.head);
    if (count) count.textContent = pins.length ? `${countLabel(pins.length, ['метка', 'метки', 'меток'])} от жителей` : 'Метки от жителей';

    const intro = introHidden()
      ? ''
      : html`
          <div class="now-intro">
            ${icon('megaphone')}
            <p>Позовите людей на футбол, прогулку или джем. Метку видят все, и она исчезает, когда встреча закончилась. <button type="button" class="link-btn" data-action="rules">Правила меток</button></p>
            <button type="button" class="icon-btn icon-btn--sm" data-action="hide-intro" aria-label="Скрыть подсказку">${icon('x')}</button>
          </div>
        `;

    if (!list.length) {
      render(
        els.body,
        html`
          <div class="map-panel__list map-panel__list--cards">${intro}</div>
          <div class="empty">
            <span class="empty__icon">${icon('radio', 'icon--lg')}</span>
            <h2 class="empty__title">${state.kind ? 'Таких меток пока нет' : 'Пока тихо'}</h2>
            <p class="empty__text">Поставьте метку первым — соседи увидят её на карте.</p>
            <div class="empty__actions"><button type="button" class="btn btn--accent" data-action="create">${icon('plus')}Поставить метку</button></div>
          </div>
        `,
      );
      return;
    }

    render(
      els.body,
      html`
        <ul class="map-panel__list map-panel__list--cards" role="list">
          ${intro ? html`<li>${intro}</li>` : ''} ${list.map((pin) => pinCard(pin, { current: pin.id === state.pin }))}
        </ul>
      `,
    );
  }

  /** → true, если ответ актуален. */
  async function loadPins() {
    const id = ++requestId;
    try {
      const next = await listPins();
      if (id !== requestId || !active) return false;
      pins = next;
      map.setPins(pins);
      paintKinds();
      const count = $('[data-now-count]', els.head);
      if (count) count.textContent = pins.length ? `${countLabel(pins.length, ['метка', 'метки', 'меток'])} от жителей` : 'Метки от жителей';
      if (!state.pin && !state.create) paintList();
      return true;
    } catch (error) {
      console.error('Не удалось загрузить метки', error);
      if (id !== requestId || !active) return false;
      render(els.body, html`<div class="empty"><span class="empty__icon">${icon('wifi-off', 'icon--lg')}</span><h2 class="empty__title">Метки не загрузились</h2><div class="empty__actions"><button type="button" class="btn btn--primary" data-action="reload">Повторить</button></div></div>`);
      return false;
    }
  }

  /* ── Превью метки ── */

  function paintPreview(pin) {
    const kind = PIN_KIND_BY_ID[pin.kind] || PIN_KIND_BY_ID.other;
    const names = pin.mine ? [pin.author.name, ...participantNames(pin, 3)].slice(0, Math.min(4, pin.goingTotal)) : participantNames(pin, 4);
    render(
      els.body,
      html`
        <article class="preview" aria-labelledby="pin-title">
          <div class="preview__bar">
            <button type="button" class="btn btn--sm btn--ghost" data-action="back">${icon('arrow-left', 'icon--sm')}Все метки</button>
            <button type="button" class="icon-btn icon-btn--sm" data-action="share-pin" aria-label="Поделиться" title="Поделиться">${icon('share-2')}</button>
          </div>
          <div class="preview__meta">
            <span class="badge badge--timed">${icon(kind.icon)}${kind.label}</span>
            ${pinTimeBadge(pin)}
          </div>
          <div>
            <h2 class="preview__title" id="pin-title">${pin.title}</h2>
            ${pin.note ? html`<p class="preview__text" style="margin-top: var(--space-2)">${pin.note}</p>` : ''}
          </div>
          <div class="pin-author">
            ${avatar(pin.author.name, 'sm')}
            <span><strong>${pin.mine ? 'Ваша метка' : pin.author.name}</strong> · ${formatAgo(pin.createdAt)}</span>
          </div>
          <dl class="facts preview__facts">
            <div class="fact">${icon('map-pin')}<div><dt>Где</dt><dd>${pin.placeLabel}</dd></div></div>
            <div class="fact">${icon('clock')}<div><dt>Сколько продлится</dt><dd>${formatDuration((new Date(pin.endsAt) - new Date(pin.startsAt)) / 60000)}</dd></div></div>
            <div class="fact">${icon('navigation')}<div><dt>От центра</dt><dd>${formatDistance(distanceFromCenter(pin.coords))}</dd></div></div>
          </dl>
          ${pinMeter(pin)}
          ${names.length
            ? html`<div class="pin-going">
                <span class="avatar-stack">${names.map((name) => avatar(name, 'sm'))}</span>
                <span>${names.slice(0, 2).join(', ')}${pin.goingTotal > 2 ? ` и ещё ${pin.goingTotal - 2}` : ''}</span>
              </div>`
            : ''}
          ${pin.mine
            ? html`<button type="button" class="btn btn--outline btn--block" data-action="delete-pin">${icon('trash-2')}Снять метку</button>`
            : html`<button type="button" class="btn btn--accent btn--lg btn--block" data-action="pin-going" data-id="${pin.id}" aria-pressed="${pin.joined}">
                ${icon(pin.joined ? 'check' : 'plus')}${pin.joined ? 'Вы идёте — отменить' : 'Я приду'}
              </button>`}
          <div class="alert alert--neutral">${icon('shield-check')}<span>Встречайтесь в людных местах и не делитесь личными данными в описании.</span></div>
          ${pin.mine ? '' : html`<button type="button" class="btn btn--sm btn--danger" data-action="report">${icon('flag', 'icon--sm')}Пожаловаться</button>`}
        </article>
      `,
    );
    els.body.scrollTop = 0;
  }

  async function showPin(id) {
    try {
      const pin = await getPin(id);
      if (!active || state.pin !== id) return;
      map.select(id);
      paintPreview(pin);
      els.foot.hidden = true;
      if (isMobileLayout() && stage.getSheet() === 'peek') stage.setSheet('half');
      setTimeout(() => map.flyTo(pin.coords, Math.max(map.getZoom(), 14)), 60);
    } catch (error) {
      if (!active) return;
      console.warn('Метка из ссылки не найдена', error);
      toast(error.message || 'Метка не найдена', { type: 'danger' });
      setQuery({ pin: '' });
    }
  }

  /* ── Создание метки ── */

  function paintCreatePlace() {
    stage.showPicker(true);
    map.select(null);
    render(
      els.body,
      html`
        <form class="preview create-pin" data-form="place" novalidate>
          <div class="preview__bar">
            <button type="button" class="btn btn--sm btn--ghost" data-action="cancel-create">${icon('x', 'icon--sm')}Отмена</button>
            <span class="create-pin__step">Шаг 1 из 2</span>
          </div>
          <div>
            <h2 class="preview__title">Где собираемся?</h2>
            <p class="preview__text" style="margin-top: var(--space-2)">Подвиньте карту, чтобы метка указывала на место встречи.</p>
          </div>
          <div class="field">
            <label class="field__label" for="pin-place">Как найти <span class="optional">— необязательно</span></label>
            <input class="input" id="pin-place" name="placeLabel" maxlength="60" placeholder="Например, поле у стадиона «Динамо»" value="${draft?.placeLabel || ''}" />
            <p class="field__hint">Ориентир, который поймут все: вход, площадка, памятник</p>
          </div>
          <button type="submit" class="btn btn--accent btn--lg btn--block">Дальше ${icon('arrow-right')}</button>
        </form>
      `,
    );
    els.foot.hidden = true;
    if (isMobileLayout()) stage.setSheet('half');
  }

  function paintCreateDetails() {
    stage.showPicker(false);
    const kind = draft.kind || '';
    const example = PIN_KIND_BY_ID[kind]?.example || 'Футбол 5 на 5, нужно ещё двое';
    render(
      els.body,
      html`
        <form class="preview create-pin" data-form="details" novalidate>
          <div class="preview__bar">
            <button type="button" class="btn btn--sm btn--ghost" data-action="create-back">${icon('arrow-left', 'icon--sm')}Место</button>
            <span class="create-pin__step">Шаг 2 из 2</span>
          </div>
          <h2 class="preview__title">Что происходит?</h2>

          <fieldset class="fieldset">
            <legend>Тип</legend>
            <div class="chip-row chip-row--wrap">
              ${PIN_KINDS.map(
                (k) => html`<label class="chip chip--lg"><input type="radio" name="kind" value="${k.id}" ${kind === k.id ? 'checked' : ''} aria-describedby="pin-kind-error" />${icon(k.icon)}${k.label}</label>`,
              )}
            </div>
            <p class="field__error" id="pin-kind-error" role="alert"></p>
          </fieldset>

          <div class="field">
            <label class="field__label" for="pin-title">Заголовок</label>
            <input class="input" id="pin-title" name="title" maxlength="80" placeholder="${example}" value="${draft.title || ''}" aria-describedby="pin-title-error pin-title-count" required />
            <p class="field__error" id="pin-title-error" role="alert"></p>
            <span class="field__counter" id="pin-title-count">${(draft.title || '').length} / 80</span>
          </div>

          <fieldset class="fieldset">
            <legend>Когда начинаем</legend>
            <div class="chip-row chip-row--wrap">
              ${START_OPTIONS.map((o) => html`<label class="chip"><input type="radio" name="startsInMin" value="${o.value}" ${(draft.startsInMin ?? 30) === o.value ? 'checked' : ''} />${o.label}</label>`)}
            </div>
          </fieldset>

          <fieldset class="fieldset">
            <legend>Сколько продлится</legend>
            <div class="segmented" role="radiogroup">
              ${DURATION_OPTIONS.map((o) => html`<label class="segmented__item"><input type="radio" name="durationMin" value="${o.value}" ${(draft.durationMin ?? 120) === o.value ? 'checked' : ''} />${o.label}</label>`)}
            </div>
          </fieldset>

          <div class="field">
            <span class="field__label" id="pin-need-label">Сколько ещё человек нужно</span>
            <div class="create-pin__need">
              <div class="counter" role="group" aria-labelledby="pin-need-label">
                <button type="button" class="icon-btn icon-btn--sm" data-action="need-dec" aria-label="Меньше">${icon('minus')}</button>
                <output class="counter__value" name="needOut" aria-live="polite">${draft.need ?? 0}</output>
                <button type="button" class="icon-btn icon-btn--sm" data-action="need-inc" aria-label="Больше">${icon('plus')}</button>
              </div>
              <span class="field__hint" data-need-hint>${(draft.need ?? 0) === 0 ? 'Не важно, приходите все' : ''}</span>
            </div>
            <input type="hidden" name="need" value="${draft.need ?? 0}" />
          </div>

          <div class="field">
            <label class="field__label" for="pin-note">Подробности <span class="optional">— необязательно</span></label>
            <textarea class="textarea" id="pin-note" name="note" maxlength="140" rows="3" placeholder="Что взять с собой, как узнать вас на месте">${draft.note || ''}</textarea>
          </div>

          <div class="create-pin__place">
            ${icon('map-pin')}<span>${draft.placeLabel || 'Точка на карте'}</span>
            <button type="button" class="link-btn" data-action="create-back">Изменить</button>
          </div>

          <button type="submit" class="btn btn--accent btn--lg btn--block">Опубликовать метку</button>
          <p class="field__hint">Метку увидят все. Не пишите телефон и адрес квартиры — договоритесь на месте.</p>
        </form>
      `,
    );
    els.foot.hidden = true;
    if (isMobileLayout()) stage.setSheet('full');
  }

  function readDetails(form) {
    return {
      ...draft,
      kind: form.querySelector('input[name="kind"]:checked')?.value || '',
      title: form.elements.title.value,
      startsInMin: Number(form.querySelector('input[name="startsInMin"]:checked')?.value || 0),
      durationMin: Number(form.querySelector('input[name="durationMin"]:checked')?.value || 120),
      need: Number(form.elements.need.value || 0),
      note: form.elements.note.value,
    };
  }

  function setFieldError(form, id, message) {
    const error = $(`#${id}`, form);
    error.textContent = message;
  }

  async function submitDetails(form) {
    draft = readDetails(form);
    let valid = true;
    const title = form.elements.title;
    if (!draft.kind) {
      setFieldError(form, 'pin-kind-error', 'Выберите тип метки');
      valid = false;
    } else setFieldError(form, 'pin-kind-error', '');
    if (draft.title.trim().length < 5) {
      setFieldError(form, 'pin-title-error', 'Опишите встречу хотя бы в пяти символах');
      title.setAttribute('aria-invalid', 'true');
      valid = false;
    } else {
      setFieldError(form, 'pin-title-error', '');
      title.setAttribute('aria-invalid', 'false');
    }
    if (!valid) {
      (draft.kind ? title : form.querySelector('input[name="kind"]')).focus();
      return;
    }
    const button = $('button[type="submit"]', form);
    button.classList.add('is-loading');
    try {
      const pin = await createPin(draft);
      draft = null;
      toast(`Метка опубликована. Исчезнет через ${formatDuration((new Date(pin.endsAt) - Date.now()) / 60000)}`, { type: 'success' });
      await loadPins();
      setQuery({ create: '', pin: pin.id });
    } catch (error) {
      console.error('Не удалось опубликовать метку', error);
      toast(error.code === 'auth_required' ? 'Войдите, чтобы опубликовать метку' : 'Не получилось опубликовать. Попробуйте ещё раз', { type: 'danger' });
    } finally {
      button.classList.remove('is-loading');
    }
  }

  /* ── События ── */

  function openRules() {
    openDialog({ title: 'Правила меток', body: rulesBody(), footer: html`<button type="button" class="btn btn--primary" data-dialog-close>Понятно</button>` });
  }

  function startCreate() {
    withAuth(store, 'Чтобы поставить метку, войдите — это займёт минуту. Аккаунт VL.ru подходит.', () => {
      draft = { coords: null, placeLabel: '' };
      setQuery({ create: 'place', pin: '' });
    });
  }

  function bindEvents() {
    disposer.add(
      on(els.root, 'click', '[data-action]', async (event, el) => {
        const { action, id } = el.dataset;
        switch (action) {
          case 'kind':
            setQuery({ kind: id || '', pin: '' });
            break;
          case 'select':
            setQuery({ pin: id });
            break;
          case 'back':
            setQuery({ pin: '' });
            break;
          case 'create':
            startCreate();
            break;
          case 'cancel-create':
            draft = null;
            setQuery({ create: '' });
            break;
          case 'create-back':
            draft = readDetails($('form', els.body));
            setQuery({ create: 'place' });
            break;
          case 'hide-intro':
            try {
              sessionStorage.setItem(INTRO_KEY, '1');
            } catch (error) {
              console.warn('sessionStorage недоступен', error);
            }
            paintList();
            break;
          case 'rules':
            openRules();
            break;
          case 'reload':
            loadPins();
            break;
          case 'pin-going': {
            await withAuth(store, 'Войдите, чтобы отметиться «Я приду» — автор увидит, сколько человек собирается.', async () => {
              const pin = pins.find((p) => p.id === id);
              const updated = await setGoing(id, !pin?.joined);
              toast(updated.joined ? 'Отметили: вы идёте. Автор увидит это' : 'Отметку сняли');
              await loadPins();
              if (state.pin === id) paintPreview(updated);
            });
            break;
          }
          case 'share-pin': {
            const pin = pins.find((p) => p.id === state.pin);
            if (pin) shareLink({ title: pin.title, text: pin.note, hash: `#/now?pin=${pin.id}` });
            break;
          }
          case 'delete-pin': {
            const ok = await confirmDialog({ title: 'Снять метку?', text: 'Она сразу исчезнет с карты у всех.', confirmLabel: 'Снять', danger: true });
            if (!ok) return;
            await deletePin(state.pin);
            toast('Метка снята');
            await loadPins();
            setQuery({ pin: '' });
            break;
          }
          case 'report':
            openDialog({
              title: 'Пожаловаться на метку',
              body: html`
                <form id="report-form" class="report-form">
                  ${['Реклама или спам', 'Оскорбления', 'Опасная встреча', 'Личные данные', 'Другое'].map(
                    (reason, i) => html`<label class="check"><input type="radio" name="reason" value="${reason}" ${i === 0 ? 'checked' : ''} /><span>${reason}</span></label>`,
                  )}
                </form>
              `,
              footer: html`<button type="button" class="btn btn--ghost" data-dialog-close>Отмена</button><button type="submit" form="report-form" class="btn btn--primary">Отправить</button>`,
              onMount(dialogEl, close) {
                $('form', dialogEl).addEventListener('submit', async (submitEvent) => {
                  submitEvent.preventDefault();
                  await reportPin(state.pin, new FormData(submitEvent.target).get('reason'));
                  close();
                  toast('Спасибо. Модератор проверит метку', { type: 'success' });
                });
              },
            });
            break;
          case 'need-inc':
          case 'need-dec': {
            const form = el.closest('form');
            const value = Math.max(0, Math.min(30, Number(form.elements.need.value) + (action === 'need-inc' ? 1 : -1)));
            form.elements.need.value = value;
            form.elements.needOut.value = value;
            $('[data-need-hint]', form).textContent = value === 0 ? 'Не важно, приходите все' : '';
            break;
          }
          default:
        }
      }),
    );

    disposer.add(
      on(els.body, 'submit', 'form', (event, form) => {
        event.preventDefault();
        if (form.dataset.form === 'place') {
          draft = { ...draft, coords: map.getCenter(), placeLabel: form.elements.placeLabel.value.trim() };
          setQuery({ create: 'details' });
        } else if (form.dataset.form === 'details') {
          submitDetails(form);
        }
      }),
    );

    disposer.add(
      on(els.body, 'input', '#pin-title', (event, input) => {
        $('#pin-title-count', els.body).textContent = `${input.value.length} / 80`;
      }),
    );

    disposer.add(
      on(els.body, 'mouseover', '.pin-card', (event, card) => map.hover(card.dataset.id)),
    );

    stage.bind({
      select: (id) => {
        if (!state.create) setQuery({ pin: id });
      },
      hover: (id) => $$('.pin-card', els.body).forEach((card) => card.classList.toggle('is-hovered', card.dataset.id === id)),
    });

    const onKey = (event) => {
      if (event.key === 'Escape' && state.pin && !document.querySelector('dialog[open]')) setQuery({ pin: '' });
    };
    document.addEventListener('keydown', onKey);
    disposer.add(() => document.removeEventListener('keydown', onKey));

    disposer.add(store.subscribe(() => loadPins(), ['user']));

    const refresh = debounce(() => {
      if (!state.create) loadPins();
    }, 100);
    timer = setInterval(refresh, 60_000);
    disposer.add(() => clearInterval(timer));
  }

  function readQuery(query) {
    return { kind: query.kind || '', pin: query.pin || null, create: ['place', 'details'].includes(query.create) ? query.create : null };
  }

  function apply(prev = {}) {
    if (state.create === 'details' && !draft?.coords) {
      setQuery({ create: draft ? 'place' : '' });
      return;
    }
    if (state.create === 'place' && !store.get().user) {
      setQuery({ create: '' });
      return;
    }
    if (state.create === 'place') {
      if (!draft) draft = { coords: null, placeLabel: '' };
      return paintCreatePlace();
    }
    if (state.create === 'details') return paintCreateDetails();

    stage.showPicker(false);
    if (state.pin) return showPin(state.pin);

    map.select(null);
    paintFoot();
    paintList();
    if (prev.pin && isMobileLayout() && stage.getSheet() === 'half') stage.setSheet('peek');
  }

  return {
    enter(ctx) {
      active = true;
      state = readQuery(ctx.query);
      paintHead();
      paintFoot();
      render(els.body, html`<ul class="map-panel__list map-panel__list--cards" aria-busy="true">${Array.from({ length: 3 }, () => html`<li class="pin-card" aria-hidden="true"><div class="skeleton skeleton--text" style="width: 50%"></div><div class="skeleton skeleton--text" style="width: 90%; height: 1.125rem"></div><div class="skeleton skeleton--text" style="width: 60%"></div></li>`)}</ul>`);
      bindEvents();
      map.setMode('pins');
      if (!state.pin) map.resetView('now');
      stage.layoutSlots();
      loadPins().then((ok) => {
        if (!ok) return;
        paintKinds();
        apply();
      });
    },

    update(ctx) {
      const prev = state;
      state = readQuery(ctx.query);
      paintKinds();
      if (prev.kind !== state.kind && !state.pin && !state.create) paintList();
      if (prev.pin !== state.pin || prev.create !== state.create) apply(prev);
    },

    leave() {
      active = false;
      disposer.dispose();
      stage.showPicker(false);
      els.top.replaceChildren();
      draft = null;
    },
  };
}
