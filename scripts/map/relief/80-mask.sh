#!/usr/bin/env bash
# Шаг 80: обрезка рельефа по полигону суши края.
# Контур суши (boundary-land.geojson) = административная граница минус
# полигоны воды OSM: береговая линия точная, включая острова. Без него —
# по самой границе (грубый берег) или без обрезки (map-relief-local).
# Вход: relief/shaded_rgb.tif + контур из $CACHE_DIR. Выход: тот же файл.
source "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"

CUTLINE="$BOUNDARY_GEOJSON"
if [ -f "$CACHE_DIR/boundary-land.geojson" ]; then
  CUTLINE="$CACHE_DIR/boundary-land.geojson"
elif [ ! -f "$BOUNDARY_GEOJSON" ]; then
  log "[80] контуров нет ($BOUNDARY_GEOJSON) — пропускаем, рельеф останется прямоугольным"
  exit 0
fi

need_docker
log "[80] обрезка по $(basename "$CUTLINE") (-cutline -dstalpha)"
docker run --rm -v "$WORK_DIR:/work" -v "$CACHE_DIR:/cache:ro" "$GDAL_IMAGE" \
  gdalwarp -q -overwrite \
  -co BIGTIFF=YES \
  -cutline "/cache/$(basename "$CUTLINE")" -cblend 2 -dstalpha \
  -co TILED=YES -co COMPRESS=DEFLATE \
  /work/relief/shaded_rgb.tif /work/relief/shaded_cut.tif \
  || fail "шаг 80: обрезка по контуру суши"
mv "$WORK_DIR/relief/shaded_cut.tif" "$WORK_DIR/relief/shaded_rgb.tif"
log "[80] обрезан: relief/shaded_rgb.tif"
