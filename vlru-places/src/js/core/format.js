/**
 * Форматирование чисел, дат и расстояний по-русски.
 * Все даты в данных хранятся в ISO с часовым поясом Владивостока (+10:00) и показываются в нём же.
 */

const TIME_ZONE = 'Asia/Vladivostok';
const numberFormat = new Intl.NumberFormat('ru-RU');
const decimalFormat = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 1 });

/** plural(5, ['место', 'места', 'мест']) → 'мест' */
export function plural(count, forms) {
  const n = Math.abs(count) % 100;
  const n1 = n % 10;
  if (n > 10 && n < 20) return forms[2];
  if (n1 > 1 && n1 < 5) return forms[1];
  if (n1 === 1) return forms[0];
  return forms[2];
}

export function countLabel(count, forms) {
  return `${numberFormat.format(count)} ${plural(count, forms)}`;
}

export function formatDistance(km) {
  if (km < 1) return `${Math.max(50, Math.round((km * 1000) / 50) * 50)} м`;
  if (km < 10) return `${decimalFormat.format(Math.round(km * 10) / 10)} км`;
  return `${numberFormat.format(Math.round(km))} км`;
}

export function formatDuration(minutes) {
  const total = Math.max(1, Math.round(minutes));
  if (total < 60) return `${total} мин`;
  const hours = Math.floor(total / 60);
  const rest = total % 60;
  if (hours >= 24) {
    const days = Math.round(hours / 24);
    return `${days} ${plural(days, ['день', 'дня', 'дней'])}`;
  }
  return rest ? `${hours} ч ${rest} мин` : `${hours} ч`;
}

const dayMonth = new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long', timeZone: TIME_ZONE });
const weekdayShort = new Intl.DateTimeFormat('ru-RU', { weekday: 'short', timeZone: TIME_ZONE });
const timeFormat = new Intl.DateTimeFormat('ru-RU', { hour: '2-digit', minute: '2-digit', timeZone: TIME_ZONE });
const dayOnly = new Intl.DateTimeFormat('ru-RU', { day: 'numeric', timeZone: TIME_ZONE });
const monthNumber = new Intl.DateTimeFormat('ru-RU', { month: 'numeric', timeZone: TIME_ZONE });
const MONTHS_SHORT = ['янв', 'фев', 'мар', 'апр', 'мая', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек'];
const yearOnly = new Intl.DateTimeFormat('ru-RU', { year: 'numeric', timeZone: TIME_ZONE });

const capitalize = (value) => value.charAt(0).toUpperCase() + value.slice(1);

export const formatTime = (iso) => timeFormat.format(new Date(iso));

/** «21 ноября» (+ год, если не текущий) */
export function formatDay(iso, now = new Date()) {
  const date = new Date(iso);
  const label = dayMonth.format(date);
  return yearOnly.format(date) === yearOnly.format(now) ? label : `${label} ${yearOnly.format(date)}`;
}

/** «Сб, 21 ноября · 19:30» */
export function formatEventWhen(startIso, now = new Date()) {
  const date = new Date(startIso);
  return `${capitalize(weekdayShort.format(date))}, ${formatDay(startIso, now)} · ${formatTime(startIso)}`;
}

/** «21 ноя» для маркера события */
export function formatDayShort(iso) {
  const { day, month } = dateTileParts(iso);
  return `${day} ${month}`;
}

/** Для плитки даты: { day: '21', month: 'ноя' } */
export function dateTileParts(iso) {
  const date = new Date(iso);
  return { day: dayOnly.format(date), month: MONTHS_SHORT[Number(monthNumber.format(date)) - 1] };
}

/**
 * Статус быстрой метки относительно текущего времени.
 * → { state: 'soon' | 'live' | 'ended', label: 'через 25 мин' | 'идёт, ещё 40 мин' | 'завершилась' }
 */
export function pinTiming(startsAt, endsAt, now = Date.now()) {
  const start = new Date(startsAt).getTime();
  const end = new Date(endsAt).getTime();
  if (now < start) {
    const minutes = Math.round((start - now) / 60000);
    return { state: 'soon', label: minutes < 1 ? 'начинается' : `через ${formatDuration(minutes)}`, short: minutes < 1 ? 'сейчас' : formatDuration(minutes) };
  }
  if (now < end) {
    const minutes = Math.round((end - now) / 60000);
    return { state: 'live', label: `идёт, ещё ${formatDuration(minutes)}`, short: 'идёт' };
  }
  return { state: 'ended', label: 'завершилась', short: 'всё' };
}

/** «5 минут назад» для времени публикации */
export function formatAgo(iso, now = Date.now()) {
  const minutes = Math.round((now - new Date(iso).getTime()) / 60000);
  if (minutes < 1) return 'только что';
  if (minutes < 60) return `${minutes} ${plural(minutes, ['минуту', 'минуты', 'минут'])} назад`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours} ${plural(hours, ['час', 'часа', 'часов'])} назад`;
  return formatDay(iso);
}

export function initials(name = '') {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0].toUpperCase())
    .join('');
}

/** Цвет аватара по имени — стабильный, из мягкой палитры токенов. */
const AVATAR_TONES = [
  ['var(--orange-100)', 'var(--orange-800)'],
  ['var(--violet-100)', 'var(--violet-700)'],
  ['var(--blue-100)', 'var(--blue-600)'],
  ['var(--green-100)', 'var(--green-600)'],
  ['var(--yellow-100)', 'var(--yellow-500)'],
  ['var(--red-100)', 'var(--red-500)'],
];

export function avatarStyle(name = '') {
  let hash = 0;
  for (const ch of name) hash = (hash * 31 + ch.charCodeAt(0)) >>> 0;
  const [bg, fg] = AVATAR_TONES[hash % AVATAR_TONES.length];
  return `--avatar-bg:${bg};--avatar-fg:${fg}`;
}
