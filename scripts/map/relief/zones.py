#!/usr/bin/env python3
"""Пишет плавный высотный рамп (фон карты) по min/max DEM.

Вход:  /work/dem/krai_dem.tif. Выход: /work/relief/ramp.txt для
gdaldem color-relief. Фон — плавный зелёный градиент в духе
mapy.com/OpenTopoMap: форму рельефа дают отмывка и горизонтали,
резкие ступени не нужны. Шаг RAMP_STEP, между точками color-relief
интерполирует.
"""
from osgeo import gdal

gdal.UseExceptions()

RAMP_STEP = 25  # м; плавность градиента фона

PALETTE = [
    (0.00, (222, 230, 178)),  # низины: тёплый светло-зелёный
    (0.10, (196, 216, 148)),
    (0.25, (168, 202, 122)),
    (0.42, (142, 188, 104)),
    (0.58, (124, 174, 100)),
    (0.72, (116, 162, 104)),
    (0.85, (124, 152, 106)),
    (1.00, (140, 142, 102)),  # высоты: тёплый оливковый
]


def pick_color(t):
    for (t0, c0), (t1, c1) in zip(PALETTE, PALETTE[1:]):
        if t <= t1:
            k = 0.0 if t1 == t0 else (t - t0) / (t1 - t0)
            return tuple(round(a + (b - a) * k) for a, b in zip(c0, c1))
    return PALETTE[-1][1]


dem = gdal.Open('/work/dem/krai_dem.tif')
band = dem.GetRasterBand(1)
minimum, maximum = band.ComputeStatistics(False)[:2]

lo = int(minimum // RAMP_STEP) * RAMP_STEP
hi = int(-(-maximum // RAMP_STEP)) * RAMP_STEP  # ceil

with open('/work/relief/ramp.txt', 'w') as out:
    h = lo
    while h <= hi:
        r, g, b = pick_color((h - minimum) / max(maximum - minimum, 1.0))
        out.write(f'{h:.2f} {r} {g} {b}\n')
        h += RAMP_STEP

print(f'elevation range: {minimum:.0f}..{maximum:.0f} m, '
      f'smooth ramp {lo}..{hi} by {RAMP_STEP} m', flush=True)
