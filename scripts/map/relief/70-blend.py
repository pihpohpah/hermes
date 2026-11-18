#!/usr/bin/env python3
"""Шаг 70: смешивание высотного градиента с отмывкой.

Вход:  color.tif + shade.tif (шаг 60, сетка x2 ~15 м/px).
Выход: shaded_rgb.tif — итоговый растр рельефа (без контуров).

Отмывка: color * (0.68 + 0.32 * shade/255) — заметная, но не кричащая.
Полосы по BLOCK_ROWS строк: полный массив x2 в float32 не влез бы в RAM.
"""
import numpy as np
from osgeo import gdal

gdal.UseExceptions()

BLOCK_ROWS = 2048  # высота полосы; пик ~8 ГБ на сетке x2

color = gdal.Open('color.tif')
shade = gdal.Open('shade.tif')
width, height = color.RasterXSize, color.RasterYSize

driver = gdal.GetDriverByName('GTiff')
out = driver.Create(
    'shaded_rgb.tif', width, height, 3, gdal.GDT_Byte,
    options=['TILED=YES', 'COMPRESS=DEFLATE', 'BIGTIFF=YES', 'PHOTOMETRIC=RGB'],
)
out.SetGeoTransform(color.GetGeoTransform())
out.SetProjection(color.GetProjection())

for y0 in range(0, height, BLOCK_ROWS):
    rows = min(BLOCK_ROWS, height - y0)
    rgb = np.empty((3, rows, width), dtype=np.float32)
    for index in range(3):
        rgb[index] = color.GetRasterBand(index + 1).ReadAsArray(0, y0, width, rows) \
            .astype(np.float32)
    shade_arr = shade.GetRasterBand(1).ReadAsArray(0, y0, width, rows) \
        .astype(np.float32)
    rgb *= 0.68 + 0.32 * (shade_arr / 255.0)
    del shade_arr
    mixed = np.clip(rgb, 0, 255).astype(np.uint8)
    del rgb
    for index in range(3):
        out.GetRasterBand(index + 1).WriteArray(mixed[index], 0, y0)
    del mixed
    print(f'blended rows {y0}-{y0 + rows}', flush=True)

out.FlushCache()
out = None
print('relief blended:', width, 'x', height, flush=True)
