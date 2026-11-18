#!/usr/bin/env bash
# Шаг 15: контур суши края для обрезки рельефа (boundary-land.geojson).
# Административная граница (boundary-r151225.geojson) рисует берег грубо и
# задирается на акваторию заливов; полигоны воды OSM (coastline/water_polygons,
# артефакт сборки городской карты) дают точную береговую линию с островами.
# Суша = граница минус вода. Выход: $CACHE_DIR/boundary-land.geojson.
source "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"

WATER_SHP="$CACHE_DIR/coastline/water_polygons.shp"
OUT="$CACHE_DIR/boundary-land.geojson"
[ -f "$BOUNDARY_GEOJSON" ] || fail "нет границы края: $BOUNDARY_GEOJSON"
[ -f "$WATER_SHP" ] || fail "нет полигонов воды: $WATER_SHP — соберите городскую карту (build-map.sh)"

need_docker
log "[15] суша = граница - вода -> $(basename "$OUT")"
docker run --rm -v "$CACHE_DIR:/cache" -v "$WORK_DIR:/work" "$GDAL_IMAGE" sh -c '
  set -e
  cd /tmp
  ogr2ogr -q -f GPKG land.gpkg /cache/boundary-r151225.geojson -nln boundary
  ogr2ogr -q -f GPKG -append land.gpkg /cache/coastline/water_polygons.shp -nln water
  ogr2ogr -q -f GeoJSON /work/land_raw.geojson land.gpkg -nln land \
    -dialect SQLITE \
    -sql "SELECT ST_Difference(b.geom, (SELECT ST_Union(w.geom) FROM water w)) AS geom FROM boundary b" \
    -lco RFC7946=YES
' || fail "шаг 15: разность геометрий"

# ST_Difference отдаёт GeometryCollection (полигоны + остаточные линии) —
# оставляем только MultiPolygon.
python3 - "$WORK_DIR/land_raw.geojson" "$OUT" <<'PY' || fail "шаг 15: извлечение полигонов"
import json, sys
src, dst = sys.argv[1], sys.argv[2]
geom = json.load(open(src))['features'][0]['geometry']
if geom['type'] == 'GeometryCollection':
    geom = next(g for g in geom['geometries'] if g['type'] == 'MultiPolygon')
assert geom['type'] == 'MultiPolygon', geom['type']
json.dump(
    {'type': 'FeatureCollection', 'features': [{'type': 'Feature', 'properties': {}, 'geometry': geom}]},
    open(dst, 'w'),
)
print(f'[15] полигонов суши: {len(geom["coordinates"])}')
PY
rm -f "$WORK_DIR/land_raw.geojson"
log "[15] готово: $OUT"
