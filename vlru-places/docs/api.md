# Данные и API

Сейчас данные лежат в `data/*.json`, а функции в `src/js/api/` фильтруют их на клиенте с задержкой 120–700 мс.
Экраны вызывают только эти функции, поэтому backend подключается заменой реализации.

## Сущности

### Place — место

```jsonc
{
  "id": "tokarevskiy-mayak",            // slug, он же часть URL
  "type": "place",
  "scenario": "city | nature | sea",
  "category": "lighthouses",             // id из src/js/data/taxonomy.js (CATEGORIES)
  "title": "Маяк Токаревского",
  "subtitle": "Эгершельд",               // уточнение или null
  "short": "Тизер до 110 символов",
  "description": ["абзац", "абзац"],
  "district": "Эгершельд",
  "address": "Конец ул. Токаревского",
  "coords": [131.8597, 43.0805],         // [lng, lat], как в GeoJSON
  "hours": "Круглосуточно",
  "price": { "free": true, "label": "Бесплатно" },
  "duration": "1–1,5 часа",
  "season": "Круглый год",
  "access": ["walk", "transit", "car", "ferry", "boat"],
  "howToGet": "Как добраться",
  "facts": ["Короткие советы"],
  "tags": ["закат", "с детьми"],         // для поиска и подборок
  "featured": true,
  "difficulty": "easy | medium | hard",  // природа и часть моря
  "beach": "sand | pebble | rocks | none", // море
  "swimSeason": "Июль — сентябрь",       // море, или null
  "photos": [{
    "src": "assets/photos/x-1.jpg", "srcSm": "assets/photos/x-1-sm.jpg",
    "width": 1400, "height": 933, "alt": "Что на фото",
    "author": "Автор", "license": "CC BY-SA 4.0", "licenseUrl": "https://…", "source": "https://commons.wikimedia.org/…"
  }]
}
```

Клиент добавляет вычисляемое поле `distanceKm`: расстояние по прямой от Центральной площади.

### Event — событие

То же, что место, плюс поля `startsAt` и `endsAt` (ISO с `+10:00`), `placeId` (ссылка на место или null), `venue`, `age`
и `organizer`. Без своих фото событие берёт фото места. Прошедшие события в выдачу не попадают.

### Pin — быстрая метка «Сейчас»

```jsonc
{
  "id": "pin-01",
  "kind": "sport | walk | music | games | help | other",
  "title": "Футбол на Спортивной, нужно ещё 4 человека",
  "note": "До 140 символов",
  "placeLabel": "Поле у стадиона «Динамо»",
  "coords": [131.874, 43.119],
  "startsAt": "ISO", "endsAt": "ISO",     // метка исчезает после endsAt
  "need": 4,                              // сколько ещё нужно, 0 — не важно
  "going": 6,                             // сколько уже идёт
  "author": { "id": "u-…", "name": "Артём" },
  "createdAt": "ISO"
}
```

Ответ API дополнительно содержит `joined`, `mine` и `goingTotal` для текущего пользователя.
В демо-файле `data/pins.json` время задано относительно загрузки (`startsInMin` и `durationMin`), а координаты — через
центр района (`areas`). Так метки всегда выглядят свежими.

### History — сигналы для рекомендаций

```jsonc
{ "viewed": ["id", …], "visited": ["id", …], "searches": ["водопады", …] }  // новые — в начале
```

`viewed` — открытые карточки (до 30), `visited` — отметки «Уже были здесь» на странице места, `searches` — поисковые
запросы от трёх букв (до 10). Продолжение последнего запроса заменяет его, поэтому ввод «вод» → «водопады» даёт одну
запись. В демо история лежит в `store.history` и копится и без входа. Начальное значение — демо-профиль, чтобы лента
«Для вас» не была пустой; «Сбросить всё» в профиле возвращает его.

### User и Submission

```jsonc
{ "id": "u-9141234567", "name": "Аня", "phone": "+79141234567", "createdAt": "ISO" }
{ "id": "s-…", "type": "place | event | fix", "status": "review | published | rejected", "createdAt": "ISO",
  "scenario": "city", "category": "concert", "title": "…", "address": "…", "coords": [lng, lat] | null,
  "date": "2026-12-05", "time": "19:00", "description": "…", "hours": "…", "price": "…", "link": "…",
  "fixId": "id исправляемого объекта", "fixReasons": ["Изменились часы"], "contact": "почта или телефон",
  "photos": ["имена файлов"] }
```

## Функции демо-API

| Модуль | Функция | Эндпоинт для продакшена |
|---|---|---|
| `api/places.js` | `listItems({ scenario, type, q, category, filters, collection, sort })` → `{ items, total, categoryCounts, typeCounts }` | `GET /places?…` |
| | `getItem(id)` | `GET /places/:id` |
| | `getSimilar(item)` · `getEventsAt(placeId)` · `getUpcomingEvents()` | `GET /places/:id/similar` и т. п. |
| | `getScenarioCounts()` · `getCollectionCounts(scenario)` | агрегаты для вкладок и подборок |
| `api/pins.js` | `listPins({ kind })` · `getPin(id)` | `GET /pins?bbox=…` |
| | `createPin(data)` · `deletePin(id)` | `POST /pins`, `DELETE /pins/:id` |
| | `setGoing(id, going)` | `PUT/DELETE /pins/:id/going` |
| | `reportPin(id, reason)` | `POST /pins/:id/reports` |
| `api/auth.js` | `requestCode(phone)` · `verifyCode(phone, code)` · `register(phone, name)` · `updateProfile()` · `logout()` | SSO VL.ru |
| `api/submissions.js` | `submitProposal(data)` | `POST /submissions` (multipart с фото) |
| `api/history.js` | `trackView(id)` · `trackSearch(q)` · `toggleVisited(id)` · `isVisited(id)` | `POST /me/events`, `PUT/DELETE /me/visited/:id` |
| `api/recommendations.js` | `getRecommendations({ limit })` → `{ places, events, routes }` | `GET /me/recommendations` |
| `api/lucky.js` | `rollLucky({ type, scenario, stops, exclude })` · `getLuckyPool(scenario)` | `GET /lucky?type&scenario&stops` |

Ошибки бросаются как `ApiError` с полем `code`: `auth_required`, `validation` или `not_found`. Экраны показывают
`message` пользователю, поэтому сообщения сервера должны быть на русском и понятными.

## Рекомендации и «Мне повезёт»

Обе функции — макеты поведения для прототипа, их логика намеренно простая. На сервере её заменит настоящая модель,
а контракт ответа останется.

- **`getRecommendations`.** Мок: честный случайный выбор без фильтров и без учёта истории. Кандидаты — все актуальные
  места и события сценария, перемешанные и обрезанные до лимита (мест — до `limit`, событий — 3). Персонализированные
  причины в подписях не выдумываем: у записей `{ item, reason }` пустая причина. Маршруты `{ id, scenario, title, reason, stops }`
  собираются из случайных мест сценария, причём каждая следующая точка — ближайшая к предыдущей (в городе 4 точки,
  за городом 3). Это временная заглушка: в продакшене ленту отдаёт рекомендательный сервис (см. TODO в модуле).
- **`rollLucky`.** Это честный случайный выбор среди актуальных объектов сценария, история не учитывается. `type: 'any'`
  сначала случайно выбирает тип. Маршрут начинается со случайного места, а каждая следующая точка — ближайшая
  к предыдущей; из трёх ближайших предпочитается другая категория. Ответ:
  `{ type: 'place' | 'event', item }` или `{ type: 'route', stops: [{ item, fromPreviousKm }] }`. Если в сценарии
  нечего выдать (например, нет событий), возвращается `null`.

## Фильтры

Набор фильтров по сценарию задан в `FILTERS` (`src/js/data/taxonomy.js`), а логика — в `matchesFilters` (`api/places.js`).

| Сценарий | Фильтры |
|---|---|
| Город | бесплатно, пешком от центра (≤ 2,5 км), можно с детьми (тег) |
| Природа | расстояние от центра, сложность, без машины (`walk`, `transit` или `ferry`), бесплатно |
| Море | тип берега, расстояние, купальный сезон, без машины |

Подборки справочника (`COLLECTIONS`) — сохранённые запросы-предикаты, а не отдельные сущности. На сервере это могут быть
редакционные списки.
