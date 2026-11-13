/**
 * Профиль (#/profile?tab=pins|going|submissions): метки пользователя, отметки «Я приду»
 * и предложения в редакцию со статусом проверки. Без входа показывает приглашение войти.
 */

import { html, render, on, createDisposer } from '../core/dom.js';
import { store } from '../core/store.js';
import { setQuery, navigate } from '../core/router.js';
import { icon } from '../ui/icon.js';
import { toast } from '../ui/toast.js';
import { openDialog, confirmDialog } from '../ui/dialog.js';
import { openAuthDialog } from '../ui/auth-dialog.js';
import { listPins, setGoing } from '../api/pins.js';
import { updateProfile, logout, formatPhone } from '../api/auth.js';
import { pinCard, avatar } from '../ui/cards.js';
import { formatAgo, formatDay } from '../core/format.js';
import { footer } from '../ui/footer.js';

const TABS = [
  { id: 'pins', label: 'Мои метки', icon: 'radio' },
  { id: 'going', label: 'Я иду', icon: 'users' },
  { id: 'submissions', label: 'Предложения', icon: 'flag' },
];

const STATUS = {
  review: { label: 'На проверке', badge: 'warning', icon: 'hourglass' },
  published: { label: 'Опубликовано', badge: 'success', icon: 'circle-check' },
  rejected: { label: 'Отклонено', badge: 'danger', icon: 'circle-x' },
};

const TYPE_LABEL = { place: 'Место', event: 'Событие', fix: 'Правка' };

export const profileView = {
  title: 'Профиль',
  section: null,

  mount(target, ctx) {
    const disposer = createDisposer();
    let tab = TABS.some((t) => t.id === ctx.query.tab) ? ctx.query.tab : 'pins';
    let pins = [];
    let mounted = true;

    function guest() {
      render(
        target,
        html`
          <div class="page container">
            <div class="state-page">
              <span class="state-page__icon state-page__icon--neutral">${icon('user', 'icon--xl')}</span>
              <h1 class="state-page__title">Профиль появится после входа</h1>
              <p class="state-page__text">Войдите по номеру телефона — здесь будут ваши метки, отметки «Я приду» и статусы предложений.</p>
              <div class="state-page__actions">
                <button type="button" class="btn btn--primary btn--lg" data-action="login">${icon('log-in')}Войти</button>
                <a class="btn btn--outline btn--lg" href="#/map">На карту</a>
              </div>
            </div>
          </div>
          ${footer()}
        `,
      );
    }

    function tabContent(user) {
      const now = Date.now();
      if (tab === 'pins') {
        const mine = pins.filter((pin) => pin.mine);
        const past = store.get().myPins.filter((pin) => new Date(pin.endsAt).getTime() <= now);
        if (!mine.length && !past.length) {
          return html`
            <div class="empty">
              <span class="empty__icon">${icon('radio', 'icon--lg')}</span>
              <h2 class="empty__title">Меток пока нет</h2>
              <p class="empty__text">Позовите соседей на футбол, прогулку или джем — метка появится на карте «Сейчас».</p>
              <div class="empty__actions"><a class="btn btn--accent" href="#/now">${icon('plus')}Поставить метку</a></div>
            </div>
          `;
        }
        return html`
          ${mine.length ? html`<ul class="profile__grid" role="list">${mine.map((pin) => pinCard(pin))}</ul>` : ''}
          ${past.length
            ? html`
                <h3 class="profile__subtitle">Прошедшие</h3>
                <ul class="profile__rows" role="list">
                  ${past.map((pin) => html`<li class="profile-row profile-row--muted">${icon('radio')}<span><strong>${pin.title}</strong><span>${formatDay(pin.startsAt)} · ${pin.placeLabel}</span></span></li>`)}
                </ul>
              `
            : ''}
        `;
      }
      if (tab === 'going') {
        const going = pins.filter((pin) => pin.joined && !pin.mine);
        return going.length
          ? html`<ul class="profile__grid" role="list">${going.map((pin) => pinCard(pin))}</ul>`
          : html`
              <div class="empty">
                <span class="empty__icon">${icon('users', 'icon--lg')}</span>
                <h2 class="empty__title">Вы пока никуда не идёте</h2>
                <p class="empty__text">Загляните в «Сейчас» — там метки соседей, к которым можно присоединиться.</p>
                <div class="empty__actions"><a class="btn btn--outline" href="#/now">Открыть «Сейчас»</a></div>
              </div>
            `;
      }
      const submissions = store.get().submissions;
      return submissions.length
        ? html`
            <ul class="profile__rows" role="list">
              ${submissions.map((s) => {
                const status = STATUS[s.status] || STATUS.review;
                return html`
                  <li class="profile-row">
                    ${icon(s.type === 'event' ? 'calendar' : s.type === 'fix' ? 'pencil' : 'map-pin')}
                    <span><strong>${s.type === 'fix' ? s.fixTitle || s.title || 'Правка' : s.title}</strong><span>${TYPE_LABEL[s.type]} · ${formatAgo(s.createdAt)}</span></span>
                    <span class="badge badge--${status.badge}">${icon(status.icon)}${status.label}</span>
                  </li>
                `;
              })}
            </ul>
          `
        : html`
            <div class="empty">
              <span class="empty__icon">${icon('flag', 'icon--lg')}</span>
              <h2 class="empty__title">Предложений пока нет</h2>
              <p class="empty__text">Знаете место, которого нет в справочнике? Расскажите — редакция проверит и добавит.</p>
              <div class="empty__actions"><a class="btn btn--primary" href="#/add">${icon('plus')}Добавить место</a></div>
            </div>
          `;
    }

    function paint() {
      const { user, submissions } = store.get();
      if (!user) return guest();
      const counts = {
        pins: pins.filter((pin) => pin.mine).length,
        going: pins.filter((pin) => pin.joined && !pin.mine).length,
        submissions: submissions.length,
      };
      render(
        target,
        html`
          <div class="page container profile">
            <header class="profile__head">
              ${avatar(user.name, 'xl')}
              <div class="profile__who">
                <h1 class="profile__name">${user.name}</h1>
                <p>${formatPhone(user.phone)} · на VL.ru с ${formatDay(user.createdAt || new Date().toISOString())}</p>
              </div>
              <div class="profile__actions">
                <button type="button" class="btn btn--outline" data-action="rename">${icon('pencil')}Изменить имя</button>
                <button type="button" class="btn btn--ghost" data-action="logout">${icon('log-out')}Выйти</button>
              </div>
            </header>
            <div class="tabs" role="group" aria-label="Разделы профиля">
              ${TABS.map(
                (t) => html`
                  <button type="button" class="tab" data-action="tab" data-id="${t.id}" aria-pressed="${tab === t.id}">
                    ${icon(t.icon)}${t.label}<span class="tab__count">${counts[t.id]}</span>
                  </button>
                `,
              )}
            </div>
            <div class="profile__panel" role="region" aria-label="${TABS.find((t) => t.id === tab).label}">${tabContent(user)}</div>
            <div class="profile__demo">
              <div>
                <strong>Демо-данные</strong>
                <p>Метки, отметки и предложения хранятся только в этом браузере.</p>
              </div>
              <button type="button" class="btn btn--sm btn--danger" data-action="reset">${icon('rotate-ccw', 'icon--sm')}Сбросить всё</button>
            </div>
          </div>
          ${footer()}
        `,
      );
    }

    async function load() {
      const next = await listPins();
      if (!mounted) return;
      pins = next;
      paint();
    }

    disposer.add(
      on(target, 'click', '[data-action]', async (event, el) => {
        const { action, id } = el.dataset;
        if (action === 'login') openAuthDialog();
        if (action === 'tab') setQuery({ tab: id === 'pins' ? '' : id });
        if (action === 'select') navigate('/now', { pin: id });
        if (action === 'pin-going') {
          const pin = pins.find((p) => p.id === id);
          await setGoing(id, !pin?.joined);
          toast(pin?.joined ? 'Отметку сняли' : 'Отметили: вы идёте');
        }
        if (action === 'logout') {
          logout();
          toast('Вы вышли из аккаунта');
        }
        if (action === 'rename') {
          openDialog({
            title: 'Изменить имя',
            body: html`<form id="rename-form" class="field"><label class="field__label" for="rename-input">Имя</label><input class="input" id="rename-input" name="name" maxlength="40" value="${store.get().user.name}" required /></form>`,
            footer: html`<button type="button" class="btn btn--ghost" data-dialog-close>Отмена</button><button type="submit" form="rename-form" class="btn btn--primary">Сохранить</button>`,
            onMount(dialogEl, close) {
              const form = dialogEl.querySelector('form');
              form.elements.name.select();
              form.addEventListener('submit', async (submitEvent) => {
                submitEvent.preventDefault();
                const name = form.elements.name.value.trim();
                if (name.length < 2) return;
                await updateProfile({ name });
                close();
                toast('Имя обновлено', { type: 'success' });
              });
            },
          });
        }
        if (action === 'reset') {
          const ok = await confirmDialog({ title: 'Сбросить демо-данные?', text: 'Удалятся вход, метки, отметки, маршрут и предложения в этом браузере.', confirmLabel: 'Сбросить', danger: true });
          if (ok) {
            store.reset();
            toast('Демо-данные сброшены');
          }
        }
      }),
    );

    disposer.add(store.subscribe(load, ['user', 'myPins', 'joinedPins', 'submissions']));
    load();

    return {
      update(next) {
        tab = TABS.some((t) => t.id === next.query.tab) ? next.query.tab : 'pins';
        paint();
      },
      unmount() {
        mounted = false;
        disposer.dispose();
      },
    };
  },
};
