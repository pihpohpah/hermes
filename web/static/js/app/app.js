/**
 * Пошли VL.ru — клиентская логика (htmx + Alpine.js). Входная точка.
 *
 * Сервер отдаёт страницы и HTML-фрагменты (htmx их подменяет). Здесь живёт то, что
 * декларативно не выражается: карта, диалоги, маршрут, метки «Сейчас»,
 * мастер добавления и делегированные обработчики поиска/фильтров.
 *
 * Разбивка по модулям: util (DOM/форматирование), api (REST + CSRF),
 * ui (тосты/диалоги), auth, taxonomy (справочники фильтров), nav (htmx-свопы
 * панели), route, pins («Сейчас»), add (мастер), media (лайтбокс/ics/жалобы),
 * layout (геометрия сцены), catalog (поиск/фильтры), stage (карта/шторка),
 * map (MapLibre), map-style (стиль + токены), map-tiles (PMTiles и офлайн),
 * offline-map (офлайн-карта в профиле), actions (делегированные data-action).
 */
import { currentParams } from './util.js';
import { registerRoute } from './route.js';
import { registerNowCreate } from './pins.js';
import { registerAddMaster } from './add.js';
import { bindCatalogListeners } from './catalog.js';
import { initStage, initMiniMaps } from './stage.js';
import { registerOfflineMap } from './offline-map.js';
import './actions.js';

/* htmx после свопа копирует class/style со старых узлов на новые (settle),
 * затирая инлайн-стили Alpine (x-show) раньше, чем тот применит их — меню
 * пользователя «открывалось само» после буст-перехода. Settle нам не нужен. */
if (window.htmx) htmx.config.attributesToSettle = [];

document.addEventListener('alpine:init', () => {
  registerRoute();
  registerNowCreate();
  registerAddMaster();
  registerOfflineMap();
});

bindCatalogListeners();

window.addEventListener('popstate', (event) => {
  if (event.state && event.state.htmx) return; // восстановлением управляет htmx
  location.reload();
});

/* Клик по ссылке «на саму себя» (тот же путь и query) не должен запускать
 * буст-своп — иначе страница «бликает» перерисовкой себя же. Перехватываем
 * на capture-фазе до обработчиков htmx. */
document.addEventListener(
  'click',
  (event) => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    const anchor = event.target.closest('a[href]');
    if (!anchor || anchor.target || anchor.hasAttribute('download')) return;
    const url = new URL(anchor.href, location.href);
    if (url.origin !== location.origin) return;
    if (url.pathname === location.pathname && url.search === location.search) {
      event.preventDefault();
      event.stopPropagation();
    }
  },
  true,
);

/* Инициализация видимой страницы. Запускается при первой загрузке и после
 * каждого hx-boost-свопа body (htmx:load). Контроллер карты жив, только пока
 * на странице тот же узел сцены: при переходе (в т.ч. «Изменить» маршрута)
 * body подменяется, старый контроллер уничтожаем и создаём заново. */
function initPage() {
  const stage = document.getElementById('map-stage');
  if (stage) {
    if (window.placesMap && window.__mapStageNode !== stage) {
      try {
        window.placesMap.destroy();
      } catch {
        /* контейнер уже отсоединён */
      }
      window.placesMap = null;
    }
    if (!window.placesMap) {
      window.__mapStageNode = stage;
      initStage();
    }
  } else if (window.placesMap) {
    try {
      window.placesMap.destroy();
    } catch {
      /* контейнер уже отсоединён */
    }
    window.placesMap = null;
    window.__mapStageNode = null;
  }
  initMiniMaps();

  // Правила: ?section=moderation прокручивает к разделу модерации.
  if (location.pathname === '/rules') {
    const section = currentParams().get('section');
    if (section) {
      const target = document.getElementById(section);
      if (target) setTimeout(() => target.scrollIntoView({ behavior: 'smooth', block: 'start' }), 100);
    }
  }
}

document.addEventListener('DOMContentLoaded', initPage);
document.addEventListener('htmx:load', initPage);
