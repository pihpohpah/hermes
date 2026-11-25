# HTTP API

Маршруты регистрируются в `internal/application/routes/router.go`. Мутации
требуют сессии, кроме отмеченных в [TODO](../TODO.md).

## Прогноз и оценка условий

- `GET /forecast/place/{id}?activity=sup&date=YYYY-MM-DD&days=10` — почасовые
  оценки, `sources` (источник, доступность, demo), `coverage`,
  `contributions`, `warnings` и `limitations`. `days` от 1 до 10, по умолчанию
  `1` (только `date` или сегодня).
- `GET /forecast/picks?activity=&date=` — места, ранжированные по условиям на
  день: «куда поехать по погоде».
- `POST /forecast/refresh` — перезагрузить погоду и пересчитать оценки всех
  мест за день по выбранным активностям.

Как устроены данные и какие у них ограничения — в
[weather-data-coverage.md](weather-data-coverage.md).

## Радар «Сейчас»

Радар показывает временные метки жителей: встречи, спорт, помощь. Метка живёт
от `starts_at` до `ends_at`, автор считается идущим
(`going_total = count(pin_goings) + 1`). Чтение доступно анонимно, а вошедшему
пользователю ответ дополняется полями `joined`, `mine` и `going_total`.

- `GET /pins?bbox=minLon,minLat,maxLon,maxLat&kind=sport` — активные метки во вьюпорте
- `GET /pins/{id}` · `POST /pins` · `DELETE /pins/{id}` (только автор)
- `PUT /pins/{id}/going` / `DELETE` — «Я приду»
- `POST /pins/{id}/reports` — жалоба; после `PINS_REPORT_HIDE_THRESHOLD` жалоб метка скрывается

Модерация задаётся окружением:

| Переменная | По умолчанию |
| :--- | :--- |
| `PINS_MAX_ACTIVE_PER_USER` | `5` (0 — без лимита) |
| `PINS_MAX_PER_HOUR` | `10` (0 — без лимита) |
| `PINS_STOP_WORDS` | пусто, список через запятую |
| `PINS_NIGHT_BAN_FROM` / `PINS_NIGHT_BAN_TO` | пусто (выкл), локальное время `HH:MM` |
| `PINS_REPORT_HIDE_THRESHOLD` | `3` (0 — выкл) |
| `PINS_MAX_DURATION_H` | `12` |
| `PINS_PURGE_GRACE_H` | `24`, используется `pins_cleanup` |

## Каталог

Карточка места содержит категорию по сценарию, район, адрес, часы, цену,
сезон, доступ, теги, фото с атрибуцией и тип берега. Место и событие
адресуются UUID или slug.

- `GET /places?q=&category=&layer=&type=&lat=&lon=&radius_m=` — поиск по названию, описанию, адресу, району и тегам
- `GET /places/{id}` · `GET /places/{id}/similar?limit=6`
- `GET /places/stats?scenario=sea` — счётчики для вкладок: `category_counts`, `type_counts`
- `POST /places` — создать место
- `GET /events?place_id=&category=&q=&all=1` — по умолчанию только предстоящие, `all=1` добавляет прошедшие
- `GET /events/{id}`
- `GET /collections?scenario=` · `GET /collections/{id}` — редакционные подборки с элементами

## Предложения пользователей

Предложения новых мест, событий и правок проходят редакцию:
`review → published/rejected`. Автор видит только свои. Повторная отправка с
тем же заголовком `Idempotency-Key` и тем же телом возвращает сохранённую
запись (200 вместо 201), с другим телом — 409. Тип `fix` требует `fix_id` и
`fix_reasons`, тип `event` — `date`.

- `POST /submissions` · `GET /submissions?status=review` · `GET /submissions/{id}`

## Фото

`POST /photos` принимает multipart с полем `file`: JPG, PNG или WebP не больше
`PHOTOS_MAX_UPLOAD_MB` (по умолчанию 10 МБ).

Файл сразу сохраняется в `PHOTOS_STORAGE_DIR` (по умолчанию
`<WEB_DIR>/uploads/photos/гггг-мм/{id}.jpg`) двумя JPEG-вариантами: полным
(сторона до 1600 px) и миниатюрой (до 640 px). В таблице `photos` появляется
запись со статусом `active`: id в формате UUIDv7 совпадает с именем файла,
сохраняются размеры, `sha256` и автор из сессии.

Ответ — `201 {id, status, url, url_sm, width, height}`. Ошибки: 400 — не
изображение, 401 — нет входа, 403 — нет CSRF-токена, 413 — превышен лимит.

Фото прикрепляются к предложению полем `photos[]` (`src`, `src_sm`, `width`,
`height`) и показываются в профиле миниатюрой.
