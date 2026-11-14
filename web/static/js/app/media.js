/* Диалоги: поделиться, календарь (.ics), лайтбокс фото, переименование, жалоба. */
import { $, escapeHtml, icon } from './util.js';
import { api } from './api.js';
import { openDialog, toast } from './ui.js';

export async function shareLink({ title, text = '', url }) {
  const absolute = new URL(url, location.origin).href;
  if (navigator.share) {
    try {
      await navigator.share({ title, text, url: absolute });
      return;
    } catch (error) {
      if (error.name === 'AbortError') return;
      console.warn('Системное меню «Поделиться» недоступно, копируем ссылку', error);
    }
  }
  try {
    await navigator.clipboard.writeText(absolute);
    toast('Ссылка скопирована', { type: 'success' });
  } catch (error) {
    console.warn('Не удалось скопировать ссылку', error);
    toast(`Ссылка: ${absolute}`, { duration: 8000 });
  }
}

/* Экранирование текста по RFC 5545 и фолдинг строк (порт из detail.js макета). */
const icsText = (value = '') => String(value).replace(/[\\,;]/g, (ch) => `\\${ch}`).replace(/\r?\n/g, '\\n');

function icsFold(line) {
  if (new TextEncoder().encode(line).length <= 75) return line;
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

export function downloadIcs(dataset) {
  const stamp = (iso) => new Date(iso).toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '');
  const startsAt = dataset.icsStarts;
  if (!startsAt) return;
  const end = dataset.icsEnds || new Date(new Date(startsAt).getTime() + 2 * 3600000).toISOString();
  const ics = [
    'BEGIN:VCALENDAR',
    'VERSION:2.0',
    'PRODID:-//VL.ru//Mesta//RU',
    'BEGIN:VEVENT',
    `UID:${dataset.icsId}@places.vl.ru`,
    `DTSTAMP:${stamp(new Date().toISOString())}`,
    `DTSTART:${stamp(startsAt)}`,
    `DTEND:${stamp(end)}`,
    `SUMMARY:${icsText(dataset.icsTitle)}`,
    `LOCATION:${icsText(`${dataset.icsVenue || ''}, ${dataset.icsAddress || ''}`)}`,
    `DESCRIPTION:${icsText(dataset.icsShort || '')}`,
    'END:VEVENT',
    'END:VCALENDAR',
  ]
    .map(icsFold)
    .join('\r\n');

  const blob = new Blob([ics], { type: 'text/calendar;charset=utf-8' });
  const link = document.createElement('a');
  link.href = URL.createObjectURL(blob);
  link.download = `${dataset.icsId}.ics`;
  link.click();
  setTimeout(() => URL.revokeObjectURL(link.href), 1000);
  toast('Файл календаря скачан', { type: 'success' });
}

export function openLightbox(photos, start, title) {
  let index = Math.max(0, start);
  const body = () => {
    const photo = photos[index];
    const credits = [
      photo.author ? `Фото: ${escapeHtml(photo.author)}` : '',
      photo.license
        ? `<a href="${escapeHtml(photo.license_url || '#')}" target="_blank" rel="noopener">${escapeHtml(photo.license)}</a>`
        : '',
      photo.source ? `<a href="${escapeHtml(photo.source)}" target="_blank" rel="noopener">источник</a>` : '',
    ]
      .filter(Boolean)
      .join(' · ');
    return `
      <figure class="lightbox">
        <img src="${escapeHtml(photo.src)}" alt="${escapeHtml(photo.alt || '')}" />
        <figcaption>
          <span>${escapeHtml(photo.alt || '')}</span>
          ${credits ? `<span class="detail__muted">${credits}</span>` : ''}
        </figcaption>
      </figure>`;
  };
  const footer = () =>
    photos.length > 1
      ? `<button type="button" class="btn btn--outline" data-step="-1">${icon('chevron-left')}Назад</button>
         <span class="lightbox__counter tabular">${index + 1} из ${photos.length}</span>
         <button type="button" class="btn btn--outline" data-step="1">Дальше${icon('chevron-right')}</button>`
      : '';

  const step = (delta) => {
    index = (index + delta + photos.length) % photos.length;
    dialog.setContent({ body: body(), footer: footer() });
  };

  const dialog = openDialog({
    title,
    body: body(),
    footer: footer(),
    wide: true,
    className: 'dialog--lightbox',
    onMount(el) {
      $(':scope .dialog__footer', el)?.addEventListener('click', (event) => {
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

export function openRenameDialog() {
  const current =
    $('.profile__name')?.textContent?.trim() || $('.header__user-name')?.textContent?.trim() || '';
  openDialog({
    title: 'Как вас зовут?',
    body: `
      <form class="auth-form" id="rename-form" novalidate>
        <div class="field">
          <label class="field__label" for="rename-name">Имя</label>
          <input class="input" id="rename-name" name="name" type="text" maxlength="40" value="${escapeHtml(current)}" required />
          <p class="field__error" id="rename-name-error" role="alert"></p>
        </div>
      </form>`,
    footer: `<button type="button" class="btn btn--ghost" data-dialog-close>Отмена</button>
      <button type="submit" form="rename-form" class="btn btn--primary">Сохранить</button>`,
    onMount(el, close) {
      const input = $('#rename-name', el);
      setTimeout(() => {
        input.focus();
        input.select();
      }, 50);
      $('#rename-form', el).addEventListener('submit', async (event) => {
        event.preventDefault();
        const name = input.value.trim();
        const error = $('#rename-name-error', el);
        if (name.length < 2) {
          error.textContent = 'Имя — хотя бы пара букв';
          input.focus();
          return;
        }
        try {
          await api('/users/me', { method: 'PATCH', body: { display_name: name } });
          close();
          location.reload();
        } catch (err) {
          error.textContent = err.message || 'Не получилось сохранить';
        }
      });
    },
  });
}

export function openReportDialog(pinId) {
  const reasons = ['Реклама или спам', 'Оскорбления', 'Опасная встреча', 'Личные данные', 'Другое'];
  openDialog({
    title: 'Пожаловаться на метку',
    body: `
      <form id="report-form" class="report-form">
        ${reasons
          .map((reason, i) => `<label class="check"><input type="radio" name="reason" value="${escapeHtml(reason)}" ${i === 0 ? 'checked' : ''} /><span>${escapeHtml(reason)}</span></label>`)
          .join('')}
      </form>`,
    footer: `<button type="button" class="btn btn--ghost" data-dialog-close>Отмена</button>
      <button type="submit" form="report-form" class="btn btn--primary">Отправить</button>`,
    onMount(el, close) {
      $('#report-form', el).addEventListener('submit', async (event) => {
        event.preventDefault();
        const reason = new FormData(event.target).get('reason');
        try {
          await api(`/pins/${encodeURIComponent(pinId)}/reports`, { method: 'POST', body: { reason } });
          close();
          toast('Спасибо. Модератор проверит метку', { type: 'success' });
        } catch (error) {
          toast(error.message || 'Не получилось отправить жалобу', { type: 'danger' });
        }
      });
    },
  });
}
