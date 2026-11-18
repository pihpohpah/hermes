#!/usr/bin/env bash
# Шаг 10: скачивание тайлов Copernicus DEM GLO-30 и сборка мозаики.
# Вход: bbox из common.sh. Выход: dem/krai_dem.tif + маркеры bbox/resolution.
# Тайлы кешируются в dem/tiles — повторный запуск докачивает недостающее.
source "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"

# DEM-тайлы 1°x1°, покрывающие bbox: от N42 до N48, от E130 до E139.
dem_tile_list() {
  local west south east north lat lon
  IFS=, read -r west south east north <<<"$BBOX"
  for ((lat = ${south%.*}; lat <= ${north%.*}; lat++)); do
    for ((lon = ${west%.*}; lon <= ${east%.*}; lon++)); do
      local latPart lonPart
      if [ "$lat" -ge 0 ]; then latPart="N$(printf '%02d' "$lat")"; else latPart="S$(printf '%02d' "${lat#-}")"; fi
      if [ "$lon" -ge 0 ]; then lonPart="E$(printf '%03d' "$lon")"; else lonPart="W$(printf '%03d' "${lon#-}")"; fi
      echo "${DEM_PREFIX}_${latPart}_00_${lonPart}_00_DEM"
    done
  done
}

mkdir -p "$WORK_DIR/dem/tiles"
log "Скачиваем тайлы DEM GLO-30 по bbox $BBOX (кеш: dem/tiles, 6 потоков)"

mapfile -t tiles < <(dem_tile_list)
log "всего тайлов: ${#tiles[@]}"
for tile in "${tiles[@]}"; do
  printf '%s/%s/%s.tif %s\n' "$DEM_BUCKET" "$tile" "$tile" "$WORK_DIR/dem/tiles/$tile.tif"
done | xargs -P 6 -n 2 bash "$MAP_LIB_DIR/dem-download.sh" \
  || fail "не удалось скачать тайлы DEM"
log "тайлы на диске: $(find "$WORK_DIR/dem/tiles" -name '*.tif' | wc -l) (остальное — море, пропуск)"

west=${BBOX%,*}; west=${west%,*}; west=${west%,*}
rest=${BBOX#*,}; south=${rest%,*}; rest=${rest#*,}
east=${rest%,*}; north=${rest#*,}

log "Собираем мозаику gdalbuildvrt -> krai_dem.tif ($DEM_TR градусов/px)"
need_docker
docker run --rm -v "$WORK_DIR:/work" "$GDAL_IMAGE" sh -c "
  set -e
  cd /work
  gdalbuildvrt -q -te $west $south $east $north -tr $DEM_TR $DEM_TR \
    dem/mosaic.vrt dem/tiles/*.tif
  gdal_translate -q --config CPL_VSIL_CURL_CACHE_SIZE 67108864 \
    dem/mosaic.vrt dem/krai_dem.tif
  python3 - <<'PY'
from osgeo import gdal
gdal.UseExceptions()
ds = gdal.Open('/work/dem/krai_dem.tif')
band = ds.GetRasterBand(1)
minimum, maximum = band.ComputeStatistics(False)[:2]
print(f'mosaic stats: {minimum:.0f}..{maximum:.0f} m', flush=True)
if maximum < 1000:
    raise SystemExit(f'подозрительный DEM: max {maximum:.0f} m')
PY
" || fail "не удалось собрать мозаику DEM"

[ -f "$WORK_DIR/dem/krai_dem.tif" ] || fail "DEM не создан"
printf '%s' "$BBOX" >"$WORK_DIR/dem/bbox.txt"
printf '%s' "$DEM_TR" >"$WORK_DIR/dem/resolution.txt"
log "мозаика готова: dem/krai_dem.tif"
