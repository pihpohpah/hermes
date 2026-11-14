/* Справочники (зеркало presenter/taxonomy.go для JS-диалогов). */

export const SCENARIO_LABELS = { city: 'Город', nature: 'Природа', sea: 'Море' };

export const CATEGORY_LABELS = {
  city: { walks: 'Прогулки', viewpoints: 'Смотровые', history: 'История', museums: 'Музеи', art: 'Арт-пространства', stage: 'Музыка и сцена' },
  nature: { hiking: 'Сопки и тропы', waterfalls: 'Водопады', lakes: 'Озёра', parks: 'Парки и сады', capes: 'Мысы и скалы', reserves: 'Заповедные места' },
  sea: { beaches: 'Пляжи', bays: 'Бухты', lighthouses: 'Маяки', islands: 'Острова', diving: 'Дайвинг', boats: 'Морские прогулки' },
  event: { concert: 'Концерт', exhibition: 'Выставка', tour: 'Экскурсия', festival: 'Фестиваль', sport: 'Спорт', party: 'Андеграунд', lecture: 'Лекция', market: 'Маркет' },
};

export const TYPE_LABELS = { place: 'Место', event: 'Событие', fix: 'Правка карточки' };

export const DISTANCE_OPTIONS = [
  { value: '', label: 'Любое' },
  { value: '30', label: 'До 30 км' },
  { value: '100', label: 'До 100 км' },
  { value: '250', label: 'До 250 км' },
];

export const DIFFICULTY_OPTIONS = [
  { value: 'easy', label: 'Лёгкая' },
  { value: 'medium', label: 'Средняя' },
  { value: 'hard', label: 'Сложная' },
];

export const BEACH_OPTIONS = [
  { value: 'sand', label: 'Песок' },
  { value: 'pebble', label: 'Галька' },
  { value: 'rocks', label: 'Скалы' },
];

// Зеркало presenter.Filters: набор полей расширенных фильтров по сценарию.
export const FILTERS = {
  city: [
    { id: 'free', type: 'toggle', label: 'Бесплатно' },
    { id: 'walkable', type: 'toggle', label: 'Пешком от центра', hint: 'Не дальше 2,5 км от Центральной площади' },
    { id: 'kids', type: 'toggle', label: 'Можно с детьми' },
  ],
  nature: [
    { id: 'distance', type: 'single', label: 'Расстояние от центра', options: DISTANCE_OPTIONS },
    { id: 'difficulty', type: 'multi', label: 'Сложность', options: DIFFICULTY_OPTIONS },
    { id: 'noCar', type: 'toggle', label: 'Можно без машины', hint: 'Автобус, электричка или паром' },
    { id: 'free', type: 'toggle', label: 'Бесплатно' },
  ],
  sea: [
    { id: 'beach', type: 'multi', label: 'Берег', options: BEACH_OPTIONS },
    { id: 'distance', type: 'single', label: 'Расстояние от центра', options: DISTANCE_OPTIONS },
    { id: 'swim', type: 'toggle', label: 'Можно купаться', hint: 'Есть купальный сезон' },
    { id: 'noCar', type: 'toggle', label: 'Можно без машины', hint: 'Автобус, электричка или паром' },
  ],
};

export const FILTER_IDS = ['free', 'walkable', 'kids', 'distance', 'difficulty', 'noCar', 'beach', 'swim'];
