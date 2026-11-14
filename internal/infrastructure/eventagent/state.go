package eventagent

import (
	"fmt"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// Контракт ответа GET /state: сырое состояние дедупликации агента. events —
// мапа «стабильный id события → запись», внутри event лежит то же событие,
// что и в /runs/{id}, только без id и source_urls: источники — в поле sources
// записи.
type stateResponse struct {
	Events map[string]stateEntryResponse `json:"events"`
}

type stateEntryResponse struct {
	Event   eventResponse `json:"event"`
	Sources []string      `json:"sources"`
}

func (s stateResponse) toDomainEvents() ([]models.Event, error) {
	events := make([]models.Event, 0, len(s.Events))
	for id, entry := range s.Events {
		event, err := entry.Event.toDomainWithID(id)
		if err != nil {
			return nil, fmt.Errorf("state event `%s`: %w", id, err)
		}
		if len(event.SourceURLs) == 0 {
			event.SourceURLs = sourceURLMap(entry.Sources)
		}
		events = append(events, event)
	}

	return events, nil
}
