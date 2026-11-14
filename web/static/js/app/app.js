/**
 * Места VL.ru — клиентская логика (htmx + Alpine.js). Входная точка.
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

document.addEventListener('alpine:init', () => {
  registerRoute();
  registerNowCreate();
  registerAddMaster();
  registerOfflineMap();
});

bindCatalogListeners();

window.addEventListener('popstate', () => location.reload());

document.addEventListener('DOMContentLoaded', () => {
  initStage();
  initMiniMaps();

  // Правила: ?section=moderation прокручивает к разделу модерации.
  if (location.pathname === '/rules') {
    const section = currentParams().get('section');
    if (section) {
      const target = document.getElementById(section);
      if (target) setTimeout(() => target.scrollIntoView({ behavior: 'smooth', block: 'start' }), 100);
    }
  }
});
