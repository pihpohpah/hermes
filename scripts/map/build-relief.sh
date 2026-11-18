#!/usr/bin/env bash
# Оркестратор рельефа: собирает три PMTiles-архива из Copernicus DEM GLO-30.
#
#   primorsky-relief.pmtiles    — высотный градиент (PNG) с запечённой отмывкой
#   primorsky-contours.pmtiles  — горизонтали 20/100 м (MVT z5-14): кольца
#                                 рисует клиент, гладкие на любом зуме
#   primorsky-dem.pmtiles       — DEM в terrarium-RGB для terrain (не
#                                 используется приложением; RELIEF_SKIP_DEM=1
#                                 чтобы пропустить)
#
# Шаги — отдельные файлы в scripts/map/relief/ (см. заголовки шагов),
# общая логика — в scripts/map/lib/. Лог каждой строки — с меткой времени,
# по каждому шагу — затраченное время.
#
# Переменные окружения: RELIEF_BBOX, RELIEF_MINZOOM/MAXZOOM, DEM_MAXZOOM,
# RELIEF_OUTPUT, RELIEF_CONTOURS_OUTPUT, DEM_OUTPUT, RELIEF_REBUILD_DEM=1,
# RELIEF_SKIP_DEM=1, RELIEF_BOUNDARY, MAP_CACHE_DIR.
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
source "$MAP_LIB_DIR/pmtiles.sh"

RELIEF_DIR="$MAP_DIR/relief"

# Мозаика DEM дорогая: переиспользуем, если bbox и разрешение совпадают.
dem_ready() {
  [ "${RELIEF_REBUILD_DEM:-0}" != "1" ] \
    && [ -f "$WORK_DIR/dem/krai_dem.tif" ] \
    && [ -f "$WORK_DIR/dem/bbox.txt" ] \
    && [ -f "$WORK_DIR/dem/resolution.txt" ] \
    && [ "$(cat "$WORK_DIR/dem/bbox.txt")" = "$BBOX" ] \
    && [ "$(cat "$WORK_DIR/dem/resolution.txt")" = "$DEM_TR" ]
}

# run_step <номер> <всего> <название> <команда...>
run_step() {
  local num="$1" total="$2" name="$3"
  shift 3
  log "── шаг $num/$total: $name ──"
  local t0=$SECONDS
  if "$@"; then
    local s=$((SECONDS - t0))
    log "── шаг $num/$total: $name — готово за $((s / 60)) мин $((s % 60)) с ──"
  else
    fail "шаг $num/$total '$name' завершился с ошибкой (лог выше)"
  fi
}

main() {
  mkdir -p "$WORK_DIR/relief" "$WORK_DIR/dem"
  need_docker

  local steps=12
  [ "${RELIEF_SKIP_DEM:-0}" = "1" ] || steps=14

  if dem_ready; then
    log "DEM-мозаика уже собрана для bbox $BBOX — пропускаем шаг 1/$steps (RELIEF_REBUILD_DEM=1 для пересборки)"
  else
    run_step 1 "$steps" "Мозаика DEM (скачивание GLO-30)" \
      bash "$RELIEF_DIR/10-dem-mosaic.sh"
  fi
  run_step 2 "$steps" "Сглаживание DEM (гауссиан)"      run_py relief/20-smooth.py
  run_step 2.5 "$steps" "Контур суши (граница - вода)"  bash "$RELIEF_DIR/15-land-cutline.sh"
  run_step 3 "$steps" "Контуры 20/100 м (gdal_contour)" bash "$RELIEF_DIR/30-contours.sh"
  run_step 4 "$steps" "Сглаживание линий (Чайкин)"      bash "$RELIEF_DIR/40-chaikin-run.sh"
  run_step 5 "$steps" "Слияние контуров + kind"         bash "$RELIEF_DIR/50-contours-merge.sh"
  run_step 5.5 "$steps" "Обрезка контуров по границе края" bash "$RELIEF_DIR/52-contours-clip.sh"
  run_step 6 "$steps" "Отмывка и цвет (сетка x2)"       bash "$RELIEF_DIR/60-relief-raster.sh"
  run_step 7 "$steps" "Смешивание цвета и отмывки"      gsh "python3 /scripts/relief/70-blend.py"
  run_step 8 "$steps" "Обрезка по границе края"         bash "$RELIEF_DIR/80-mask.sh"
  run_step 9 "$steps" "Тайлы рельефа z$MIN_ZOOM-$MAX_ZOOM" bash "$RELIEF_DIR/90-relief-tiles.sh"
  run_step 10 "$steps" "Упаковка relief.pmtiles" \
    bash -c "source '$MAP_LIB_DIR/pmtiles.sh'; tiles_to_mbtiles relief/tiles relief/relief.mbtiles 'Приморский край · рельеф'; to_pmtiles relief/relief.mbtiles '$OUTPUT'"
  run_step 11 "$steps" "MVT контуров z$CONTOURS_MIN_ZOOM-$CONTOURS_MAX_ZOOM" \
    bash "$RELIEF_DIR/95-contours-mvt.sh"
  run_step 12 "$steps" "Упаковка contours.pmtiles" \
    bash -c "source '$MAP_LIB_DIR/pmtiles.sh'; tiles_to_mbtiles relief/contours_mvt relief/contours.mbtiles 'Приморский край · горизонтали' pbf \"\$(contours_tilejson)\"; to_pmtiles relief/contours.mbtiles '$OUTPUT_CONTOURS'"
  if [ "${RELIEF_SKIP_DEM:-0}" = "1" ]; then
    report "$OUTPUT" "$OUTPUT_CONTOURS"
    return
  fi
  run_step 13 "$steps" "Terrarium DEM z$MIN_ZOOM-$DEM_MAX_ZOOM" bash "$RELIEF_DIR/96-dem-terrarium.sh"
  run_step 14 "$steps" "Упаковка dem.pmtiles" \
    bash -c "source '$MAP_LIB_DIR/pmtiles.sh'; tiles_to_mbtiles dem/demtiles dem/dem.mbtiles 'Приморский край · DEM'; to_pmtiles dem/dem.mbtiles '$OUTPUT_DEM'"
  report "$OUTPUT" "$OUTPUT_CONTOURS" "$OUTPUT_DEM"
}

main "$@"
