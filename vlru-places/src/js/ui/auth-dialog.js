/**
 * Вход и регистрация: телефон → код из SMS → имя (только для нового номера).
 * openAuthDialog({ reason }) → Promise<user | null>. reason объясняет, зачем нужен вход в этом месте.
 */

import { html, $ } from '../core/dom.js';
import { icon } from './icon.js';
import { openDialog } from './dialog.js';
import { toast } from './toast.js';
import { requestCode, verifyCode, register, isValidPhone, formatPhone } from '../api/auth.js';

const RESEND_SEC = 45;

function phoneDigits(value) {
  return value.replace(/\D/g, '').replace(/^[78](?=\d{10})/, '').slice(0, 10);
}

function maskPhone(digits) {
  const d = digits;
  let out = '';
  if (d.length) out += d.slice(0, 3);
  if (d.length > 3) out += ` ${d.slice(3, 6)}`;
  if (d.length > 6) out += `-${d.slice(6, 8)}`;
  if (d.length > 8) out += `-${d.slice(8, 10)}`;
  return out;
}

function phoneStep(reason, phone = '') {
  return {
    title: 'Вход в VL.ru',
    lead: reason || 'Аккаунт VL.ru подходит: если вы уже зарегистрированы на портале, войдёте по тому же номеру.',
    body: html`
      <form class="auth-form" data-step="phone" novalidate>
        <div class="field">
          <label class="field__label" for="auth-phone">Номер телефона</label>
          <div class="input-prefix">
            <span class="input-prefix__text" aria-hidden="true">+7</span>
            <input class="input" id="auth-phone" name="phone" type="tel" inputmode="tel" autocomplete="tel-national" placeholder="914 000-00-00" value="${maskPhone(phone)}" aria-describedby="auth-phone-error" required />
          </div>
          <p class="field__error" id="auth-phone-error" role="alert"></p>
        </div>
        <button type="submit" class="btn btn--primary btn--lg btn--block">Получить код</button>
        <p class="auth-form__legal">Нажимая «Получить код», вы принимаете <a href="#/rules" data-dialog-close>правила сервиса</a>.</p>
      </form>
    `,
  };
}

function codeStep(phone) {
  return {
    title: 'Код из SMS',
    lead: html`Отправили код на ${formatPhone(phone)}. <button type="button" class="link-btn" data-action="change-phone">Изменить номер</button>`,
    body: html`
      <form class="auth-form" data-step="code" novalidate>
        <div class="field">
          <label class="field__label" for="auth-code">Код</label>
          <input class="input input--code" id="auth-code" name="code" type="text" inputmode="numeric" autocomplete="one-time-code" maxlength="4" placeholder="••••" aria-describedby="auth-code-error" required />
          <p class="field__error" id="auth-code-error" role="alert"></p>
        </div>
        <button type="submit" class="btn btn--primary btn--lg btn--block">Войти</button>
        <p class="auth-form__resend" data-resend aria-live="polite"></p>
        <div class="alert alert--neutral">${icon('info')}<span>Это демо: подойдёт любой код из 4 цифр. Код 0000 покажет ошибку.</span></div>
      </form>
    `,
  };
}

function registerStep() {
  return {
    title: 'Как вас зовут?',
    lead: 'Имя увидят участники ваших меток. Фамилию указывать не обязательно.',
    body: html`
      <form class="auth-form" data-step="register" novalidate>
        <div class="field">
          <label class="field__label" for="auth-name">Имя</label>
          <input class="input" id="auth-name" name="name" type="text" autocomplete="given-name" maxlength="40" placeholder="Например, Аня" aria-describedby="auth-name-error" required />
          <p class="field__error" id="auth-name-error" role="alert"></p>
        </div>
        <label class="check">
          <input type="checkbox" name="rules" aria-describedby="auth-rules-error" />
          <span class="check__text">Согласен с правилами меток: без рекламы, личных данных и оскорблений</span>
        </label>
        <p class="field__error" id="auth-rules-error" role="alert"></p>
        <button type="submit" class="btn btn--primary btn--lg btn--block">Готово</button>
      </form>
    `,
  };
}

function setError(form, name, message) {
  const control = form.elements[name];
  const error = $(`#${control.getAttribute('aria-describedby')}`, form);
  control.setAttribute('aria-invalid', message ? 'true' : 'false');
  if (error) error.textContent = message || '';
  if (message) control.focus();
}

export function openAuthDialog({ reason = '' } = {}) {
  return new Promise((resolve) => {
    let phone = '';
    let resendTimer = null;
    let result = null;
    let resendBound = false;

    const startResendTimer = (el) => {
      clearInterval(resendTimer);
      let left = RESEND_SEC;
      const target = $('[data-resend]', el);
      const tick = () => {
        if (!target.isConnected) return clearInterval(resendTimer);
        if (left <= 0) {
          clearInterval(resendTimer);
          target.innerHTML = '<button type="button" class="link-btn" data-action="resend">Отправить код ещё раз</button>';
          return;
        }
        target.textContent = `Новый код можно запросить через 0:${String(left).padStart(2, '0')}`;
        left -= 1;
      };
      tick();
      resendTimer = setInterval(tick, 1000);
    };

    const dialog = openDialog({
      ...phoneStep(reason),
      className: 'dialog--auth',
      onClose: () => {
        clearInterval(resendTimer);
        resolve(result);
      },
      onMount(el) {
        const form = $('form', el);
        if (!form) return;
        const step = form.dataset.step;
        const submit = $('button[type="submit"]', form);

        if (step === 'phone') {
          const input = form.elements.phone;
          input.addEventListener('input', () => {
            input.value = maskPhone(phoneDigits(input.value));
            if (input.getAttribute('aria-invalid') === 'true' && isValidPhone(input.value)) setError(form, 'phone', '');
          });
          setTimeout(() => input.focus(), 50);
        }

        if (step === 'code') {
          const input = form.elements.code;
          input.addEventListener('input', () => {
            input.value = input.value.replace(/\D/g, '').slice(0, 4);
            if (input.value.length === 4) form.requestSubmit();
          });
          startResendTimer(el);
          setTimeout(() => input.focus(), 50);
        }

        if (step === 'register') setTimeout(() => form.elements.name.focus(), 50);

        el.querySelector('[data-action="change-phone"]')?.addEventListener('click', () => dialog.setContent(phoneStep(reason, phone)));
        // onMount вызывается на каждом шаге, а <dialog> один — слушатель вешается один раз
        if (!resendBound) {
          resendBound = true;
          el.addEventListener('click', async (event) => {
            if (!event.target.closest('[data-action="resend"]')) return;
            await requestCode(phone);
            toast('Отправили новый код');
            startResendTimer(el);
          });
        }

        form.addEventListener('submit', async (event) => {
          event.preventDefault();
          submit.classList.add('is-loading');
          try {
            if (step === 'phone') {
              phone = phoneDigits(form.elements.phone.value);
              await requestCode(phone);
              dialog.setContent(codeStep(phone));
            } else if (step === 'code') {
              const response = await verifyCode(phone, form.elements.code.value);
              if (response.status === 'ok') {
                result = response.user;
                toast(`С возвращением, ${response.user.name}`, { type: 'success' });
                dialog.close();
              } else {
                dialog.setContent(registerStep());
              }
            } else if (step === 'register') {
              if (!form.elements.rules.checked) {
                setError(form, 'rules', 'Чтобы продолжить, примите правила');
                return;
              }
              setError(form, 'rules', '');
              result = await register(phone, form.elements.name.value);
              toast(`Готово, ${result.name}. Теперь можно ставить метки`, { type: 'success' });
              dialog.close();
            }
          } catch (error) {
            if (error.code !== 'validation') console.error('Ошибка входа', error);
            const field = { phone: 'phone', code: 'code', register: 'name' }[step];
            setError(form, field, error.message);
          } finally {
            submit.classList.remove('is-loading');
          }
        });
      },
    });
  });
}

/** Выполняет действие, предварительно запросив вход, если пользователь не авторизован. */
export async function withAuth(store, reason, action) {
  if (store.get().user) return action();
  const user = await openAuthDialog({ reason });
  return user ? action() : null;
}
