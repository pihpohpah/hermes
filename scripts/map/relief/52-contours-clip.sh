#!/usr/bin/env bash
# Шаг 52: обрезка контуров по полигону границы края.
# DEM-мозаика и контуры строятся прямоугольником по bbox: без обрезки кольца
# уходят на соседние регионы (Китай, Хабаровский край). Растровый рельеф
# режется шагом 80, здесь — то же для векторных горизонталей.
# Вход: relief/contours_all.gpkg + граница из build-map.sh.
# Выход: relief/contours_clip.gpkg (читает шаг 95).
source "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"

if [ ! -f "$BOUNDARY_GEOJSON" ]; then
  log "[52] граница не найдена ($BOUNDARY_GEOJSON) — пропускаем, контуры останутся прямоугольными"
  exit 0
fi

need_docker
log "[52] обрезка контуров по $(basename "$BOUNDARY_GEOJSON")"
docker run --rm -v "$WORK_DIR:/work" -v "$CACHE_DIR:/cache:ro" "$GDAL_IMAGE" \
  sh -c "set -e; cd /work/relief
  before=\$(ogrinfo -q -sql \"SELECT COUNT(*) FROM contours\" contours_all.gpkg | grep -o '[0-9]\+' | head -1)
  rm -f contours_clip.gpkg
  ogr2ogr -q -f GPKG contours_clip.gpkg contours_all.gpkg \
    -nln contours -nlt LINESTRING \
    -clipsrc /cache/$(basename "$BOUNDARY_GEOJSON")
  after=\$(ogrinfo -q -sql \"SELECT COUNT(*) FROM contours\" contours_clip.gpkg | grep -o '[0-9]\+' | head -1)
  echo \"[52] линий: \$before -> \$after\"
" || fail "шаг 52: обрезка контуров"
