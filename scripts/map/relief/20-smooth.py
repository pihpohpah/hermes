#!/usr/bin/env python3
"""Шаг 20: сглаживание DEM сепарабельным гауссианом 5x5 (два прохода).

Вход:  /work/dem/krai_dem.tif (Copernicus GLO-30, шумит артефактами
       интерферометрии — без сглаживания отмывка даёт «шубу»).
Выход: /work/relief/dem_smooth.tif. Размытие сохраняет крупную форму
       склонов — приём OpenTopoMap.
"""
import numpy as np
from osgeo import gdal

gdal.UseExceptions()

dem = gdal.Open('/work/dem/krai_dem.tif')
arr = dem.GetRasterBand(1).ReadAsArray().astype(np.float32)
kernel = np.array([1, 4, 6, 4, 1], dtype=np.float32)
kernel /= kernel.sum()
for axis_pass in range(2):
    arr = np.apply_along_axis(lambda r: np.convolve(r, kernel, mode='same'), 0, arr)
    arr = np.apply_along_axis(lambda r: np.convolve(r, kernel, mode='same'), 1, arr)
    print(f'smooth: проход {axis_pass + 1}/2', flush=True)

out = gdal.GetDriverByName('GTiff').Create(
    '/work/relief/dem_smooth.tif', dem.RasterXSize, dem.RasterYSize, 1,
    gdal.GDT_Float32, options=['TILED=YES', 'COMPRESS=DEFLATE', 'BIGTIFF=YES'],
)
out.SetGeoTransform(dem.GetGeoTransform())
out.SetProjection(dem.GetProjection())
out.GetRasterBand(1).WriteArray(arr)
out.FlushCache()
print('dem smoothed:', dem.RasterXSize, 'x', dem.RasterYSize, flush=True)
