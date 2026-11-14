/* Обратная связь: тосты и модальные диалоги. */
import { $, escapeHtml, icon } from './util.js';

const TOAST_ICONS = { info: 'info', success: 'circle-check', danger: 'triangle-alert' };

function toastRegion() {
  const open = document.querySelector('dialog[open]');
  if (!open) return document.getElementById('toasts');
  let region = open.querySelector(':scope > .toast-region');
  if (!region) {
    region = document.createElement('div');
    region.className = 'toast-region';
    region.setAttribute('role', 'status');
    region.setAttribute('aria-live', 'polite');
    open.append(region);
  }
  return region;
}

export function toast(message, { type = 'info', action = null, duration = 4500, group = null } = {}) {
  const region = toastRegion();
  if (!region) return () => {};
  if (group) region.querySelectorAll(`[data-group="${group}"]`).forEach((node) => node.remove());

  const el = document.createElement('div');
  el.className = `toast toast--${type}`;
  el.innerHTML = `${icon(TOAST_ICONS[type] || 'info')}<span class="toast__text">${escapeHtml(message)}</span>${
    action ? '<button type="button" class="toast__action"></button>' : ''
  }`;

  let timer;
  const close = () => {
    clearTimeout(timer);
    el.classList.add('is-leaving');
    setTimeout(() => el.remove(), 400);
  };

  if (action) {
    const button = el.querySelector('.toast__action');
    button.textContent = action.label;
    button.addEventListener('click', () => {
      action.onClick();
      close();
    });
  }
  el.addEventListener('mouseenter', () => clearTimeout(timer));
  el.addEventListener('mouseleave', () => {
    timer = setTimeout(close, duration);
  });

  if (group) el.dataset.group = group;
  region.append(el);
  while (region.children.length > 3) region.firstElementChild.remove();
  timer = setTimeout(close, duration);
  return close;
}

export function openDialog({ title = '', lead = '', body = '', footer = '', className = '', wide = false, onMount } = {}) {
  const dialog = document.createElement('dialog');
  dialog.className = ['dialog', wide ? 'dialog--wide' : '', className].filter(Boolean).join(' ');
  // Формы внутри диалогов обрабатываются своим JS — boost не нужен.
  dialog.setAttribute('hx-disable', '');
  dialog.innerHTML = `
    <div class="dialog__header">
      <h2 class="dialog__title">${escapeHtml(title)}</h2>
      <button type="button" class="icon-btn" data-dialog-close aria-label="Закрыть">${icon('x')}</button>
    </div>
    ${lead ? `<p class="dialog__lead">${lead}</p>` : ''}
    <div class="dialog__body">${body}</div>
    ${footer ? `<div class="dialog__footer">${footer}</div>` : ''}`;

  const apiEl = {
    el: dialog,
    close: () => dialog.close(),
    setContent({ body: nextBody, footer: nextFooter, lead: nextLead }) {
      const bodyNode = $('.dialog__body', dialog);
      if (nextBody !== undefined) bodyNode.innerHTML = nextBody;
      const footerNode = $('.dialog__footer', dialog);
      if (nextFooter !== undefined) {
        if (footerNode) footerNode.innerHTML = nextFooter;
        else if (nextFooter) {
          const node = document.createElement('div');
          node.className = 'dialog__footer';
          node.innerHTML = nextFooter;
          dialog.append(node);
        }
      }
      const leadNode = $('.dialog__lead', dialog);
      if (nextLead !== undefined && leadNode) leadNode.innerHTML = nextLead;
    },
  };

  dialog.addEventListener('click', (event) => {
    if (event.target === dialog || event.target.closest('[data-dialog-close]')) dialog.close();
  });
  dialog.addEventListener('close', () => dialog.remove());
  dialog.addEventListener('cancel', () => dialog.remove());

  document.body.append(dialog);
  dialog.showModal();
  onMount?.(dialog, () => dialog.close(), apiEl);
  return apiEl;
}

export function confirmDialog({ title, text = '', confirmLabel = 'Подтвердить', danger = false }) {
  return new Promise((resolve) => {
    let done = false;
    openDialog({
      title,
      body: text ? `<p>${escapeHtml(text)}</p>` : '',
      footer: `<button type="button" class="btn btn--ghost" data-dialog-close>Отмена</button>
        <button type="button" class="btn ${danger ? 'btn--danger' : 'btn--primary'}" data-confirm>${escapeHtml(confirmLabel)}</button>`,
      onMount(el, close) {
        $('[data-confirm]', el).addEventListener('click', () => {
          done = true;
          close();
        });
        el.addEventListener('close', () => resolve(done));
      },
    });
  });
}
