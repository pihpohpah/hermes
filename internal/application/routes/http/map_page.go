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

// MapStyleHandler отдаёт style.json с поправкой на режим сервера.
//
// Без рельефа растровый источник и слой удаляются: иначе MapLibre будет
// запрашивать несуществующий /api/map/relief.pmtiles и сыпать ошибками на
// каждый кадр, а статус карты станет «часть тайлов недоступна».
type MapStyleHandler struct {
	stylePath string
	relief    bool
}

func NewMapStyleHandler(mapDir string, relief bool) *MapStyleHandler {
	return &MapStyleHandler{stylePath: filepath.Join(mapDir, "style.json"), relief: relief}
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
	if !h.relief {
		stripped, stripErr := stripRelief(raw)
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

// stripRelief removes the raster source and its layer from the style document.
func stripRelief(raw []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode map style: %w", err)
	}

	if sources, ok := doc["sources"].(map[string]any); ok {
		delete(sources, reliefSourceKey)
	}

	if layers, ok := doc["layers"].([]any); ok {
		kept := make([]any, 0, len(layers))
		for _, item := range layers {
			layer, isMap := item.(map[string]any)
			if isMap && layer["source"] == reliefSourceKey {
				continue
			}
			kept = append(kept, item)
		}
		doc["layers"] = kept
	}

	return json.Marshal(doc)
}

const reliefSourceKey = "relief"
