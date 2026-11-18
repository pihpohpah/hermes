#!/usr/bin/env bash
# Шаг 50: слияние сглаженных контуров в один GPKG с атрибутом kind
# (minor — 20 м, major — 100 м). Потоково, без GeoJSON-промежутков:
# те весили бы десятки ГБ и не влезли бы в RAM.
# Чтение chaikin-GPKG без -nlt ломается (Z-hint в gpkg_geometry_columns)
# — -nlt LINESTRING сбрасывает Z и чинит чтение.
# Вход: contours_{minor,major}_smooth.gpkg. Выход: contours_all.gpkg.
source "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"

gsh '
  rm -f contours_all.gpkg
  echo "[50] сливаем minor..."
  ogr2ogr -q -f GPKG contours_all.gpkg contours_minor_smooth.gpkg \
    -nln contours -nlt LINESTRING \
    -sql "SELECT *, '"'"'minor'"'"' AS kind FROM contours"
  echo "[50] добавляем major..."
  ogr2ogr -q -f GPKG -append contours_all.gpkg contours_major_smooth.gpkg \
    -nln contours -nlt LINESTRING \
    -sql "SELECT *, '"'"'major'"'"' AS kind FROM contours"
  echo "contours merged: $(ogrinfo -q -sql "SELECT COUNT(*) FROM contours" contours_all.gpkg | grep -o "[0-9]\+")"
' || fail "шаг 50: слияние контуров"
