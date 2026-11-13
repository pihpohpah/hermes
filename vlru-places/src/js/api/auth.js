/**
 * Вход по номеру телефона и коду из SMS (имитация).
 * Демо-правила: подходит любой четырёхзначный код, кроме 0000 — он показывает состояние ошибки.
 * Новый номер после кода проходит короткую регистрацию (имя). Известные номера хранятся в store.accounts.
 */

import { delay, ApiError } from './http.js';
import { store } from '../core/store.js';

/** 10 цифр национального номера: «+7» или «8» в начале отбрасываются, только если цифр 11. */
const digits = (phone) => {
  const all = String(phone).replace(/\D/g, '');
  return all.length === 11 && /^[78]/.test(all) ? all.slice(1) : all;
};

export function isValidPhone(phone) {
  return digits(phone).length === 10;
}

export async function requestCode(phone) {
  if (!isValidPhone(phone)) throw new ApiError('validation', 'Введите номер полностью: 10 цифр после +7');
  await delay(500);
  return { resendInSec: 45 };
}

/** → { status: 'ok', user } для известного номера или { status: 'register' } для нового. */
export async function verifyCode(phone, code) {
  await delay(450);
  if (!/^\d{4}$/.test(code)) throw new ApiError('validation', 'Код состоит из 4 цифр');
  if (code === '0000') throw new ApiError('validation', 'Неверный код. Проверьте SMS или запросите новый');
  const known = store.get().accounts[digits(phone)];
  if (known) {
    store.set({ user: known });
    return { status: 'ok', user: known };
  }
  return { status: 'register' };
}

export async function register(phone, name) {
  const trimmed = String(name || '').trim();
  if (trimmed.length < 2) throw new ApiError('validation', 'Имя должно быть не короче двух букв');
  await delay(350);
  const user = { id: `u-${digits(phone)}`, name: trimmed, phone: `+7${digits(phone)}`, createdAt: new Date().toISOString() };
  store.set((state) => ({ user, accounts: { ...state.accounts, [digits(phone)]: user } }));
  return user;
}

export async function updateProfile(patch) {
  const { user } = store.get();
  if (!user) throw new ApiError('auth_required', 'Нужно войти');
  await delay(250);
  const next = { ...user, ...patch };
  store.set((state) => ({ user: next, accounts: { ...state.accounts, [digits(user.phone)]: next } }));
  return next;
}

export function logout() {
  store.set({ user: null });
}

export function formatPhone(phone) {
  const d = digits(phone).padEnd(10, '_');
  return `+7 ${d.slice(0, 3)} ${d.slice(3, 6)}-${d.slice(6, 8)}-${d.slice(8, 10)}`;
}
