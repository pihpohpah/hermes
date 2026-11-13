package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MapPageHandler отдаёт страницу офлайн-карты.
//
// Страница самодостаточна: MapLibre, PMTiles, шрифты и style.json лежат рядом в
// web/map, поэтому карта рисуется при отключённой сети.
type MapPageHandler struct {
	mapDir string
}

func NewMapPageHandler(mapDir string) *MapPageHandler {
	return &MapPageHandler{mapDir: mapDir}
}

func (h *MapPageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	http.ServeFile(w, r, filepath.Join(h.mapDir, "index.html"))
}

// MapAssetsHandler раздаёт файлы карты (JS, CSS, шрифты, style.json).
type MapAssetsHandler struct {
	files http.Handler
}

func NewMapAssetsHandler(mapDir string) *MapAssetsHandler {
	// Запросы приходят как /map/style.json, а файлы лежат в mapDir/style.json:
	// префикс нужно срезать, иначе FileServer ищет mapDir/map/....
	return &MapAssetsHandler{files: http.StripPrefix("/map", http.FileServer(http.Dir(mapDir)))}
}

func (h *MapAssetsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Go не знает расширение .pbf и отдаёт глифы как octet-stream: MapLibre их
	// принимает, но диапазонные запросы и кеширование ведут себя нестабильно.
	if strings.HasSuffix(r.URL.Path, ".pbf") {
		w.Header().Set("Content-Type", "application/x-protobuf")
	}

	h.files.ServeHTTP(w, r)
}

// MapStyleHandler отдаёт style.json с поправкой на наличие архивов.
//
// Без рельефа вырезаются растровый источник зон и его слой, без DEM —
// источник raster-dem и hillshade: иначе MapLibre будет запрашивать
// несуществующие /api/map/*.pmtiles и сыпать ошибками на каждый кадр.
type MapStyleHandler struct {
	stylePath string
	relief    bool
	dem       bool
}

func NewMapStyleHandler(mapDir string, relief, dem bool) *MapStyleHandler {
	return &MapStyleHandler{stylePath: filepath.Join(mapDir, "style.json"), relief: relief, dem: dem}
}

func (h *MapStyleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	raw, err := os.ReadFile(h.stylePath)
	if err != nil {
		http.Error(w, "map style is not available", http.StatusServiceUnavailable)

		return
	}

	body := raw
	if !h.relief || !h.dem {
		stripped, stripErr := stripSources(raw, h.relief, h.dem)
		if stripErr != nil {
			http.Error(w, "map style is invalid", http.StatusInternalServerError)

			return
		}
		body = stripped
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}

// stripSources убирает источники и слои отсутствующих архивов из стиля.
func stripSources(raw []byte, relief, dem bool) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode map style: %w", err)
	}

	drop := map[string]bool{}
	if !relief {
		drop[reliefSourceKey] = true
	}
	if !dem {
		drop[demSourceKey] = true
	}

	if sources, ok := doc["sources"].(map[string]any); ok {
		for key := range drop {
			delete(sources, key)
		}
	}

	if layers, ok := doc["layers"].([]any); ok {
		kept := make([]any, 0, len(layers))
		for _, item := range layers {
			layer, isMap := item.(map[string]any)
			if isMap && drop[fmt.Sprintf("%v", layer["source"])] {
				continue
			}
			kept = append(kept, item)
		}
		doc["layers"] = kept
	}

	return json.Marshal(doc)
}

const (
	reliefSourceKey = "relief"
	demSourceKey    = "relief-dem"
)
