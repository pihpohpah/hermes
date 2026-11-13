#!/usr/bin/env bash
# Собирает офлайн-рельеф Приморского края: PNG-тайлы отмывки (hillshade) в PMTiles.
#
# Зачем отдельный архив: векторные тайлы OSM рельефа не содержат. Отмывка
# приходит из Copernicus DEM GLO-90 (90 м), читается по HTTP-range прямо из
# COG-файла на S3 — качать гигабайты не нужно.
#
# Пайплайн:
#   1. вырезать окно DEM из удалённого COG по bbox края (gdal_translate + /vsicurl)
#   2. посчитать диапазон высот и выбрать 8–20 равных зон («ступеней»)
#   3. посчитать отмывку (gdaldem hillshade) и дискретную раскраску зон
#   4. смешать их в один RGBA-растр, чтобы природа читалась по цвету
#   5. нарезать в MBTiles и сконвертировать в PMTiles
#
# Высотные зоны зависят от покрытия: для агломерации Владивостока (~650 м
# перепад) выйдет ~13 зон по 50 м, для всего края (~1900 м) — ~19 по 100 м.
# Рампа не зашита: подбирается шаг из «красивого» ряда так, чтобы зон было
# от 8 до 20, цвета — дискретная гипсометрическая палитра.
#
# Внешние зависимости: docker (образ GDAL тянется автоматически).
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CACHE_DIR="${MAP_CACHE_DIR:-$ROOT_DIR/.cache/map}"
WORK_DIR="$CACHE_DIR/relief"
GDAL_IMAGE="${GDAL_IMAGE:-ghcr.io/osgeo/gdal:alpine-normal-latest}"
PMTILES_IMAGE="${PMTILES_IMAGE:-ghcr.io/protomaps/go-pmtiles:latest}"

MIN_ZOOM="${RELIEF_MINZOOM:-8}"
MAX_ZOOM="${RELIEF_MAXZOOM:-13}"
OUTPUT="${RELIEF_OUTPUT:-$ROOT_DIR/web/static/pmtiles/primorsky-relief.pmtiles}"
# docker run -v требует абсолютный путь; относительный путь он принимает за имя тома.
case "$OUTPUT" in
  /*) ;;
  *) OUTPUT="$ROOT_DIR/$OUTPUT" ;;
esac

# Bbox края совпадает с bounding_box в scripts/map/config.json.
BBOX="${RELIEF_BBOX:-130.394,42.123,139.309,48.465}"

# Copernicus DEM GLO-90: тайлы 1°x1°, ключ N43_00_E131_00. Читаем только нужные.
DEM_BUCKET="${DEM_BUCKET:-https://copernicus-dem-90m.s3.amazonaws.com}"
DEM_PREFIX="Copernicus_DSM_COG_30"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31mОШИБКА:\033[0m %s\n' "$*" >&2; exit 1; }

command -v docker >/dev/null 2>&1 || fail "нужен docker: без него не собрать тайлы"

# DEM-тайлы, покрывающие bbox: от N42 до N48, от E130 до E139.
dem_tile_list() {
  local west south east north
  IFS=, read -r west south east north <<<"$BBOX"
  local lat lon
  for ((lat = ${south%.*}; lat <= ${north%.*}; lat++)); do
    for ((lon = ${west%.*}; lon <= ${east%.*}; lon++)); do
      local latPart lonPart
      if [ "$lat" -ge 0 ]; then latPart="N$(printf '%02d' "$lat")"; else latPart="S$(printf '%02d' "${lat#-}")"; fi
      if [ "$lon" -ge 0 ]; then lonPart="E$(printf '%03d' "$lon")"; else lonPart="W$(printf '%03d' "${lon#-}")"; fi
      echo "${DEM_PREFIX}_${latPart}_00_${lonPart}_00_DEM"
    done
  done
}

# Не хватает ли DEM-тайла для точки — учитывается при сборке мозаики.
fetch_dem() {
  mkdir -p "$WORK_DIR/dem"
  log "Собираем мозаику DEM GLO-90 по bbox $BBOX"

  local -a tiles=()
  while IFS= read -r line; do tiles+=("$line"); done < <(dem_tile_list)

  # /vsicurl + gdalbuildvrt: GDAL читает только нужные блоки COG по range.
  local vrtList="$WORK_DIR/dem/tiles.txt"
  : >"$vrtList"
  local tile
  for tile in "${tiles[@]}"; do
    echo "/vsicurl/${DEM_BUCKET}/${tile}/${tile}.tif" >>"$vrtList"
  done

  local west south east north
  IFS=, read -r west south east north <<<"$BBOX"

  cat >"$WORK_DIR/dem/fetch.sh" <<FETCH
set -e
cd /work
# /vsicurl с повторами: удалённый COG иногда отдаёт обрезанный блок.
gdalbuildvrt -q -te $west $south $east $north -tr 0.00083333333 0.00083333333 \
  dem/mosaic.vrt \$(tr '\\n' ' ' < /work/dem/tiles.txt)
gdal_translate -q --config GDAL_HTTP_MAX_RETRY 5 \
  --config GDAL_HTTP_RETRY_DELAY 2 \
  --config CPL_VSIL_CURL_CACHE_SIZE 67108864 \
  dem/mosaic.vrt dem/krai_dem.tif
FETCH

  local attempt
  for attempt in 1 2 3; do
    if docker run --rm -v "$WORK_DIR:/work" "$GDAL_IMAGE" sh /work/dem/fetch.sh; then
      break
    fi
    [ "$attempt" -eq 3 ] && fail "не удалось собрать мозаику DEM после 3 попыток"
    log "DEM-мозаика не собралась (попытка $attempt), повторяем"
    rm -f "$WORK_DIR/dem/krai_dem.tif" "$WORK_DIR/dem/mosaic.vrt"
    sleep 3
  done

  [ -f "$WORK_DIR/dem/krai_dem.tif" ] || fail "DEM не создан"
}

render_relief() {
  log "Считаем диапазон высот, отмывку и цветовые зоны"
  cat >"$WORK_DIR/relief/zones.py" <<'ZONES'
"""Выбирает 8-20 высотных зон по min/max DEM и пишет дискретный рамп.

Шаг подбирается из «красивого» ряда (10/25/50/100/150/250/500 м) так, чтобы
зон получилось 8-20. Цвет каждой зоны — палитра, интерполированная по её
центру: зелень низин -> песок -> коричневый -> серый -> снег. Рамп пишется
для gdaldem color-relief -nearest_color_entry: входы по центрам зон, значение
округляется до ближайшего центра, поэтому на тайлах читаются именно ступени.
"""
import sys

from osgeo import gdal

gdal.UseExceptions()

BAND_MIN, BAND_MAX = 8, 20
NICE_STEPS = (10, 25, 50, 100, 150, 250, 500)

# Позиция в нормированном диапазоне -> (r, g, b). Ниже нуля — глубокая вода.
PALETTE = [
    (0.00, (52, 96, 110)),
    (0.04, (70, 122, 88)),
    (0.18, (126, 158, 96)),
    (0.36, (188, 178, 124)),
    (0.54, (184, 150, 110)),
    (0.72, (158, 146, 134)),
    (0.86, (172, 172, 168)),
    (1.00, (244, 244, 240)),
]


def pick_color(t):
    """Линейная интерполяция палитры в точке t (0..1)."""
    for (t0, c0), (t1, c1) in zip(PALETTE, PALETTE[1:]):
        if t <= t1:
            k = 0.0 if t1 == t0 else (t - t0) / (t1 - t0)
            return tuple(round(a + (b - a) * k) for a, b in zip(c0, c1))
    return PALETTE[-1][1]


dem = gdal.Open('/work/dem/krai_dem.tif')
band = dem.GetRasterBand(1)
minimum, maximum = band.ComputeStatistics(False)[:2]

span = max(maximum - minimum, 1.0)
step = next((s for s in NICE_STEPS if BAND_MIN <= -(-span // s) <= BAND_MAX), None)
if step is None:
    # Перепад экстремальный: берём средний шаг и давим число зон в диапазон.
    step = max(round(span / ((BAND_MIN + BAND_MAX) / 2)), 1)

lo = int(minimum // step) * step
hi = int(-(-maximum // step)) * step  # ceil для отрицательных тоже
count = max(BAND_MIN, min(BAND_MAX, round((hi - lo) / step)))

with open('/work/relief/ramp.txt', 'w') as out:
    for index in range(count):
        center = lo + (index + 0.5) * (hi - lo) / count
        r, g, b = pick_color((center - minimum) / span)
        out.write(f'{center:.2f} {r} {g} {b}\n')

print(f'elevation range: {minimum:.0f}..{maximum:.0f} m, '
      f'{count} zones by {step} m ({lo}..{hi})', flush=True)
ZONES

  cat >"$WORK_DIR/relief/render.sh" <<'RENDER'
set -e
cd /work/relief
python3 zones.py
gdaldem hillshade -q -z 2 -s 1 -az 315 -alt 45 -compute_edges /work/dem/krai_dem.tif shade.tif
# -nearest_color_entry: пиксель получает цвет ближайшей точки рампа,
# поэтому вместо градиента на тайлах читаются дискретные высотные зоны.
gdaldem color-relief -q -nearest_color_entry /work/dem/krai_dem.tif ramp.txt color.tif
RENDER

  cat >"$WORK_DIR/relief/blend.py" <<'BLEND'
"""Смешивает цвет высот с отмывкой: color * (0.45 + 0.55 * shade/255)."""
import numpy as np
from osgeo import gdal

gdal.UseExceptions()

color = gdal.Open('color.tif')
shade = gdal.Open('shade.tif')
width, height = color.RasterXSize, color.RasterYSize

rgb = np.empty((3, height, width), dtype=np.float32)
for index in range(3):
    rgb[index] = color.GetRasterBand(index + 1).ReadAsArray().astype(np.float32)

shade_arr = shade.GetRasterBand(1).ReadAsArray().astype(np.float32)
factor = 0.45 + 0.55 * (shade_arr / 255.0)
mixed = np.clip(rgb * factor, 0, 255).astype(np.uint8)

driver = gdal.GetDriverByName('GTiff')
out = driver.Create(
    'shaded_rgb.tif', width, height, 3, gdal.GDT_Byte,
    options=['TILED=YES', 'COMPRESS=DEFLATE', 'PHOTOMETRIC=RGB'],
)
out.SetGeoTransform(color.GetGeoTransform())
out.SetProjection(color.GetProjection())
for index in range(3):
    out.GetRasterBand(index + 1).WriteArray(mixed[index])
out.FlushCache()
out = None

print('relief blended:', width, 'x', height)
BLEND

  docker run --rm -v "$WORK_DIR:/work" "$GDAL_IMAGE" sh -c "
    set -e
    sh /work/relief/render.sh
    cd /work/relief && python3 blend.py
  " || fail "не удалось посчитать отмывку"

  [ -f "$WORK_DIR/relief/shaded_rgb.tif" ] || fail "растр рельефа не создан"
}

to_tiles() {
  log "Нарезаем тайлы z$MIN_ZOOM-$MAX_ZOOM"
  cat >"$WORK_DIR/relief/tile.sh" <<'TILE'
set -e
cd /work/relief
# Растр отмывки строим в Web Mercator и нарезаем gdal2tiles: он понимает
# диапазон зумов напрямую, в отличие от gdal_translate+MBTiles, который сам
# выбирает зум по разрешению исходника.
#
# Ресемплинг только nearest: bilinear размывает границы дискретных высотных
# зон в плавный градиент, а нужна читаемая «лесенка».
rm -f shaded_merc.tif
gdalwarp -q -overwrite -t_srs EPSG:3857 -r near -co TILED=YES -co COMPRESS=DEFLATE \
  shaded_rgb.tif shaded_merc.tif

rm -rf tiles
gdal2tiles.py -q --xyz -z "$1-$2" -r near --processes=2 --no-kml shaded_merc.tif tiles
echo "tiles produced: $(find tiles -name '*.png' | wc -l)"
TILE

  docker run --rm -v "$WORK_DIR:/work" -e MIN_ZOOM="$MIN_ZOOM" -e MAX_ZOOM="$MAX_ZOOM"     "$GDAL_IMAGE" sh /work/relief/tile.sh "$MIN_ZOOM" "$MAX_ZOOM" \
    || fail "не удалось нарезать тайлы"
}

# PNG-тайлы ссылками собираем в MBTiles, затем в PMTiles.
tiles_to_mbtiles() {
  # PNG-тайлы собираем в MBTiles вручную: gdal2tiles пишет XYZ, а MBTiles
  # хранит TMS, поэтому Y переворачивается.
  cat >"$WORK_DIR/relief/mbtiles.py" <<'PYTILES'
import os
import sqlite3

tiles_root = '/work/relief/tiles'
out = '/work/relief/relief.mbtiles'
if os.path.exists(out):
    os.remove(out)

db = sqlite3.connect(out)
db.execute('CREATE TABLE metadata (name text, value text)')
db.execute('CREATE TABLE tiles (zoom_level integer, tile_column integer, tile_row integer, tile_data blob)')

min_zoom = max_zoom = None
count = 0
for zoom in sorted(os.listdir(tiles_root)):
    if not zoom.isdigit():
        continue
    z = int(zoom)
    for x in os.listdir(os.path.join(tiles_root, zoom)):
        xdir = os.path.join(tiles_root, zoom, x)
        if not os.path.isdir(xdir):
            continue
        for name in os.listdir(xdir):
            if not name.endswith('.png'):
                continue
            y = int(name[:-4])
            with open(os.path.join(xdir, name), 'rb') as handle:
                data = handle.read()
            db.execute(
                'INSERT INTO tiles VALUES (?, ?, ?, ?)',
                (z, int(x), (2 ** z - 1) - y, data),
            )
            count += 1
            min_zoom = z if min_zoom is None else min(min_zoom, z)
            max_zoom = z if max_zoom is None else max(max_zoom, z)

meta = {
    'name': 'Приморский край · рельеф',
    'format': 'png',
    'minzoom': str(min_zoom),
    'maxzoom': str(max_zoom),
    'type': 'baselayer',
    # Реальный bbox покрытия: у локальной сборки он меньше края, и честные
    # границы спасают от ложного «рельеф должен быть» за пределами области.
    'bounds': os.environ.get('BBOX', '130.394,42.123,139.309,48.465'),
}
for key, value in meta.items():
    db.execute('INSERT INTO metadata VALUES (?, ?)', (key, value))
db.commit()
db.close()
print(f'mbtiles: {count} tiles, z{min_zoom}-{max_zoom}')
PYTILES

  docker run --rm -v "$WORK_DIR:/work" -e BBOX="$BBOX" "$GDAL_IMAGE" python3 /work/relief/mbtiles.py \
    || fail "не удалось собрать MBTiles из тайлов"
}

to_pmtiles() {
  log "Конвертируем MBTiles -> PMTiles"
  mkdir -p "$(dirname "$OUTPUT")"
  docker run --rm -v "$WORK_DIR:/data" -v "$(dirname "$OUTPUT"):/out" "$PMTILES_IMAGE" \
    convert --force /data/relief/relief.mbtiles "/out/$(basename "$OUTPUT")" >/dev/null

  [ -f "$OUTPUT" ] || fail "PMTiles не создан: $OUTPUT"

  docker run --rm -v "$(dirname "$OUTPUT"):/out" "$PMTILES_IMAGE" verify "/out/$(basename "$OUTPUT")" >/dev/null \
    || fail "архив рельефа не прошёл проверку"
}

report() {
  local size
  size=$(du -h "$OUTPUT" | cut -f1)
  log "Готово: $OUTPUT ($size)"
  docker run --rm -v "$(dirname "$OUTPUT"):/out" "$PMTILES_IMAGE" show "/out/$(basename "$OUTPUT")" \
    | grep -E "bounds|min zoom|max zoom|addressed tiles|format"
}

main() {
  mkdir -p "$WORK_DIR/relief"
  fetch_dem
  render_relief
  to_tiles
  tiles_to_mbtiles
  to_pmtiles
  report
}

main "$@"
