#!/usr/bin/env python3
"""Кодирует DEM в terrarium-RGB: h = R*256 + G + B/256 - 32768.

Вход:  /work/dem/dem_merc.tif (DEM уже в Web Mercator — bilinear применён
       к float-высотам, а не к байтам, иначе кодировка расплывётся).
Выход: /work/dem/dem_rgb.tif — нарезается без ресемплинга (шаг 96).

Яма за краем покрытия заливается нулём: -32768 м давало бы чёрные
провала в terrain.
"""
import numpy as np
from osgeo import gdal

gdal.UseExceptions()

dem = gdal.Open('/work/dem/dem_merc.tif')
band = dem.GetRasterBand(1)
arr = band.ReadAsArray().astype(np.float32)
nodata = band.GetNoDataValue()
valid = np.ones(arr.shape, dtype=bool) if nodata is None else arr != nodata

v = np.where(valid, np.clip(np.round(arr), -32768, 32767), 0).astype(np.int32) + 32768
r = (v >> 8).astype(np.uint8)
g = (v & 255).astype(np.uint8)
b = np.zeros_like(r)

out = gdal.GetDriverByName('GTiff').Create(
    '/work/dem/dem_rgb.tif', dem.RasterXSize, dem.RasterYSize, 3, gdal.GDT_Byte,
    options=['TILED=YES', 'COMPRESS=DEFLATE'],
)
out.SetGeoTransform(dem.GetGeoTransform())
out.SetProjection(dem.GetProjection())
for index, data in enumerate((r, g, b), 1):
    out.GetRasterBand(index).WriteArray(data)
out.FlushCache()
out = None
print('terrarium encoded:', dem.RasterXSize, 'x', dem.RasterYSize, flush=True)
