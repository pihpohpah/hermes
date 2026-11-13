/**
 * Шапка продукта: знак VL.ru, название «Места», навигация, «Добавить», вход / профиль.
 * На экранах < 48rem навигация уходит в выпадающую панель (кнопка-бургер).
 */

import { html, render, $, on } from '../core/dom.js';
import { store } from '../core/store.js';
import { icon } from './icon.js';
import { avatar } from './cards.js';
import { bindMenu } from './menu.js';
import { openAuthDialog } from './auth-dialog.js';
import { logout, formatPhone } from '../api/auth.js';
import { toast } from './toast.js';
import { navigate } from '../core/router.js';

const NAV = [
  { id: 'map', href: '#/map', label: 'Карта', icon: 'map', description: 'Все места на одной карте' },
  { id: 'catalog', href: '#/catalog', label: 'Справочник', icon: 'layout-grid', description: 'Поиск, подборки и события' },
  { id: 'now', href: '#/now', label: 'Сейчас', icon: 'radio', description: 'Быстрые метки от жителей' },
];

const VLRU_LINKS = [
  { href: 'https://www.vl.ru/', label: 'VL.ru' },
  { href: 'https://www.vl.ru/afisha', label: 'Афиша' },
  { href: 'https://www.vl.ru/weather', label: 'Погода' },
  { href: 'https://www.farpost.ru/', label: 'Фарпост' },
];

let section = 'map';
let liveCount = null;
let root;
let userMenu = null;

function userBlock(user) {
  if (!user) {
    return html`
      <button type="button" class="btn btn--ghost header__login" data-action="login" aria-label="Войти">
        ${icon('log-in')}<span class="header__login-text">Войти</span>
      </button>
    `;
  }
  return html`
    <div class="menu-anchor">
      <button type="button" class="header__user" id="user-menu-button" aria-haspopup="menu" aria-expanded="false" aria-controls="user-menu" aria-label="Меню профиля: ${user.name}">
        ${avatar(user.name, 'sm')}<span class="header__user-name">${user.name}</span>${icon('chevron-down', 'icon--sm')}
      </button>
      <div class="menu" id="user-menu" role="menu" hidden>
        <div class="menu__header">
          ${avatar(user.name, 'lg')}
          <div><strong>${user.name}</strong><span>${formatPhone(user.phone)}</span></div>
        </div>
        <button type="button" class="menu__item" role="menuitem" data-value="profile">${icon('user')}Профиль</button>
        <button type="button" class="menu__item" role="menuitem" data-value="pins">${icon('radio')}Мои метки</button>
        <button type="button" class="menu__item" role="menuitem" data-value="submissions">${icon('flag')}Мои предложения</button>
        <div class="menu__separator" role="separator"></div>
        <button type="button" class="menu__item" role="menuitem" data-value="logout">${icon('log-out')}Выйти</button>
      </div>
    </div>
  `;
}

function navLink(item) {
  const current = item.id === section;
  return html`
    <a class="nav__link" href="${item.href}"${current ? html` aria-current="page"` : ''}>
      ${item.label}
      ${item.id === 'now' && liveCount ? html`<span class="nav__live"><span class="live-dot" aria-hidden="true"></span><span aria-hidden="true">${liveCount}</span><span class="visually-hidden">, активных меток: ${liveCount}</span></span>` : ''}
    </a>
  `;
}

function paint() {
  const { user } = store.get();
  userMenu?.destroy();
  userMenu = null;
  render(
    root,
    html`
      <div class="header__inner">
        <a class="brand" href="#/map" aria-label="VL.ru Места — на главную">
          <img class="brand__logo" src="assets/brand/vlru-logo.svg" alt="" width="58" height="30" />
          <span class="brand__divider" aria-hidden="true"></span>
          <span class="brand__name">Места</span>
        </a>
        <nav class="nav" aria-label="Разделы">${NAV.map(navLink)}</nav>
        <div class="header__actions">
          <a class="btn btn--primary header__add" href="#/add" aria-label="Добавить место или событие">
            ${icon('plus')}<span class="header__add-text">Добавить</span>
          </a>
          ${userBlock(user)}
          <button type="button" class="icon-btn header__burger" data-action="mobile-menu" aria-expanded="false" aria-controls="mobile-menu" aria-label="Меню">
            ${icon('menu')}
          </button>
        </div>
      </div>
    `,
  );

  // Если мобильное меню открыто, перерисованная кнопка должна это отражать
  if (document.getElementById('mobile-menu')) {
    const burger = $('[data-action="mobile-menu"]', root);
    burger.setAttribute('aria-expanded', 'true');
    render(burger, icon('x'));
  }

  const menuButton = $('#user-menu-button', root);
  if (menuButton) {
    userMenu = bindMenu(menuButton, $('#user-menu', root), {
      onSelect(value) {
        if (value === 'logout') {
          logout();
          toast('Вы вышли из аккаунта');
        } else {
          navigate('/profile', value === 'profile' ? {} : { tab: value });
        }
      },
    });
  }
}

/* ── Мобильное меню ── */

function closeMobileMenu({ focus = false } = {}) {
  document.getElementById('mobile-menu')?.remove();
  document.getElementById('mobile-menu-backdrop')?.remove();
  const burger = $('[data-action="mobile-menu"]', root);
  burger?.setAttribute('aria-expanded', 'false');
  if (burger) render(burger, icon('menu'));
  if (focus) burger?.focus();
  document.removeEventListener('keydown', onMenuKey);
}

function onMenuKey(event) {
  if (event.key === 'Escape') closeMobileMenu({ focus: true });
}

function openMobileMenu() {
  const burger = $('[data-action="mobile-menu"]', root);
  burger.setAttribute('aria-expanded', 'true');
  render(burger, icon('x'));

  const backdrop = document.createElement('div');
  backdrop.className = 'mobile-menu-backdrop';
  backdrop.id = 'mobile-menu-backdrop';
  backdrop.addEventListener('click', () => closeMobileMenu());

  const panel = document.createElement('nav');
  panel.className = 'mobile-menu';
  panel.id = 'mobile-menu';
  panel.setAttribute('aria-label', 'Разделы');
  const { user } = store.get();
  render(
    panel,
    html`
      <div class="mobile-menu__nav">
        ${NAV.map(
          (item) => html`
            <a class="mobile-menu__link" href="${item.href}"${item.id === section ? html` aria-current="page"` : ''}>
              <span class="mobile-menu__icon">${icon(item.icon)}</span>
              <span><strong>${item.label}</strong><small>${item.description}</small></span>
              ${item.id === 'now' && liveCount ? html`<span class="nav__live"><span class="live-dot" aria-hidden="true"></span>${liveCount}</span>` : icon('chevron-right', 'icon--sm')}
            </a>
          `,
        )}
        <a class="mobile-menu__link" href="#/add">
          <span class="mobile-menu__icon">${icon('circle-plus')}</span>
          <span><strong>Добавить</strong><small>Место, событие или правку</small></span>
          ${icon('chevron-right', 'icon--sm')}
        </a>
      </div>
      <div class="mobile-menu__footer">
        ${user
          ? html`<a class="btn btn--outline btn--block" href="#/profile">${avatar(user.name, 'xs')}Профиль</a>`
          : html`<button type="button" class="btn btn--inverse btn--block" data-action="login">${icon('log-in')}Войти или зарегистрироваться</button>`}
        <div class="mobile-menu__links">${VLRU_LINKS.map((link) => html`<a href="${link.href}" target="_blank" rel="noopener">${link.label}</a>`)}</div>
      </div>
    `,
  );
  panel.addEventListener('click', (event) => {
    if (event.target.closest('a')) closeMobileMenu();
    if (event.target.closest('[data-action="login"]')) {
      closeMobileMenu();
      openAuthDialog();
    }
  });

  document.body.append(backdrop, panel);
  document.addEventListener('keydown', onMenuKey);
  panel.querySelector('a')?.focus();
}

export function initHeader(target) {
  root = target;
  paint();
  on(root, 'click', '[data-action="login"]', () => openAuthDialog());
  on(root, 'click', '[data-action="mobile-menu"]', () => {
    if (document.getElementById('mobile-menu')) closeMobileMenu();
    else openMobileMenu();
  });
  store.subscribe(paint, ['user']);
  window.matchMedia('(min-width: 48rem)').addEventListener('change', () => closeMobileMenu());
}

export function setHeaderSection(id) {
  closeMobileMenu();
  if (section === id) return;
  section = id;
  paint();
}

export function setLiveCount(count) {
  if (liveCount === count) return;
  liveCount = count;
  paint();
}

export { VLRU_LINKS };
