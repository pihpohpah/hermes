import { html, render } from '../core/dom.js';
import { icon } from './icon.js';

let counter = 0;

/**
 * Модальное окно на нативном <dialog>: фокус-ловушка, Escape и возврат фокуса — средствами браузера.
 * Окно всегда по центру экрана, высота — по содержимому в пределах экрана (src/styles/components/overlay.css).
 *
 * openDialog({ title, body, footer, wide, onMount(el, close), onClose }) → { el, close, setContent }
 * body / footer — SafeHtml (html`…`).
 */
export function openDialog({ title, lead = '', body = '', footer = '', wide = false, className = '', onMount, onClose } = {}) {
  counter += 1;
  const titleId = `dialog-title-${counter}`;
  const el = document.createElement('dialog');
  el.className = `dialog${wide ? ' dialog--wide' : ''}${className ? ` ${className}` : ''}`;
  el.setAttribute('aria-labelledby', titleId);

  const paint = (content) => {
    render(
      el,
      html`
        <div class="dialog__header">
          <h2 class="dialog__title" id="${titleId}">${content.title}</h2>
          <button type="button" class="icon-btn" data-dialog-close aria-label="Закрыть">${icon('x')}</button>
        </div>
        <div class="dialog__body">
          ${content.lead ? html`<p class="dialog__lead">${content.lead}</p>` : ''} ${content.body}
        </div>
        ${content.footer ? html`<div class="dialog__footer">${content.footer}</div>` : ''}
      `,
    );
  };

  paint({ title, lead, body, footer });

  let closed = false;
  const close = (result) => {
    if (closed) return;
    closed = true;
    el.close();
    // Уведомления, показанные внутри окна, переезжают в общую область и доживают своё время
    const toasts = el.querySelectorAll(':scope > .toast-region > .toast');
    if (toasts.length) document.getElementById('toasts')?.append(...toasts);
    el.remove();
    onClose?.(result);
  };

  el.addEventListener('click', (event) => {
    if (event.target === el) close();
    if (event.target.closest('[data-dialog-close]')) close();
  });
  el.addEventListener('cancel', (event) => {
    event.preventDefault();
    close();
  });

  document.body.append(el);
  el.showModal();
  onMount?.(el, close);

  return {
    el,
    close,
    setContent(content) {
      paint({ title, lead, body, footer, ...content });
      onMount?.(el, close);
    },
  };
}

/** Подтверждение действия. → Promise<boolean> */
export function confirmDialog({ title, text, confirmLabel = 'Подтвердить', danger = false }) {
  return new Promise((resolve) => {
    openDialog({
      title,
      body: html`<p>${text}</p>`,
      footer: html`
        <button type="button" class="btn btn--ghost" data-dialog-close>Отмена</button>
        <button type="button" class="btn ${danger ? 'btn--inverse' : 'btn--primary'}" data-confirm>${confirmLabel}</button>
      `,
      onMount(el, close) {
        el.querySelector('[data-confirm]').addEventListener('click', () => {
          resolve(true);
          close(true);
        });
      },
      onClose: (result) => {
        if (result !== true) resolve(false);
      },
    });
  });
}
