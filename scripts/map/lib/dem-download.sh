#!/usr/bin/env bash
# Докачка одного DEM-тайла: $1 — URL, $2 — локальный файл.
# -C - догоняет обрыв; код 33 — файл уже докачан. 404/410 — тайла нет
# в источнике (сплошное море): мозаика собирается с дырой, gdal заполняет
# её нулями (уровень моря).
url="$1"; out="$2"
[ -f "$out.dl-ok" ] && exit 0
[ -f "$out.missing" ] && exit 0
if curl -fSL -C - --retry 8 --retry-all-errors --retry-delay 3 -o "$out" "$url"; then
  touch "$out.dl-ok"
  exit 0
fi
code=$?
if [ "$code" -eq 33 ]; then
  touch "$out.dl-ok"
  exit 0
fi
http=$(curl -s -o /dev/null -w '%{http_code}' --retry 3 "$url")
if [ "$http" = "404" ] || [ "$http" = "410" ]; then
  echo "пропускаем отсутствующий тайл (море): $(basename "$url")"
  touch "$out.missing"
  exit 0
fi
exit "$code"
