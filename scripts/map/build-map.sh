#!/usr/bin/env bash
# Собирает офлайн-архив карты Приморского края (PMTiles) из OSM-экстракта.
#
# Пайплайн:
#   1. скачать экстракт Дальневосточного ФО с Geofabrik (кэшируется)
#   2. вырезать bbox Приморья через osmium extract
#   3. отрисовать векторные тайлы tilemaker'ом (профиль OpenMapTiles + правки)
#   4. собрать PMTiles и проверить архив
#
# Внешние зависимости: docker (образы подтягиваются автоматически).
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PROFILE_DIR="$ROOT_DIR/scripts/map"
BBOX="${MAP_BBOX:-130.0,42.0,140.0,49.0}"
MAXZOOM="${MAP_MAXZOOM:-14}"
OUTPUT="${MAP_OUTPUT:-$ROOT_DIR/web/static/pmtiles/primorsky-krai-z${MAXZOOM}.pmtiles}"
CACHE_DIR="${MAP_CACHE_DIR:-$ROOT_DIR/.cache/map}"
TOOLS_IMAGE="${MAP_TOOLS_IMAGE:-hermes/map-tools:latest}"
PMTILES_IMAGE="${PMTILES_IMAGE:-ghcr.io/protomaps/go-pmtiles:latest}"
PBF_URL="${MAP_PBF_URL:-https://download.geofabrik.de/russia/far-eastern-fed-district-latest.osm.pbf}"
FAR_EAST_PBF="$CACHE_DIR/far-east.osm.pbf"
CLIPPED_PBF="$CACHE_DIR/primorsky.osm.pbf"
WORK_DIR="$CACHE_DIR/build"
COASTLINE_DIR="$CACHE_DIR/coastline"
COASTLINE_URL="https://osmdata.openstreetmap.de/download/water-polygons-split-4326.zip"
log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31mОШИБКА:\033[0m %s\n' "$*" >&2; exit 1; }
command -v docker >/dev/null 2>&1 || fail "нужен docker: без него не собрать тайлы"
geo_area() { # обрезает Water polygons до региона, чтобы ocean-слой не тянул всю планету
  if [ -f "$COASTLINE_DIR/water_polygons.shp" ]; then
    log "Береговая линия уже подготовлена: $COASTLINE_DIR"
    return
  fi
  mkdir -p "$COASTLINE_DIR"
  local zip="$CACHE_DIR/water-polygons-4326.zip"
  if [ ! -f "$zip" ]; then
    log "Скачиваем береговую линию Osmdata (~870 MB, только при первой сборке)"
    curl -fL --retry 3 --retry-delay 2 -o "$zip" "$COASTLINE_URL"
  fi
  log "Распаковываем береговую линию"
  rm -rf "$CACHE_DIR/water-4326"
  unzip -o -q "$zip" -d "$CACHE_DIR/water-4326"
  log "Обрезаем береговую линию по региону"
  docker run --rm -v "$CACHE_DIR:/data" --entrypoint ogr2ogr \
    ghcr.io/osgeo/gdal:alpine-normal-latest \
    -f "ESRI Shapefile" /data/coastline/water_polygons.shp \
    /data/water-4326/water-polygons-split-4326/water_polygons.shp \
    -clipsrc 128.0 40.0 142.0 51.0
  [ -f "$COASTLINE_DIR/water_polygons.shp" ] || fail "не удалось подготовить береговую линию"
}
build_tools() { # образ tilemaker + osmium: PBF-кроп делает osmium в том же контейнере
  if docker image inspect "$TOOLS_IMAGE" >/dev/null 2>&1; then
    return
  fi
  log "Собираем образ $TOOLS_IMAGE (tilemaker + osmium)"
  DOCKER_BUILDKIT=0 docker build -t "$TOOLS_IMAGE" -f "$PROFILE_DIR/Dockerfile.tools" "$ROOT_DIR"
}
fetch_pbf() {
  if [ -f "$FAR_EAST_PBF" ]; then
    log "Экстракт уже скачан: $FAR_EAST_PBF"
    return
  fi
  log "Скачиваем экстракт Дальневосточного ФО (~400 MB, только при первой сборке)"
  curl -fL --retry 3 --retry-delay 2 -o "$FAR_EAST_PBF.part" "$PBF_URL"
  mv "$FAR_EAST_PBF.part" "$FAR_EAST_PBF"
}
crop_pbf() {
  if [ -f "$CLIPPED_PBF" ] && [ "$CLIPPED_PBF" -nt "$FAR_EAST_PBF" ]; then
    log "Кроп уже готов: $CLIPPED_PBF"
    return
  fi
  log "Вырезаем bbox $BBOX из экстракта"
  docker run --rm -v "$CACHE_DIR:/data" --entrypoint osmium "$TOOLS_IMAGE" \
    extract -b "$BBOX" /data/far-east.osm.pbf -o /data/primorsky.osm.pbf --overwrite
  [ -f "$CLIPPED_PBF" ] || fail "osmium не создал $CLIPPED_PBF"
}
render_tiles() {
  log "Рендерим векторные тайлы z0-$MAXZOOM (tilemaker)"
  rm -rf "$WORK_DIR"
  mkdir -p "$WORK_DIR/coastline"
  cp "$COASTLINE_DIR"/water_polygons.* "$WORK_DIR/coastline/"
  # Монтируем весь кэш: tilemaker читает PBF оттуда и пишет результат в build/.
  docker run --rm \
    -v "$CACHE_DIR:/data" \
    -v "$PROFILE_DIR:/profile:ro" \
    -w /data/build \
    "$TOOLS_IMAGE" \
    --input /data/primorsky.osm.pbf \
    --output /data/build/region.mbtiles \
    --config /profile/config.json \
    --process /profile/process.lua \
    --bbox "$BBOX" \
    --threads "$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"
  [ -f "$WORK_DIR/region.mbtiles" ] || fail "tilemaker не создал MBTiles"
}
to_pmtiles() {
  log "Конвертируем MBTiles -> PMTiles"
  mkdir -p "$(dirname "$OUTPUT")"
  docker run --rm -v "$WORK_DIR:/data" -v "$(dirname "$OUTPUT"):/out" "$PMTILES_IMAGE" \
    convert /data/region.mbtiles "/out/$(basename "$OUTPUT")"
  [ -f "$OUTPUT" ] || fail "PMTiles не создан: $OUTPUT"
  log "Проверяем структуру архива"
  docker run --rm -v "$(dirname "$OUTPUT"):/out" "$PMTILES_IMAGE" verify "/out/$(basename "$OUTPUT")"
}
report() {
  local size_bytes size_human
  size_bytes=$(wc -c <"$OUTPUT" | tr -d ' ')
  size_human=$(du -h "$OUTPUT" | cut -f1)
  log "Готово: $OUTPUT ($size_human)"
  docker run --rm -v "$(dirname "$OUTPUT"):/out" "$PMTILES_IMAGE" show "/out/$(basename "$OUTPUT")" \
    | grep -E "bounds|min zoom|max zoom|addressed tiles"
  if [ "$size_bytes" -gt 524288000 ]; then
    printf '\033[1;33mВНИМАНИЕ:\033[0m файл больше 500 MB (%s).\n' "$size_human"
    cat <<'HINT'
Варианты уменьшения:
  - MAP_MAXZOOM=12 make map          # грубая карта всего края
  - два архива: z12 на весь край + z14 на агломерацию Владивостока
    (см. README, раздел «Если архив больше 500 MB»)
  - zstd: архив PMTiles не сжимается повторно, выигрыш даёт только LOD-схема
HINT
  fi
}
main() {
  mkdir -p "$CACHE_DIR"
  build_tools
  fetch_pbf
  crop_pbf
  geo_area
  render_tiles
  to_pmtiles
  report
}
main "$@"
