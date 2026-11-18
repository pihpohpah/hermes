#!/usr/bin/env bash
# Упаковка тайлов в PMTiles: каталог тайлов -> MBTiles -> PMTiles -> verify.
# Используется оркестраторами после шагов-нарезки.
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

# tiles_to_mbtiles <каталог-в-WORK_DIR> <mbtiles-в-WORK_DIR> <имя> [pbf] [tilejson]
tiles_to_mbtiles() {
  log "Пакуем тайлы $1 -> $2"
  need_docker
  docker run --rm -v "$WORK_DIR:/work" -v "$MAP_LIB_DIR:/pack:ro" \
    -e TILES_ROOT="/work/$1" -e MBTILES_OUT="/work/$2" -e META_NAME="$3" \
    -e BBOX="$BBOX" -e TILE_EXT="${4:-png}" -e META_JSON="${5:-}" \
    "$GDAL_IMAGE" python3 /pack/mbtiles.py \
    || fail "не удалось собрать MBTiles из $1"
}

# TileJSON для векторных контуров: vector_layers обязателен для pmtiles-протокола.
contours_tilejson() {
  printf '{"vector_layers":[{"id":"contours","fields":{"ELEV":"Number","kind":"String"}}]}'
}

# to_pmtiles <mbtiles-в-WORK_DIR> <итоговый-pmtiles-абсолютный-путь>
to_pmtiles() {
  log "Конвертируем $1 -> $(basename "$2")"
  mkdir -p "$(dirname "$2")"
  docker run --rm -v "$WORK_DIR:/data" -v "$(dirname "$2"):/out" "$PMTILES_IMAGE" \
    convert --force "/data/$1" "/out/$(basename "$2")" >/dev/null \
    || fail "PMTiles не создан: $2"
  [ -f "$2" ] || fail "PMTiles не создан: $2"
  rm -f "$WORK_DIR/$1"
  docker run --rm -v "$(dirname "$2"):/out" "$PMTILES_IMAGE" \
    verify "/out/$(basename "$2")" >/dev/null \
    || fail "архив не прошёл проверку: $2"
}

# report <артефакты...> — размер и ключевые параметры каждого архива.
report() {
  local target
  for target in "$@"; do
    [ -f "$target" ] || continue
    log "Готово: $target ($(du -h "$target" | cut -f1))"
    docker run --rm -v "$(dirname "$target"):/out" "$PMTILES_IMAGE" \
      show "/out/$(basename "$target")" | grep -E 'bounds|min zoom|max zoom|addressed tiles|format'
  done
}
