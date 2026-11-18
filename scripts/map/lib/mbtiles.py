#!/usr/bin/env python3
"""Тайлы из каталога {z}/{x}/{y}.<ext> в MBTiles (SQLite).

Вход:  TILES_ROOT — каталог тайлов, TILE_EXT (png|pbf)
Выход: MBTILES_OUT — SQLite: metadata + tiles (z, x, y-TMS, blob)

gdal2tiles пишет XYZ, MBTiles хранит TMS — Y переворачивается. Исходные
файлы удаляются по мере вставки: держать и каталог, и копию в SQLite
на один объём диска не хватает. Индекс (z,x,y) обязателен: go-pmtiles
convert ищет каждый тайл запросом, без индекса конвертация 350k тайлов
заняла бы ~7 часов вместо ~1 минуты.
"""
import os
import sqlite3

tiles_root = os.environ['TILES_ROOT']
out = os.environ['MBTILES_OUT']
name = os.environ.get('META_NAME', 'relief')
tile_ext = os.environ.get('TILE_EXT', 'png')
if os.path.exists(out):
    os.remove(out)

db = sqlite3.connect(out)
db.execute('CREATE TABLE metadata (name text, value text)')
db.execute('CREATE TABLE tiles (zoom_level integer, tile_column integer, tile_row integer, tile_data blob)')

min_zoom = max_zoom = None
count = 0
for zoom in sorted(os.listdir(tiles_root)):
    if not zoom.isdigit():
        continue
    z = int(zoom)
    for x in os.listdir(os.path.join(tiles_root, zoom)):
        xdir = os.path.join(tiles_root, zoom, x)
        if not os.path.isdir(xdir):
            continue
        for fname in os.listdir(xdir):
            if not fname.endswith('.' + tile_ext):
                continue
            y = int(fname[:-len(tile_ext) - 1])
            with open(os.path.join(xdir, fname), 'rb') as handle:
                data = handle.read()
            db.execute(
                'INSERT INTO tiles VALUES (?, ?, ?, ?)',
                (z, int(x), (2 ** z - 1) - y, data),
            )
            os.remove(os.path.join(xdir, fname))
            count += 1
            min_zoom = z if min_zoom is None else min(min_zoom, z)
            max_zoom = z if max_zoom is None else max(max_zoom, z)
    print(f'[{name}] z{z} собран, всего {count} тайлов', flush=True)

meta = {
    'name': name,
    'format': 'pbf' if tile_ext == 'pbf' else 'png',
    'minzoom': str(min_zoom),
    'maxzoom': str(max_zoom),
    # Векторные тайлы: pmtiles-протокол MapLibre читает TileJSON из metadata
    # json и без vector_layers не поднимет источник.
    'json': os.environ.get('META_JSON', ''),
    'type': 'baselayer',
    # Честный bbox покрытия: у локальной сборки он меньше края.
    'bounds': os.environ.get('BBOX', '130.394,42.123,139.309,48.465'),
}
for key, value in meta.items():
    db.execute('INSERT INTO metadata VALUES (?, ?)', (key, value))
db.execute('CREATE UNIQUE INDEX tiles_idx ON tiles (zoom_level, tile_column, tile_row)')
db.commit()
db.close()
print(f'{name}: {count} tiles, z{min_zoom}-{max_zoom}, {tile_ext}', flush=True)
