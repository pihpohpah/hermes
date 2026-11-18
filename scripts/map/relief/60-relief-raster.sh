#!/usr/bin/env bash
# Шаг 60: рамп высот + рабочая сетка x2 (~15 м/px) + отмывка + раскраска.
# Сетка x2 нужна, чтобы отмывка и цвет были плавными на тайлах z15
# (~4.8 м/px). Контуры в растр не растеризуем: их рисует клиент.
# Вход: dem/krai_dem.tif, relief/dem_smooth.tif, zones.py -> ramp.txt.
# Выход: relief/shade.tif + relief/color.tif (читает шаг 70).
source "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"

log "[60] рамп высот (zones.py)"
run_py relief/zones.py || fail "шаг 60: zones.py"

log "[60] сетка x2, отмывка, раскраска"
gsh '
  gdalwarp -q -overwrite -r cubic -tr 0.0001388888888888889 0.0001388888888888889 \
    -co TILED=YES -co COMPRESS=DEFLATE -co BIGTIFF=YES dem_smooth.tif dem_hi.tif
  # Отмывка: z=2, масштаб градус->метры 111120 (растр в EPSG:4326!).
  # Внимание: -s 1 на географическом растре занижает склоны ~111000 раз.
  gdaldem hillshade -q -z 2 -s 111120 -az 315 -alt 45 -compute_edges \
    -co TILED=YES -co COMPRESS=DEFLATE -co BIGTIFF=YES dem_hi.tif shade.tif
  gdaldem color-relief -q dem_hi.tif ramp.txt color.tif \
    -co TILED=YES -co COMPRESS=DEFLATE -co BIGTIFF=YES
  # dem_smooth больше не нужен: контуры, отмывка и цвет построены
  rm -f dem_smooth.tif
  echo "[60] shade.tif + color.tif готовы"
' || fail "шаг 60: отмывка и цвет"
