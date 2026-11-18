#!/usr/bin/env bash
# Шаг 95: контуры -> векторные тайлы (MVT) z5-14.
# z12-14 (все линии) — ОТДЕЛЬНЫМ проходом на каждый зум: за один прогон
# ogr2ogr не тянет пирамиду в RAM VM. z5-11 — только major (100 м):
# мелкие кольца на обзорных зумах неразличимы и раздувают тайлы.
# GDAL MVT directory пишет некорректный metadata.json (без "tiles") —
# tilejson для клиента добавляем при упаковке (vector_layers обязателен).
# Вход: relief/contours_clip.gpkg (обрезка шага 52).
# Выход: relief/contours_mvt/{z}/{x}/{y}.pbf.
source "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"

gsh '
  rm -rf contours_mvt mvt_z*
  for z in 12 13 14; do
    echo "[95] MVT z$z (все линии)..."
    ogr2ogr -q -f MVT mvt_z$z contours_clip.gpkg \
      -nln contours -nlt LINESTRING \
      -dsco MINZOOM=$z -dsco MAXZOOM=$z -dsco COMPRESS=NO
    echo "[95] z$z готово: $(find mvt_z$z -name "*.pbf" | wc -l) тайлов"
  done
  echo "[95] MVT z5-11 (только major)..."
  ogr2ogr -q -f MVT mvt_z511 contours_clip.gpkg \
    -nln contours -nlt LINESTRING \
    -sql "SELECT *, '"'"'major'"'"' AS kind FROM contours" \
    -dsco MINZOOM=5 -dsco MAXZOOM=11 -dsco COMPRESS=NO
  echo "[95] z5-11 готово: $(find mvt_z511 -name "*.pbf" | wc -l) тайлов"
  # Каталоги сливаем: зумы не пересекаются, наложений нет.
  mkdir -p contours_mvt
  for d in mvt_z511 mvt_z12 mvt_z13 mvt_z14; do cp -R $d/. contours_mvt/; done
  rm -rf mvt_z511 mvt_z12 mvt_z13 mvt_z14
  echo "mvt tiles: $(find contours_mvt -name "*.pbf" | wc -l)"
' || fail "шаг 95: нарезка MVT"
