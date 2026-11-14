package templates

import (
	"strconv"
	"strings"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

// Мелкие хелперы для шаблонов (см. components.templ).

func photoAt(item presenter.CatalogItem, index int) *models.Photo {
	if index < 0 || index >= len(item.Photos) {
		return nil
	}

	return &item.Photos[index]
}

func mediaRatioStyle(ratio string) string {
	if ratio == "" {
		return ""
	}

	return "--media-ratio:" + ratio
}

func mediaSrc(photo *models.Photo, size string) string {
	if size == "lg" || photo.SrcSm == "" {
		return photo.Src
	}

	return photo.SrcSm
}

func mediaAlt(photo *models.Photo, item presenter.CatalogItem) string {
	if photo.Alt != "" {
		return photo.Alt
	}

	return item.Title
}

func mediaDim(value, fallback int) string {
	if value <= 0 {
		value = fallback
	}

	return strconv.Itoa(value)
}

func mediaLoading(eager bool) string {
	if eager {
		return "eager"
	}

	return "lazy"
}

func cardRowClass(layout string) string {
	if layout == "row" {
		return " place-card--row"
	}

	return ""
}

func ariaCurrent(current bool) string {
	if current {
		return "true"
	}

	return "false"
}

func difficultyCodeOf(item presenter.CatalogItem) string {
	if item.Difficulty == nil {
		return ""
	}
	switch {
	case *item.Difficulty <= 2:
		return "easy"
	case *item.Difficulty == 3:
		return "medium"
	default:
		return "hard"
	}
}

func priceLabel(item presenter.CatalogItem) string {
	if item.PriceFree != nil && *item.PriceFree {
		return "Бесплатно"
	}
	if item.PriceLabel != "" {
		return item.PriceLabel
	}

	return "Уточняйте"
}

func metaSecond(item presenter.CatalogItem) string {
	if item.Type == "event" {
		return eventWhenDate(item.StartsAt)
	}

	return item.District
}

// eventWhenDate / eventWhenTime — части «Сб, 21 ноября · 19:30».
func eventWhenDate(iso string) string {
	parts := strings.SplitN(formatEventWhen(iso), " · ", 2)
	if len(parts) == 0 {
		return ""
	}

	return parts[0]
}

func eventWhenTime(iso string) string {
	parts := strings.SplitN(formatEventWhen(iso), " · ", 2)
	if len(parts) < 2 {
		return ""
	}

	return parts[1]
}

func pinNeedLeft(pin presenter.Pin) int {
	if pin.Need == nil {
		return 0
	}
	left := *pin.Need
	if pin.Joined && !pin.Mine {
		left = max(0, left-1)
	}

	return left
}

func pinMeterTotal(pin presenter.Pin) int {
	if pin.Need == nil {
		return pin.GoingTotal
	}

	return pin.GoingTotal + *pin.Need
}

func pinMeterPercent(pin presenter.Pin) int {
	total := pinMeterTotal(pin)
	if total <= 0 {
		return 0
	}

	return min(100, int(float64(pin.GoingTotal)/float64(total)*100+0.5))
}

func pinMineClass(pin presenter.Pin) string {
	if pin.Mine {
		return " pin-card--mine"
	}

	return ""
}

func mapBoolIcon(cond bool, whenFalse string) string {
	if cond {
		return "check"
	}

	return whenFalse
}

func dotClass(index int) string {
	if index == 0 {
		return "is-active"
	}

	return ""
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
