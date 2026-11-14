/**
 * Точка входа: маршруты, шапка, переключение между «сценой» карты и обычными страницами.
 *
 * Контракт страницы (views/*.js):
 *   { title, section, mount(target, ctx) → { update?(ctx), unmount?() } }
 * ctx = { path, pattern, params, query }.
 * Экраны карты (#/map, #/now) — панели внутри постоянной сцены (views/stage.js).
 */

import { route, fallback, startRouter, onRouteChange } from './core/router.js';
import { prefersReducedMotion } from './core/dom.js';
import { initHeader, setHeaderSection, setLiveCount } from './ui/header.js';
import { getStage } from './views/stage.js';
import { createExplorePanel } from './views/explore.js';
import { createNowPanel } from './views/now.js';
import { catalogView } from './views/catalog.js';
import { detailView } from './views/detail.js';
import { addView, successView } from './views/add.js';
import { profileView } from './views/profile.js';
import { rulesView } from './views/rules.js';
import { notFoundView } from './views/not-found.js';
import { countActivePins } from './api/pins.js';
import { store } from './core/store.js';
import { SCENARIO_BY_ID } from './data/taxonomy.js';

const APP_TITLE = 'Пошли — VL.ru';
const STAGE_VIEWS = {
  explore: { title: 'Карта', section: 'map' },
  now: { title: 'Сейчас', section: 'now' },
};

route('/map', STAGE_VIEWS.explore);
route('/now', STAGE_VIEWS.now);
route('/catalog', catalogView);
route('/place/:id', detailView);
route('/event/:id', detailView);
route('/add', addView);
route('/add/success', successView);
route('/profile', profileView);
route('/rules', rulesView);
fallback(notFoundView);

const viewRoot = document.getElementById('view');
const panels = {};
let activePanel = null;
let activePage = null;

function panelFor(view) {
  const stage = getStage();
  if (view === STAGE_VIEWS.explore) return (panels.explore ??= createExplorePanel(stage));
  if (view === STAGE_VIEWS.now) return (panels.now ??= createNowPanel(stage));
  return null;
}

function teardownPage() {
  activePage?.instance?.unmount?.();
  activePage = null;
  viewRoot.replaceChildren();
}

function focusHeading() {
  const heading = viewRoot.querySelector('h1');
  if (!heading) return;
  heading.setAttribute('tabindex', '-1');
  heading.focus({ preventScroll: true });
}

function show(next, previous) {
  const panel = panelFor(next.view);
  document.title = `${next.view.title} — ${APP_TITLE}`;
  setHeaderSection(next.view.section);

  if (panel) {
    teardownPage();
    viewRoot.hidden = true;
    const stage = getStage();
    stage.show();
    if (activePanel !== panel) {
      activePanel?.leave();
      activePanel = panel;
      panel.enter(next);
    } else {
      panel.update(next);
    }
    return;
  }

  activePanel?.leave();
  activePanel = null;
  if (document.getElementById('map-stage').hidden === false) getStage().hide();
  viewRoot.hidden = false;
  teardownPage();
  // Сброс прокрутки до mount: экран может сам прокрутить к нужному разделу
  window.scrollTo(0, 0);
  activePage = { pattern: next.pattern, instance: next.view.mount(viewRoot, next) || null };
  if (previous) focusHeading();
}

onRouteChange((next, previous) => {
  if (['/map', '/catalog'].includes(next.path) && SCENARIO_BY_ID[next.query.s]) {
    store.set({ scenario: next.query.s });
  }

  const samePage = previous && previous.pattern === next.pattern && previous.params.id === next.params.id;

  if (samePage && activePage?.instance?.update) {
    activePage.instance.update(next);
    return;
  }
  if (samePage && activePanel) {
    activePanel.update(next);
    return;
  }

  const bothOnStage = previous && panelFor(previous.view) && panelFor(next.view);
  const canAnimate = previous && !bothOnStage && document.startViewTransition && !prefersReducedMotion();
  if (canAnimate) document.startViewTransition(() => show(next, previous));
  else show(next, previous);
});

initHeader(document.getElementById('header'));

// Ссылка «Перейти к содержимому»: обычный #main сломал бы hash-роутинг
document.querySelector('.skip-link').addEventListener('click', (event) => {
  event.preventDefault();
  const target = viewRoot.hidden ? document.getElementById('map-panel') : document.getElementById('main');
  target?.setAttribute('tabindex', '-1');
  target?.focus();
});

async function refreshLiveCount() {
  try {
    setLiveCount(await countActivePins());
  } catch (error) {
    console.warn('Не удалось обновить число активных меток', error);
  }
}

refreshLiveCount();
setInterval(refreshLiveCount, 60_000);
store.subscribe(refreshLiveCount, ['myPins']);

window.addEventListener('offline', () => import('./ui/toast.js').then(({ toast }) => toast('Нет соединения. Карта и новые данные могут не загрузиться', { type: 'danger', duration: 8000 })));
window.addEventListener('online', () => import('./ui/toast.js').then(({ toast }) => toast('Соединение восстановлено', { type: 'success' })));

startRouter('/map');
