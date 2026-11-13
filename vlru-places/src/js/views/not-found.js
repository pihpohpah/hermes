import { html, render } from '../core/dom.js';
import { icon } from '../ui/icon.js';
import { footer } from '../ui/footer.js';

/** 404 и «объект не найден». */
export function notFoundMarkup({ title = 'Такой страницы нет', text = 'Возможно, ссылка устарела или в ней опечатка. Начните с карты или справочника.' } = {}) {
  return html`
    <div class="page container">
      <div class="state-page">
        <span class="state-page__code" aria-hidden="true">404</span>
        <h1 class="state-page__title">${title}</h1>
        <p class="state-page__text">${text}</p>
        <div class="state-page__actions">
          <a class="btn btn--primary btn--lg" href="#/map">${icon('map')}Открыть карту</a>
          <a class="btn btn--outline btn--lg" href="#/catalog">Справочник</a>
        </div>
      </div>
    </div>
    ${footer()}
  `;
}

export const notFoundView = {
  title: 'Страница не найдена',
  section: null,
  mount(target) {
    render(target, notFoundMarkup());
  },
};
