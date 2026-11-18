#!/usr/bin/env bash
# Шаг 90: нарезка растра рельефа в PNG-тайлы z$MIN_ZOOM-$MAX_ZOOM.
# Растр перепроецируется в Web Mercator и режется gdal2tiles (понимает
# диапазон зумов напрямую). Ресемплинг cubic: исходник 30 м/px апсемплится
# в тайлы z14-15, nearest давал бы «пиксельную» карту — cubic сохраняет
# гладкость (см. MAP.md, не включать nearest обратно).
# Вход: relief/shaded_rgb.tif. Выход: relief/tiles/{z}/{x}/{y}.png.
source "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"

gsh '
  rm -f color_merc.tif
  gdalwarp -q -overwrite -t_srs EPSG:3857 -r cubic \
    -co TILED=YES -co COMPRESS=DEFLATE -co BIGTIFF=YES \
    shaded_rgb.tif color_merc.tif
  rm -rf tiles
  gdal2tiles.py -q --xyz -z '"$MIN_ZOOM-$MAX_ZOOM"' -r cubic --processes=2 --no-kml \
    color_merc.tif tiles
  rm -f color_merc.tif
  # сетка x2: слои по гигабайту каждый — до упаковки не нужны
  rm -f dem_hi.tif shade.tif color.tif
  echo "tiles produced: $(find tiles -name "*.png" | wc -l)"
' || fail "шаг 90: нарезка тайлов"
