#!/usr/bin/env bash
# Общее для шагов пайплайна карты: пути, логирование, docker-хелперы.
# Каждый шаг делает source этого файла — шаг можно запускать отдельно.

MAP_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MAP_DIR="$(cd "$MAP_LIB_DIR/.." && pwd)"          # scripts/map (монтируется в docker как /scripts)
ROOT_DIR="$(cd "$MAP_DIR/../.." && pwd)"

CACHE_DIR="${MAP_CACHE_DIR:-$ROOT_DIR/.cache/map}"
WORK_DIR="$CACHE_DIR/relief"                       # все промежуточные файлы
GDAL_IMAGE="${GDAL_IMAGE:-ghcr.io/osgeo/gdal:alpine-normal-latest}"
PMTILES_IMAGE="${PMTILES_IMAGE:-ghcr.io/protomaps/go-pmtiles:latest}"

BBOX="${RELIEF_BBOX:-130.394,42.123,139.309,48.465}"
MIN_ZOOM="${RELIEF_MINZOOM:-3}"
MAX_ZOOM="${RELIEF_MAXZOOM:-15}"
DEM_MAX_ZOOM="${DEM_MAX_ZOOM:-12}"

# Горизонтали: z5-11 только major (100 м), с z12 — major+minor (20 м).
CONTOURS_MIN_ZOOM=5
CONTOURS_MAJOR_MAX_ZOOM=11
CONTOURS_MINOR_MIN_ZOOM=12
CONTOURS_MAX_ZOOM=14

OUTPUT="${RELIEF_OUTPUT:-$ROOT_DIR/web/static/pmtiles/primorsky-relief.pmtiles}"
OUTPUT_CONTOURS="${RELIEF_CONTOURS_OUTPUT:-$ROOT_DIR/web/static/pmtiles/primorsky-contours.pmtiles}"
OUTPUT_DEM="${DEM_OUTPUT:-$ROOT_DIR/web/static/pmtiles/primorsky-dem.pmtiles}"
for var in OUTPUT OUTPUT_DEM OUTPUT_CONTOURS; do
  eval "case \"\$$var\" in /*) ;; *) $var=\"$ROOT_DIR/\$$var\" ;; esac"
done

# Copernicus DEM GLO-30: тайлы 1°x1° (N45_00_E135_00), 30 м/px.
DEM_BUCKET="${DEM_BUCKET:-https://copernicus-dem-30m.s3.amazonaws.com}"
DEM_PREFIX="Copernicus_DSM_COG_10"
DEM_TR="${DEM_TR:-0.0002777777777777778}"

# Граница края для обрезки (-cutline): артефакт build-map.sh.
BOUNDARY_GEOJSON="${RELIEF_BOUNDARY:-$CACHE_DIR/boundary-r151225.geojson}"

# Лог с меткой времени: по логу видно, на каком месте зависла сборка.
log() { printf '\033[1;34m==>\033[0m [%s] %s\n' "$(date +%H:%M:%S)" "$*"; }
fail() { printf '\033[1;31mОШИБКА:\033[0m [%s] %s\n' "$(date +%H:%M:%S)" "$*" >&2; exit 1; }

need_docker() {
  command -v docker >/dev/null 2>&1 || fail "нужен docker"
  docker info >/dev/null 2>&1 || fail "docker не запущен (open -a Docker)"
}

# Python-шаги исполняются в контейнере GDAL: /work — кеш пайплайна,
# /scripts — шаги (read-only).
run_py() { # run_py <путь-в-scripts/map> [аргументы...]
  need_docker
  docker run --rm -v "$WORK_DIR:/work" -v "$MAP_DIR:/scripts:ro" "$GDAL_IMAGE" \
    python3 "/scripts/$@"
}

# Произвольная команда в контейнере GDAL с рабочим каталогом /work/relief.
gsh() { # gsh '<shell-команды>'
  need_docker
  docker run --rm -v "$WORK_DIR:/work" -v "$MAP_DIR:/scripts:ro" "$GDAL_IMAGE" \
    sh -c "set -e; cd /work/relief; $1"
}
