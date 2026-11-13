/**
 * Быстрые метки «Сейчас» — временные приглашения от жителей: «собираемся играть в футбол через полчаса».
 * Метка живёт до endsAt и затем исчезает с карты. Создавать метки и отмечаться «Я приду» можно
 * только после входа. Демо-метки из data/pins.json пересчитываются относительно текущего времени,
 * поэтому всегда выглядят свежими. Метки пользователя хранятся в store.myPins.
 */

import { getJson, delay, ApiError } from './http.js';
import { store } from '../core/store.js';

const MINUTE = 60_000;
let seeded = null;

/** Детерминированный сдвиг, чтобы метки одного района не лежали в одной точке. */
function jitter(index) {
  const angle = (index * 137.5 * Math.PI) / 180;
  const radius = 0.0018 + (index % 4) * 0.0009;
  return [Math.cos(angle) * radius * 1.35, Math.sin(angle) * radius];
}

async function loadSeed() {
  if (seeded) return seeded;
  const { pins, areas } = await getJson('pins.json');
  const now = Date.now();
  seeded = pins.map((pin, index) => {
    const startsAt = new Date(now + pin.startsInMin * MINUTE).toISOString();
    const endsAt = new Date(now + (pin.startsInMin + pin.durationMin) * MINUTE).toISOString();
    const base = pin.coords || areas[pin.area];
    const [dx, dy] = pin.coords ? [0, 0] : jitter(index);
    return {
      ...pin,
      coords: [base[0] + dx, base[1] + dy],
      startsAt,
      endsAt,
      createdAt: new Date(now - (15 + index * 7) * MINUTE).toISOString(),
      author: { name: pin.authorName },
    };
  });
  return seeded;
}

function decorate(pin) {
  const { joinedPins, user } = store.get();
  const joined = joinedPins.includes(pin.id);
  const mine = Boolean(user && pin.author?.id === user.id);
  return { ...pin, joined, mine, goingTotal: pin.going + (joined && !mine ? 1 : 0) };
}

export async function listPins({ kind = '' } = {}) {
  const seed = await loadSeed();
  await delay(150);
  const now = Date.now();
  return [...store.get().myPins, ...seed]
    .filter((pin) => new Date(pin.endsAt).getTime() > now)
    .filter((pin) => !kind || pin.kind === kind)
    .map(decorate)
    .sort((a, b) => new Date(a.startsAt) - new Date(b.startsAt));
}

export async function getPin(id) {
  const seed = await loadSeed();
  const pin = [...store.get().myPins, ...seed].find((p) => p.id === id);
  if (!pin) throw new ApiError('not_found', 'Метка не найдена или уже исчезла');
  return decorate(pin);
}

export async function countActivePins() {
  const pins = await listPins();
  return pins.length;
}

function requireUser() {
  const { user } = store.get();
  if (!user) throw new ApiError('auth_required', 'Нужно войти');
  return user;
}

/**
 * @param {{kind, title, note, placeLabel, coords:[lng,lat], startsInMin, durationMin, need}} data
 */
export async function createPin(data) {
  const user = requireUser();
  await delay(400);
  const now = Date.now();
  const pin = {
    id: `my-${now.toString(36)}`,
    kind: data.kind,
    title: data.title.trim(),
    note: data.note?.trim() || '',
    placeLabel: data.placeLabel?.trim() || 'Точка на карте',
    coords: data.coords,
    startsAt: new Date(now + data.startsInMin * MINUTE).toISOString(),
    endsAt: new Date(now + (data.startsInMin + data.durationMin) * MINUTE).toISOString(),
    createdAt: new Date(now).toISOString(),
    need: data.need,
    going: 1,
    author: { id: user.id, name: user.name },
  };
  store.set((state) => ({ myPins: [pin, ...state.myPins] }));
  return decorate(pin);
}

export async function deletePin(id) {
  requireUser();
  await delay(200);
  store.set((state) => ({ myPins: state.myPins.filter((pin) => pin.id !== id) }));
}

export async function setGoing(id, going) {
  requireUser();
  await delay(200);
  store.set((state) => ({
    joinedPins: going ? [...new Set([...state.joinedPins, id])] : state.joinedPins.filter((pinId) => pinId !== id),
  }));
  return getPin(id);
}

export async function reportPin(id, reason) {
  await delay(300);
  console.info('Жалоба на метку отправлена (демо)', { id, reason });
  return { ok: true };
}
