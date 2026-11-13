/**
 * Справочники продукта: сценарии, категории, типы меток, фильтры и подборки.
 * Это единственное место, где задаются подписи и иконки — view берут их отсюда.
 * Иконки — имена из assets/icons.svg (Lucide).
 */

export const SCENARIOS = [
  { id: 'city', label: 'Город', icon: 'building-2', description: 'Улицы, музеи, сцены и андеграунд' },
  { id: 'nature', label: 'Природа', icon: 'trees', description: 'Сопки, водопады, озёра и тропы' },
  { id: 'sea', label: 'Море', icon: 'waves', description: 'Бухты, пляжи, маяки и острова' },
];

export const SCENARIO_BY_ID = Object.fromEntries(SCENARIOS.map((s) => [s.id, s]));

export const CATEGORIES = {
  city: [
    { id: 'walks', label: 'Прогулки', icon: 'footprints' },
    { id: 'viewpoints', label: 'Смотровые', icon: 'binoculars' },
    { id: 'history', label: 'История', icon: 'castle' },
    { id: 'museums', label: 'Музеи', icon: 'landmark' },
    { id: 'art', label: 'Арт-пространства', icon: 'palette' },
    { id: 'stage', label: 'Музыка и сцена', icon: 'music' },
  ],
  nature: [
    { id: 'hiking', label: 'Сопки и тропы', icon: 'mountain' },
    { id: 'waterfalls', label: 'Водопады', icon: 'droplets' },
    { id: 'lakes', label: 'Озёра', icon: 'fish' },
    { id: 'parks', label: 'Парки и сады', icon: 'flower-2' },
    { id: 'capes', label: 'Мысы и скалы', icon: 'compass' },
    { id: 'reserves', label: 'Заповедные места', icon: 'leaf' },
  ],
  sea: [
    { id: 'beaches', label: 'Пляжи', icon: 'sun' },
    { id: 'bays', label: 'Бухты', icon: 'sailboat' },
    { id: 'lighthouses', label: 'Маяки', icon: 'lighthouse' },
    { id: 'islands', label: 'Острова', icon: 'shell' },
    { id: 'diving', label: 'Дайвинг', icon: 'anchor' },
    { id: 'boats', label: 'Морские прогулки', icon: 'ship' },
  ],
};

export const EVENT_CATEGORIES = {
  concert: { label: 'Концерт', icon: 'music' },
  exhibition: { label: 'Выставка', icon: 'palette' },
  tour: { label: 'Экскурсия', icon: 'footprints' },
  festival: { label: 'Фестиваль', icon: 'party-popper' },
  sport: { label: 'Спорт', icon: 'trophy' },
  party: { label: 'Андеграунд', icon: 'mic-vocal' },
  lecture: { label: 'Лекция', icon: 'presentation' },
  market: { label: 'Маркет', icon: 'shopping-bag' },
};

export const PIN_KINDS = [
  { id: 'sport', label: 'Спорт', icon: 'volleyball', example: 'Футбол 5 на 5, нужно ещё двое' },
  { id: 'walk', label: 'Прогулка', icon: 'footprints', example: 'Идём на закат к маяку' },
  { id: 'music', label: 'Музыка', icon: 'guitar', example: 'Джем на Фокина, берите инструменты' },
  { id: 'games', label: 'Игры', icon: 'gamepad-2', example: 'Настолки в кофейне, есть свободные места' },
  { id: 'help', label: 'Помощь', icon: 'hand-helping', example: 'Нужно донести диван до 3 этажа' },
  { id: 'other', label: 'Другое', icon: 'message-circle', example: 'Кто со мной на фотовыставку?' },
];

export const PIN_KIND_BY_ID = Object.fromEntries(PIN_KINDS.map((k) => [k.id, k]));

export const ACCESS = {
  walk: { label: 'Пешком', icon: 'footprints' },
  transit: { label: 'Общественный транспорт', icon: 'bus' },
  car: { label: 'На машине', icon: 'car' },
  ferry: { label: 'Паром', icon: 'ship' },
  boat: { label: 'Катер или лодка', icon: 'sailboat' },
};

export const DIFFICULTY = {
  easy: 'Лёгкая',
  medium: 'Средняя',
  hard: 'Сложная',
};

export const BEACH = {
  sand: 'Песок',
  pebble: 'Галька',
  rocks: 'Скалы',
  none: 'Нет пляжа',
};

/** Расстояния для фильтра, км от центра */
const DISTANCE_OPTIONS = [
  { value: '', label: 'Любое' },
  { value: '30', label: 'До 30 км' },
  { value: '100', label: 'До 100 км' },
  { value: '250', label: 'До 250 км' },
];

/**
 * Фильтры по сценарию. type:
 *   toggle — да/нет; single — один вариант; multi — несколько вариантов.
 * Логика применения — src/js/api/places.js (matchesFilters).
 */
export const FILTERS = {
  city: [
    { id: 'free', type: 'toggle', label: 'Бесплатно' },
    { id: 'walkable', type: 'toggle', label: 'Пешком от центра', hint: 'Не дальше 2,5 км от Центральной площади' },
    { id: 'kids', type: 'toggle', label: 'Можно с детьми' },
  ],
  nature: [
    { id: 'distance', type: 'single', label: 'Расстояние от центра', options: DISTANCE_OPTIONS },
    {
      id: 'difficulty',
      type: 'multi',
      label: 'Сложность',
      options: Object.entries(DIFFICULTY).map(([value, label]) => ({ value, label })),
    },
    { id: 'noCar', type: 'toggle', label: 'Можно без машины', hint: 'Автобус, электричка или паром' },
    { id: 'free', type: 'toggle', label: 'Бесплатно' },
  ],
  sea: [
    {
      id: 'beach',
      type: 'multi',
      label: 'Берег',
      options: ['sand', 'pebble', 'rocks'].map((value) => ({ value, label: BEACH[value] })),
    },
    { id: 'distance', type: 'single', label: 'Расстояние от центра', options: DISTANCE_OPTIONS },
    { id: 'swim', type: 'toggle', label: 'Можно купаться', hint: 'Есть купальный сезон' },
    { id: 'noCar', type: 'toggle', label: 'Можно без машины', hint: 'Автобус, электричка или паром' },
  ],
};

export const SORTS = [
  { id: 'recommended', label: 'Сначала интересные' },
  { id: 'distance', label: 'Ближе к центру' },
  { id: 'title', label: 'По алфавиту' },
];

export const TYPE_FILTERS = [
  { id: 'all', label: 'Всё' },
  { id: 'place', label: 'Места' },
  { id: 'event', label: 'События' },
];

/**
 * Подборки в справочнике — сохранённые запросы, а не отдельные сущности.
 * match получает объект (место или событие) и возвращает true, если он входит в подборку.
 */
export const COLLECTIONS = [
  {
    id: 'sunsets',
    title: 'Закаты и виды',
    cover: 'orlinoe-gnezdo',
    match: (item) => item.category === 'viewpoints' || hasTag(item, ['закат', 'вид', 'смотровая']),
  },
  {
    id: 'underground',
    title: 'Андеграунд и арт',
    cover: 'tssi-zarya',
    match: (item) => ['art', 'stage', 'party', 'concert', 'exhibition', 'market'].includes(item.category) || hasTag(item, ['андеграунд', 'инди', 'панк']),
  },
  {
    id: 'kids',
    title: 'Можно с детьми',
    cover: 'okeanarium',
    match: (item) => hasTag(item, ['с детьми']),
  },
  {
    id: 'trip',
    title: 'Выбраться на выходные',
    cover: 'gora-pidan',
    match: (item) => hasTag(item, ['однодневка', 'выходные', 'палатка', 'поход']) || (item.distanceKm ?? 0) > 60,
  },
];

function hasTag(item, tags) {
  return (item.tags || []).some((tag) => tags.includes(tag));
}

export function categoryOf(item) {
  if (item.type === 'event') return { id: item.category, ...EVENT_CATEGORIES[item.category] };
  return (CATEGORIES[item.scenario] || []).find((c) => c.id === item.category) || { id: item.category, label: item.category, icon: 'map-pin' };
}
