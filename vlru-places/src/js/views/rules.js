/** Правила меток и модерации (#/rules). Тело переиспользуется в диалоге на экране «Сейчас». */

import { html, render } from '../core/dom.js';
import { icon } from '../ui/icon.js';
import { footer } from '../ui/footer.js';

const RULES = [
  { icon: 'clock', title: 'Метка живёт недолго', text: 'Она исчезает, когда встреча закончилась. Максимум — три часа после начала.' },
  { icon: 'users', title: 'Для всех, кто рядом', text: 'Зовите на игру, прогулку, джем или просите помощи. Метку видят все пользователи.' },
  { icon: 'ban', title: 'Без рекламы и продаж', text: 'Метки с рекламой, продажей и сбором денег снимаются без предупреждения.' },
  { icon: 'shield-check', title: 'Без личных данных', text: 'Не пишите телефон, адрес квартиры и фамилии. Договоритесь на месте.' },
  { icon: 'flag', title: 'Жалобы проверяем быстро', text: 'Если метка нарушает правила, нажмите «Пожаловаться» — модератор посмотрит её в течение часа.' },
];

export function rulesBody() {
  return html`
    <ul class="rules-list" role="list">
      ${RULES.map((rule) => html`<li class="rules-list__item"><span class="rules-list__icon">${icon(rule.icon)}</span><div><strong>${rule.title}</strong><p>${rule.text}</p></div></li>`)}
    </ul>
  `;
}

export const rulesView = {
  title: 'Правила',
  section: null,
  mount(target, ctx) {
    render(
      target,
      html`
        <div class="page container container--narrow">
          <header class="page__head">
            <h1 class="page__title">Правила</h1>
            <p class="page__lead">Как работают быстрые метки и как редакция проверяет места, которые присылают жители.</p>
          </header>
          <section class="section" style="margin-top: 0">
            <h2 class="section__title" style="margin-bottom: var(--space-5)">Быстрые метки</h2>
            ${rulesBody()}
          </section>
          <section class="section" id="moderation">
            <h2 class="section__title" style="margin-bottom: var(--space-3)">Как мы проверяем места</h2>
            <div class="prose">
              <p>Каждое предложение читает редактор: проверяет адрес по карте, часы работы и цену по открытым источникам, при необходимости связывается с автором.</p>
              <p>Обычно проверка занимает один-два рабочих дня. Статус видно в профиле, в разделе «Мои предложения».</p>
              <p>Не публикуем места, куда запрещён доступ, частную территорию без согласия владельца и опасные маршруты без предупреждений.</p>
            </div>
          </section>
        </div>
        ${footer()}
      `,
    );
    if (ctx?.query.section === 'moderation') document.getElementById('moderation')?.scrollIntoView();
  },
};
