import { html } from '../core/dom.js';
import { VLRU_LINKS } from './header.js';

/** Подвал для страниц без карты: справочник, карточки, формы, профиль. */
export function footer() {
  const year = new Date().getFullYear();
  return html`
    <footer class="footer">
      <div class="container">
        <div class="footer__top">
          <div class="footer__about">
            <a class="brand" href="#/map" aria-label="Пошли — на главную">
              <img class="brand__logo" src="assets/brand/poshli-mark.svg" alt="" width="30" height="30" />
              <span class="brand__name">Пошли</span>
              <svg class="brand__arrow" width="18" height="13" viewBox="0 0 20 14" fill="none" aria-hidden="true">
                <path d="M1 7h15M12 1l6 6-6 6" stroke="#EA580C" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" />
              </svg>
            </a>
            <p>Город, природа и море Владивостока и Приморья на одной карте. Места добавляют жители, редакция проверяет.</p>
          </div>
          <div>
            <h2 class="footer__title">Пошли</h2>
            <ul class="footer__list">
              <li><a href="#/map">Карта</a></li>
              <li><a href="#/catalog">Справочник</a></li>
              <li><a href="#/now">Сейчас</a></li>
              <li><a href="#/add">Добавить место</a></li>
            </ul>
          </div>
          <div>
            <h2 class="footer__title">Помощь</h2>
            <ul class="footer__list">
              <li><a href="#/rules">Правила меток</a></li>
              <li><a href="#/rules?section=moderation">Как мы проверяем места</a></li>
              <li><a href="#/add?type=fix">Сообщить об ошибке</a></li>
            </ul>
          </div>
          <div>
            <h2 class="footer__title">VL.ru</h2>
            <ul class="footer__list">
              ${VLRU_LINKS.map((link) => html`<li><a href="${link.href}" target="_blank" rel="noopener">${link.label}</a></li>`)}
            </ul>
          </div>
        </div>
        <div class="footer__bottom">
          <span>© 2003–${year} ООО «Фарпост». Портал VL.ru</span>
          <span>Прототип: описания, события и метки — демонстрационные. Фото — Wikimedia Commons, авторы указаны на страницах мест.</span>
        </div>
      </div>
    </footer>
  `;
}
