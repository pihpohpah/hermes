#!/usr/bin/env bash
# Шаг 10: мировая вода (океаны и крупные озёра) -> векторные тайлы z0-10.
# Natural Earth 50m: скачать в $CACHE_DIR/water (ne_50m_ocean/lakes.zip),
# распаковать и запустить этот шаг. Обрезаем по bbox с запасом вокруг края —
# заливы Приморья, Японское море, Ханка, Амурский лиман.
# Выход: $OUTPUT_WATER (primorsky-water.pmtiles).
source "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"
source "$MAP_LIB_DIR/pmtiles.sh"

WATER_DIR="$CACHE_DIR/water"
OUTPUT_WATER="${WATER_OUTPUT:-$ROOT_DIR/web/static/pmtiles/primorsky-water.pmtiles}"
case "$OUTPUT_WATER" in /*) ;; *) OUTPUT_WATER="$ROOT_DIR/$OUTPUT_WATER" ;; esac

# bbox с запасом: вся акватория вокруг края. -clipsrc (не -spat!): океан NE —
# единый полигон на весь мир, отбор по экстенту пропустил бы его целиком.
export BBOX="125,38,145,52"
WATER_BBOX="-clipsrc 125 38 145 52"

[ -f "$WATER_DIR/ne_50m_ocean.shp" ] || fail "нет $WATER_DIR/ne_50m_ocean.shp — скачайте ne_50m_ocean.zip с naturalearthdata.com"
[ -f "$WATER_DIR/ne_50m_lakes.shp" ] || fail "нет $WATER_DIR/ne_50m_lakes.shp — скачайте ne_50m_lakes.zip"

# заплатка восточнее края: всё море, иначе из «дырки» океана проступает
# Хоккайдо. Восточнее 139.31E в пределах клипа океан + Хоккайдо, суши края
# там нет (bounds края 130.39-139.31). GeoJSON кладём в work, docker appends.
cat > "$WORK_DIR/relief/east_patch.geojson" <<'GEOJSON'
{"type":"FeatureCollection","features":[{"type":"Feature","properties":{},"geometry":{"type":"Polygon","coordinates":[[[139.31,38],[145,38],[145,52],[139.31,52],[139.31,38]]]}}]}
GEOJSON

need_docker
docker run --rm -v "$WORK_DIR:/work" -v "$CACHE_DIR:/cache:ro" "$GDAL_IMAGE" \
  sh -c "set -e; cd /work/relief
  rm -rf water_mvt water.gpkg
  echo '[10] ocean -> gpkg'
  ogr2ogr -q -f GPKG water.gpkg /cache/water/ne_50m_ocean.shp \
    -nln water -nlt PROMOTE_TO_MULTI $WATER_BBOX
  echo '[10] lakes -> gpkg'
  ogr2ogr -q -f GPKG -append water.gpkg /cache/water/ne_50m_lakes.shp \
    -nln water -nlt PROMOTE_TO_MULTI $WATER_BBOX
  echo '[10] заплатка восточнее края (Хоккайдо -> море)'
  ogr2ogr -q -f GPKG -append water.gpkg east_patch.geojson \
    -nln water -nlt PROMOTE_TO_MULTI $WATER_BBOX
  echo '[10] MVT z0-10 (без упрощения: острова и берега на z8-10)'
  ogr2ogr -q -f MVT water_mvt water.gpkg \
    -nln water -dsco MINZOOM=0 -dsco MAXZOOM=10 -dsco COMPRESS=NO -dsco SIMPLIFICATION=0
  echo \"[10] тайлов: \$(find water_mvt -name '*.pbf' | wc -l)\"
" || fail "шаг 10: вода -> MVT"

tiles_to_mbtiles relief/water_mvt relief/water.mbtiles "Приморский край · вода" pbf \
  '{"vector_layers":[{"id":"water","fields":{}}]}'

to_pmtiles relief/water.mbtiles "$OUTPUT_WATER"
report "$OUTPUT_WATER"
