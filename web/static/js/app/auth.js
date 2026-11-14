/* Вход по имени: диалог авторизации и проверка сессии.
 * Бэкенд выдаёт csrf-токен после POST /users; его наличие в localStorage —
 * признак «вошёл» для этого клиента. */
import { api, CSRF_KEY } from './api.js';
import { $, escapeHtml, storage } from './util.js';
import { openDialog, toast } from './ui.js';

export const isLoggedIn = () => Boolean(storage.get(CSRF_KEY, ''));

export function openAuthDialog({ reason = '' } = {}) {
  return new Promise((resolve) => {
    let result = null;
    openDialog({
      title: 'Вход в VL.ru',
      lead: escapeHtml(reason || 'Имя увидят участники ваших меток и редакторы справочника.'),
      className: 'dialog--auth',
      body: `
        <div class="auth-telegram" data-telegram-root hidden></div>
        <p class="auth-alt" data-telegram-alt hidden><span>или</span></p>
        <form class="auth-form" novalidate>
          <div class="field">
            <label class="field__label" for="auth-name">Имя</label>
            <input class="input" id="auth-name" name="name" type="text" autocomplete="given-name" maxlength="40" placeholder="Например, Аня" required />
            <p class="field__error" id="auth-name-error" role="alert"></p>
          </div>
          <label class="check">
            <input type="checkbox" name="rules" />
            <span class="check__text">Согласен с правилами меток: без рекламы, личных данных и оскорблений</span>
          </label>
          <p class="field__error" id="auth-rules-error" role="alert"></p>
          <button type="submit" class="btn btn--primary btn--lg btn--block">Войти</button>
        </form>`,
      onMount(el, close) {
        const form = $('form', el);
        const nameInput = form.elements.name;
        const nameError = $('#auth-name-error', form);
        const rulesError = $('#auth-rules-error', form);
        setTimeout(() => nameInput.focus(), 50);

        initTelegramLogin($('[data-telegram-root]', el), $('[data-telegram-alt]', el), {
          form,
          onSuccess(data) {
            result = data;
            close();
          },
        });

        form.addEventListener('submit', async (event) => {
          event.preventDefault();
          const name = nameInput.value.trim();
          if (name.length < 2) {
            nameError.textContent = 'Представьтесь — хотя бы пара букв';
            nameInput.focus();
            return;
          }
          nameError.textContent = '';
          if (!form.elements.rules.checked) {
            rulesError.textContent = 'Чтобы продолжить, примите правила';
            return;
          }
          rulesError.textContent = '';

          const submit = $('button[type="submit"]', form);
          submit.classList.add('is-loading');
          try {
            const data = await api('/users', {
              method: 'POST',
              body: { display_name: name, timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Vladivostok' },
            });
            storage.set(CSRF_KEY, data.csrf_token || '');
            result = data;
            toast(`Готово, ${data.display_name}. Теперь можно ставить метки`, { type: 'success' });
            close();
          } catch (error) {
            nameError.textContent = error.message || 'Не получилось войти. Попробуйте ещё раз';
          } finally {
            submit.classList.remove('is-loading');
          }
        });

        el.addEventListener('close', () => {
          telegramHandler = null;
          resolve(result);
        });
      },
    });
  });
}

/* Вход через Telegram: официальная Login library (OIDC-поток в popup).
 * Конфиг отдаёт бэкенд (GET /auth/telegram/widget): client_id и nonce.
 * Библиотека вызывает коллбек с id_token — подпись, iss, aud и exp токена
 * перепроверяет сервер, клиентскому коду он не доверяет. */
let telegramHandler = null;

window.__vlruTelegramAuth = (result) => {
  if (telegramHandler) telegramHandler(result);
};

function clientTimezone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Vladivostok';
}

async function initTelegramLogin(root, alt, { form, onSuccess }) {
  let config = null;
  try {
    config = await api('/auth/telegram/widget');
  } catch {
    return; /* конфиг недоступен — остаётся только вход по имени */
  }
  if (!config || !config.enabled || !config.client_id || !config.nonce) return;

  root.hidden = false;
  alt.hidden = false;

  const rulesError = $('#auth-rules-error', form);

  telegramHandler = async (result) => {
    if (!result || result.error || !result.id_token) {
      if (!result || result.error !== 'popup_closed') {
        rulesError.textContent = 'Не получилось войти через Telegram. Попробуйте ещё раз';
      }

      return;
    }
    if (!form.elements.rules.checked) {
      rulesError.textContent = 'Чтобы продолжить, примите правила';

      return;
    }
    rulesError.textContent = '';

    try {
      const data = await api('/auth/telegram', {
        method: 'POST',
        body: { id_token: result.id_token, timezone: clientTimezone() },
      });
      storage.set(CSRF_KEY, data.csrf_token || '');
      toast(`Готово, ${data.display_name}. Теперь можно ставить метки`, { type: 'success' });
      onSuccess(data);
    } catch (error) {
      rulesError.textContent = error.message || 'Не получилось войти через Telegram. Попробуйте ещё раз';
    }
  };

  const script = document.createElement('script');
  script.src = 'https://telegram.org/js/telegram-login.js';
  script.async = true;
  script.onload = () => {
    if (!window.Telegram || !window.Telegram.Login) return;
    root.innerHTML = '<button type="button" class="tg-auth-button">Войти через Telegram</button>';
    window.Telegram.Login.init(
      {
        client_id: Number(config.client_id),
        scope: ['profile'],
        lang: 'ru',
        nonce: config.nonce,
      },
      (result) => window.__vlruTelegramAuth(result),
    );
  };
  root.append(script);
}

/** Выполнить действие, сначала запросив вход для гостя. */
export function requireAuth(reason) {
  if (isLoggedIn()) return Promise.resolve(true);
  return openAuthDialog({ reason }).then((user) => Boolean(user));
}
