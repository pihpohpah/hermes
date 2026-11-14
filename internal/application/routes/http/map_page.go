package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MapAssetsHandler раздаёт файлы карты (MapLibre, PMTiles, шрифты).
type MapAssetsHandler struct {
	files http.Handler
}

func NewMapAssetsHandler(mapDir string) *MapAssetsHandler {
	// Запросы приходят как /map/vendor/..., а файлы лежат в mapDir/vendor/...:
	// префикс нужно срезать, иначе FileServer ищет mapDir/map/....
	return &MapAssetsHandler{files: http.StripPrefix("/map", http.FileServer(http.Dir(mapDir)))}
}

func (h *MapAssetsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Только файлы: листинг каталогов наружу не отдаём.
	if strings.HasSuffix(r.URL.Path, "/") {
		http.NotFound(w, r)

		return
	}

	// Go не знает расширение .pbf и отдаёт глифы как octet-stream: MapLibre их
	// принимает, но диапазонные запросы и кеширование ведут себя нестабильно.
	if strings.HasSuffix(r.URL.Path, ".pbf") {
		w.Header().Set("Content-Type", "application/x-protobuf")
	}

	h.files.ServeHTTP(w, r)
}

// MapStyleHandler отдаёт style.json с поправкой на наличие архива рельефа.
//
// Без архива вырезаются растровый источник рельефа и его слои, иначе MapLibre
// запрашивает несуществующий /api/map/relief.pmtiles и сыпет ошибками на каждый
// кадр. Наличие проверяется на каждый запрос: архив можно собрать, не
// перезапуская сервер.
type MapStyleHandler struct {
	stylePath  string
	reliefPath string
}

func NewMapStyleHandler(mapDir, reliefPath string) *MapStyleHandler {
	return &MapStyleHandler{stylePath: filepath.Join(mapDir, "style.json"), reliefPath: reliefPath}
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
	if _, statErr := os.Stat(h.reliefPath); statErr != nil {
		stripped, stripErr := stripSource(raw, reliefSourceKey)
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

// stripSource убирает из стиля источник и все слои, которые его рисуют.
func stripSource(raw []byte, source string) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode map style: %w", err)
	}

	if sources, ok := doc["sources"].(map[string]any); ok {
		delete(sources, source)
	}

	if layers, ok := doc["layers"].([]any); ok {
		kept := make([]any, 0, len(layers))
		for _, item := range layers {
			layer, isMap := item.(map[string]any)
			if isMap && layer["source"] == source {
				continue
			}
			kept = append(kept, item)
		}
		doc["layers"] = kept
	}

	return json.Marshal(doc)
}

const reliefSourceKey = "relief"
