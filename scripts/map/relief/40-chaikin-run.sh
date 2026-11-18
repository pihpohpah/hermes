#!/usr/bin/env bash
# Шаг 40: Чайкин для обоих наборов контуров (мелкие 20 м + крупные 100 м).
# Вход: contours_{minor,major}.gpkg. Выход: contours_*_smooth.gpkg.
source "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"

log "[40] Чайкин: minor (20 м)"
run_py relief/40-chaikin.py /work/relief/contours_minor.gpkg /work/relief/contours_minor_smooth.gpkg \
  || fail "шаг 40: chaikin minor"
# major идут на обзорные зумы z5-11 и часто смотрятся overscale (z11.9) —
# «лесенка» пиксельной сетки DEM (30 м) видна сильнее, берём 4 итерации.
log "[40] Чайкин: major (100 м), 4 итерации"
run_py relief/40-chaikin.py /work/relief/contours_major.gpkg /work/relief/contours_major_smooth.gpkg 4 \
  || fail "шаг 40: chaikin major"
