#!/usr/bin/env bash
# Шаг 30: горизонтали из сглаженного DEM — мелкие каждые 20 м,
# утолщённые каждые 100 м. Плотность «колец» — как у mapy.com/OpenTopoMap.
# -a ELEV сохраняет высоту в атрибуте (уедет в векторные тайлы).
# gdal_contour к существующему GPKG добавляет слой — удаляем старые файлы.
# Вход: dem/dem_smooth.tif. Выход: relief/contours_{minor,major}.gpkg.
source "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"

gsh '
  rm -f contours_minor.gpkg contours_major.gpkg
  echo "[30] gdal_contour 20 м (мелкие)..."
  gdal_contour -q -i 20 -nln contours -a ELEV dem_smooth.tif contours_minor.gpkg
  echo "[30] gdal_contour 100 м (крупные)..."
  gdal_contour -q -i 100 -nln contours -a ELEV dem_smooth.tif contours_major.gpkg
  echo "[30] контуров: minor $(ogrinfo -q -sql "SELECT COUNT(*) FROM contours" contours_minor.gpkg | grep -o "[0-9]\+" | tail -1), major $(ogrinfo -q -sql "SELECT COUNT(*) FROM contours" contours_major.gpkg | grep -o "[0-9]\+" | tail -1)"
' || fail "шаг 30: gdal_contour"
