/* Делегированные data-action: логин/выход, поделиться, календарь, фото,
 * маршрут, выбор карточки, метки «Сейчас», зум/геолокация, поиск.
 * Модуль подключается за эффект — обработчик встаёт при импорте. */
import { $, $$, countLabel, currentParams, storage } from './util.js';
import { api, CSRF_KEY } from './api.js';
import { confirmDialog, openDialog, toast } from './ui.js';
import { openAuthDialog, requireAuth } from './auth.js';
import { downloadIcs, openLightbox, openRenameDialog, openReportDialog, shareLink } from './media.js';
import { clearRoute, replaceRoute, ROUTE_KEY, toggleRoute } from './route.js';
import { panelSwap, refreshPins, selectItem, selectPin } from './nav.js';
import { nowCreateComponent } from './pins.js';
import { openFiltersDialog } from './catalog.js';
import { openLuckyDialog } from './lucky.js';
import { openPlanDialog, openTripPicker, tripCancel, tripSetStatus } from './trips.js';

// Эти действия уже имеют инлайновые Alpine-обработчики на самих элементах.
const INLINE_ACTIONS = ['toggle-panel', 'mobile-menu', 'hide-intro', 'toggle-picker', 'cancel-create'];

document.addEventListener('click', (event) => {
  const el = event.target.closest('[data-action]');
  if (!el) return;
  const { action, id } = el.dataset;
  if (INLINE_ACTIONS.includes(action)) return;
  if (action === 'filters' && el.hasAttribute('hx-get')) return; // кнопка на карте открывает вкладку фильтров через htmx
  if (el.tagName === 'A' && el.hasAttribute('hx-get')) return;

  switch (action) {
    case 'login':
      event.preventDefault();
      openAuthDialog().then((user) => {
        if (user) location.reload();
      });
      break;
    case 'logout':
      event.preventDefault();
      api('/users/logout', { method: 'POST' })
        .catch(() => {})
        .finally(() => {
          storage.remove(CSRF_KEY);
          toast('Вы вышли');
          location.reload();
        });
      break;
    case 'rename':
      openRenameDialog();
      break;
    case 'lucky':
      openLuckyDialog();
      break;
    case 'mvp-note':
      openDialog({
        title: 'Каво и чо',
        body: `<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Integer posuere erat a ante venenatis dapibus posuere velit aliquet. Cras mattis consectetur purus sit amet fermentum.</p>
          <p>Donec ullamcorper nulla non metus auctor fringilla. Maecenas sed diam eget risus varius blandit sit amet non magna. Vestibulum id ligula porta felis euismod semper.</p>
          <p>Nullam quis risus eget urna mollis ornare vel eu leo. Aenean lacinia bibendum nulla sed consectetur. Etiam porta sem malesuada magna mollis euismod.</p>`,
      });
      break;
    case 'share':
      event.preventDefault();
      shareLink({ title: el.dataset.shareTitle || document.title, text: el.dataset.shareText || '', url: el.dataset.shareUrl || location.href });
      break;
    case 'ics':
      downloadIcs(el.dataset);
      break;
    case 'photo': {
      const article = el.closest('article[data-photos]');
      if (!article) break;
      let photos;
      try {
        photos = JSON.parse(article.dataset.photos);
      } catch {
        photos = [];
      }
      if (!photos.length) break;
      const gallery = el.closest('.preview__gallery, .detail__gallery, .gallery');
      const imgs = gallery ? $$('img', gallery) : [];
      const img = el.querySelector('img') || el.closest('[data-photo-index]');
      let start = 0;
      if (img && imgs.length) start = imgs.indexOf(img);
      if (start < 0) start = 0;
      openLightbox(photos, start, $('.detail__title', article)?.textContent?.trim() || document.title);
      break;
    }
    case 'route-toggle':
      toggleRoute(id, el.dataset.title || '');
      if (window.Alpine) window.Alpine.store('route').ids = storage.get(ROUTE_KEY, []);
      break;
    case 'route-load': {
      // Лента «Для вас»: загрузить готовый набор точек (data-ids через запятую) в маршрут.
      const ids = (el.dataset.ids || '').split(',').map((value) => value.trim()).filter(Boolean);
      if (!ids.length) break;
      replaceRoute(ids);
      toast(`Маршрут: ${ids.length} ${countLabel(ids.length, ['точка', 'точки', 'точек'])}. Считаем путь…`);
      location.href = '/?panel=route';
      break;
    }
    case 'route-clear': {
      clearRoute();
      if (window.Alpine) window.Alpine.store('route').ids = [];
      const params = currentParams();
      if (params.get('panel') === 'route') {
        params.delete('panel');
        const qs = params.toString();
        panelSwap(`/fragments/map-panel?${qs}`, `/?${qs}`);
      }
      break;
    }
    case 'filters':
      openFiltersDialog({ scenario: el.dataset.scenario || currentParams().get('s') || 'city' });
      break;
    case 'select': {
      const stage = document.getElementById('map-stage');
      const mode = stage?.dataset.stageMode || 'items';
      if (mode === 'pins') selectPin(id);
      else selectItem(id);
      break;
    }
    case 'pin-going':
      requireAuth('Войдите, чтобы отметиться «Я приду» — автор увидит, сколько человек собирается.').then(async (ok) => {
        if (!ok) return;
        const joined = el.getAttribute('aria-pressed') === 'true';
        try {
          await api(`/pins/${encodeURIComponent(id)}/going`, { method: joined ? 'DELETE' : 'PUT' });
          toast(joined ? 'Отметку сняли' : 'Отметили: вы идёте. Автор увидит это');
          await refreshPins();
          htmx.ajax('GET', `/fragments/pin/${encodeURIComponent(id)}`, { target: '#map-panel-inner' });
        } catch (error) {
          toast(error.message || 'Не получилось отметиться', { type: 'danger' });
        }
      });
      break;
    case 'delete-pin':
      confirmDialog({ title: 'Снять метку?', text: 'Она сразу исчезнет с карты у всех.', confirmLabel: 'Снять', danger: true }).then(async (ok) => {
        if (!ok) return;
        try {
          await api(`/pins/${encodeURIComponent(id)}`, { method: 'DELETE' });
          toast('Метка снята');
          const params = currentParams();
          params.delete('pin');
          const qs = params.toString();
          panelSwap(`/fragments/pins${qs ? `?${qs}` : ''}`, `/now${qs ? `?${qs}` : ''}`);
          await refreshPins();
        } catch (error) {
          toast(error.message || 'Не получилось снять метку', { type: 'danger' });
        }
      });
      break;
    case 'report':
      openReportDialog(id);
      break;
    case 'create':
      nowCreateComponent()?.start();
      break;
    case 'plan-route':
      openPlanDialog({ activity: el.dataset.activity || 'hike' });
      break;
    case 'plan-place':
      openPlanDialog({
        stops: [{ id: el.dataset.placeId, title: el.dataset.placeName }],
        activity: el.dataset.activity || 'walk',
      });
      break;
    case 'trip-picker':
      openTripPicker();
      break;
    case 'trip-done':
      tripSetStatus(id, 'done');
      break;
    case 'trip-cancel':
      tripCancel(id);
      break;
    case 'zoom-in':
      window.placesMap?.zoomIn();
      break;
    case 'zoom-out':
      window.placesMap?.zoomOut();
      break;
    case 'locate': {
      const fallback = () => {
        window.placesMap?.showMe([131.8855, 43.1155]);
        toast('Геолокация недоступна — показываем центр Владивостока');
      };
      if (!navigator.geolocation) {
        fallback();
        break;
      }
      navigator.geolocation.getCurrentPosition(
        ({ coords }) => {
          const point = [coords.longitude, coords.latitude];
          const inBounds = point[0] > 130.5 && point[0] < 139 && point[1] > 42 && point[1] < 49;
          if (inBounds) window.placesMap?.showMe(point);
          else {
            window.placesMap?.showMe([131.8855, 43.1155]);
            toast('Вы далеко от Приморья — показываем центр Владивостока');
          }
        },
        fallback,
        { timeout: 6000, maximumAge: 60000 },
      );
      break;
    }
    case 'retry-map':
      window.__remountMap?.();
      break;
    case 'clear-search': {
      event.preventDefault();
      const params = currentParams();
      params.delete('q');
      const qs = params.toString();
      panelSwap(`/fragments/map-panel?${qs}`, `/?${qs}`);
      break;
    }
    default:
  }
});
