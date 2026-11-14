package eventagent

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// Контракт ответов /runs и /runs/{id} сервиса event-agent (см. schemas.py
// webscraper). Даты приходят строками (YYYY-MM-DD, HH:MM) и здесь же
// превращаются в доменные типы — parse, don't validate.
type runResponse struct {
	RunID          string          `json:"run_id"`
	Status         string          `json:"status"`
	AlreadyRunning bool            `json:"already_running"`
	Error          *string         `json:"error"`
	Events         []eventResponse `json:"events"`
}

type eventResponse struct {
	ID          string   `json:"id"`
	Change      string   `json:"change"`
	Title       string   `json:"title"`
	Category    string   `json:"category"`
	StartDate   string   `json:"start_date"`
	EndDate     *string  `json:"end_date"`
	StartTime   *string  `json:"start_time"`
	City        *string  `json:"city"`
	Venue       *string  `json:"venue"`
	Price       *string  `json:"price"`
	IsFree      *bool    `json:"is_free"`
	Description *string  `json:"description"`
	EventURL    *string  `json:"event_url"`
	IsCancelled bool     `json:"is_cancelled"`
	SourceURLs  []string `json:"source_urls"`
}

const (
	dateLayout = "2006-01-02"
	timeLayout = "15:04"
)

func (r runResponse) toDomain() (models.ImportedRun, error) {
	status, err := models.NewRunStatus(r.Status)
	if err != nil {
		return models.ImportedRun{}, err
	}

	events := make([]models.Event, 0, len(r.Events))
	for _, dto := range r.Events {
		event, mapErr := dto.toDomain()
		if mapErr != nil {
			return models.ImportedRun{}, fmt.Errorf("event `%s`: %w", dto.ID, mapErr)
		}
		events = append(events, event)
	}

	return models.ImportedRun{
		ID:             r.RunID,
		Status:         status,
		AlreadyRunning: r.AlreadyRunning,
		Error:          r.Error,
		Events:         events,
	}, nil
}

func (e eventResponse) toDomain() (models.Event, error) {
	return e.toDomainWithID(e.ID)
}

// toDomainWithID мапит событие с явным внешним ключом: в /state id лежит в
// ключе мапы, а не внутри самого события.
func (e eventResponse) toDomainWithID(externalID string) (models.Event, error) {
	category, err := models.NewEventCategoryCode(e.Category)
	if err != nil {
		return models.Event{}, err
	}

	loc, err := models.PrimoryeLocation()
	if err != nil {
		return models.Event{}, fmt.Errorf("load primorye location: %w", err)
	}

	start, end, err := resolveTimeWindow(e, loc)
	if err != nil {
		return models.Event{}, err
	}

	event := models.Event{
		ExternalID:  &externalID,
		Title:       e.Title,
		Category:    category,
		StartsAt:    start,
		EndsAt:      end,
		Description: stringValue(e.Description),
		Venue:       stringValue(e.Venue),
		Status:      eventStatus(e.IsCancelled),
		PriceFree:   e.IsFree,
		PriceLabel:  stringValue(e.Price),
		EventURL:    httpURL(e.EventURL),
		SourceURLs:  sourceURLMap(e.SourceURLs),
	}

	return event, nil
}

func eventStatus(isCancelled bool) models.EventStatus {
	if isCancelled {
		return models.EventStatusCancelled
	}

	return models.EventStatusScheduled
}

// httpURL keeps only http(s) page addresses; anything else becomes nil, so a
// malformed URL can not break the catalog CHECK constraint.
func httpURL(raw *string) *string {
	if raw == nil {
		return nil
	}
	value := strings.TrimSpace(*raw)
	if !strings.HasPrefix(value, "https://") && !strings.HasPrefix(value, "http://") {
		return nil
	}

	return &value
}

// sourceURLMap keys source page URLs by host, so the API can tell a listing
// on ticketland.ru from an announcement on visit-primorye.ru. The first page
// of a host wins.
func sourceURLMap(urls []string) map[string]string {
	if len(urls) == 0 {
		return nil
	}

	out := make(map[string]string, len(urls))
	for _, raw := range urls {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			continue
		}
		host := parsed.Hostname()
		if _, exists := out[host]; !exists {
			out[host] = raw
		}
	}

	return out
}

// resolveTimeWindow собирает starts_at/ends_at из календарных дат агента:
// время начала берётся из start_time (иначе 00:00), конец — конец последнего
// дня события (end_date, иначе день начала) по Владивостоку.
func resolveTimeWindow(e eventResponse, loc *time.Location) (time.Time, time.Time, error) {
	day, err := time.ParseInLocation(dateLayout, e.StartDate, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse start_date `%s`: %w", e.StartDate, err)
	}

	hour, minute := 0, 0
	if e.StartTime != nil {
		parsedTime, parseErr := time.Parse(timeLayout, *e.StartTime)
		if parseErr != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse start_time `%s`: %w", *e.StartTime, parseErr)
		}
		hour, minute = parsedTime.Hour(), parsedTime.Minute()
	}

	endDay := day
	if e.EndDate != nil {
		endDay, err = time.ParseInLocation(dateLayout, *e.EndDate, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse end_date `%s`: %w", *e.EndDate, err)
		}
	}

	start := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc)
	end := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), 23, 59, 59, 0, loc)

	return start.UTC(), end.UTC(), nil
}

func stringValue(raw *string) string {
	if raw == nil {
		return ""
	}

	return *raw
}
