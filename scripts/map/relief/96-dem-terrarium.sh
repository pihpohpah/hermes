#!/usr/bin/env bash
# Шаг 96: DEM -> terrarium-RGB -> тайлы z$MIN_ZOOM-$DEM_MAX_ZOOM.
# Terrarium-кодировка для MapLibre terrain (raster-dem): h = R*256+G+B/256-32768.
# Вход: dem/krai_dem.tif. Выход: dem/demtiles/{z}/{x}/{y}.png.
source "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"

docker run --rm -v "$WORK_DIR:/work" -v "$MAP_DIR:/scripts:ro" "$GDAL_IMAGE" sh -c "
  set -e
  cd /work/dem
  echo '[96] перепроекция в Web Mercator (bilinear по float-высотам)...'
  gdalwarp -q -overwrite -t_srs EPSG:3857 -r bilinear -ot Float32 \
    -co TILED=YES -co COMPRESS=DEFLATE krai_dem.tif dem_merc.tif
  echo '[96] кодирование terrarium-RGB...'
  python3 /scripts/relief/terrarium.py
  rm -rf demtiles
  gdal2tiles.py -q --xyz -z '$MIN_ZOOM-$DEM_MAX_ZOOM' -r near --processes=2 --no-kml \
    dem_rgb.tif demtiles
  echo 'dem tiles produced: '\$(find demtiles -name '*.png' | wc -l)
" || fail "шаг 96: terrarium DEM"
