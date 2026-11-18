#!/usr/bin/env python3
"""Шаг 40: сглаживание контуров алгоритмом Чайкина (corner cutting), 2 итерации.

Вход:  argv[1] — контуры из gdal_contour (вершины на каждом пикселе сетки —
       «лесенка» у колец, избыточные вершины в MVT).
       argv[3] — необязательно: число итераций (по умолчанию 2).
Выход: argv[2] — гладкие кривые.

Каждый отрезок (P0,P1) заменяется точками 0.75/0.25 и 0.25/0.75.
Дополнительно выкидываются соседние дублирующиеся вершины: gdal_contour
иногда отдаёт нулевые отрезки, на которых MVT-драйвер зависает при
нарезке z13+.
"""
import os
import sys

import numpy as np
from osgeo import gdal, ogr

gdal.UseExceptions()

ITERATIONS = int(sys.argv[3]) if len(sys.argv) > 3 else 2
# Линии длиннее MAX_PTS вершин режем на куски (см. ниже).
MAX_PTS = 50000
src_path, dst_path = sys.argv[1], sys.argv[2]

src = ogr.Open(src_path)
layer = src.GetLayer(0)
srs = layer.GetSpatialRef()
defn = layer.GetLayerDefn()

out_driver = ogr.GetDriverByName('GPKG')
if os.path.exists(dst_path):
    os.remove(dst_path)
out = out_driver.CreateDataSource(dst_path)
out_layer = out.CreateLayer('contours', srs, ogr.wkbLineString)
for i in range(defn.GetFieldCount()):
    out_layer.CreateField(defn.GetFieldDefn(i))
out_defn = out_layer.GetLayerDefn()

count = 0
for feature in layer:
    geom = feature.GetGeometryRef()
    if geom is None or geom.GetGeometryType() != ogr.wkbLineString:
        continue
    pts = np.array(geom.GetPoints())[:, :2]  # 2D, высота в поле ELEV
    if len(pts) < 3:
        continue
    closed = bool(geom.GetPointCount() > 2 and np.allclose(pts[0], pts[-1]))
    if closed:
        pts = pts[:-1]
    for _ in range(ITERATIONS):
        n = len(pts)
        nxt = np.roll(pts, -1, axis=0)  # кольцо замкнуто всегда
        q = 0.75 * pts + 0.25 * nxt
        r = 0.25 * pts + 0.75 * nxt
        new_pts = np.empty((2 * n, pts.shape[1]))
        new_pts[0::2] = q
        new_pts[1::2] = r
        pts = new_pts
    if closed:
        pts = np.vstack([pts, pts[0]])
    elif len(pts) >= 2:
        p0 = geom.GetPoint(0)
        pN = geom.GetPoint(geom.GetPointCount() - 1)
        pts[0] = (p0[0], p0[1])
        pts[-1] = (pN[0], pN[1])

    # нулевые отрезки (дубли соседних вершин) завешивают MVT-драйвер на z13+
    seg = np.sqrt(((pts[1:] - pts[:-1]) ** 2).sum(axis=1))
    keep = np.concatenate([[True], seg > 1e-12])
    pts = pts[keep]
    if len(pts) < 2:
        continue

    # gdal_contour может отдать один уровень ОДНОЙ линией через весь край
    # (5+ млн вершин): MVT-драйвер падает на таких («Unable to write
    # feature»). Режем длинные линии на куски с перекрытием в 1 точку —
    # стык на экране незаметен, геометрия не меняется.
    for start in range(0, len(pts), MAX_PTS - 1):
        chunk = pts[start:start + MAX_PTS]
        if len(chunk) < 2:
            continue
        new_geom = ogr.Geometry(ogr.wkbLineString)
        for x, y, *_ in chunk:
            new_geom.AddPoint(float(x), float(y))
        new_feature = ogr.Feature(out_defn)
        new_feature.SetGeometry(new_geom)
        for i in range(defn.GetFieldCount()):
            new_feature.SetField(defn.GetFieldDefn(i).GetNameRef(),
                                 feature.GetField(i))
        out_layer.CreateFeature(new_feature)
    count += 1
    if count % 50000 == 0:
        print(f'chaikin: {count} линий...', flush=True)

out = None
src = None
print(f'chaikin: {count} lines smoothed -> {dst_path}', flush=True)
