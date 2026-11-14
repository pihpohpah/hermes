package templates

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Форматирование чисел, дат и расстояний по-русски — порт core/format.js макета.
// Даты в данных — RFC3339 в UTC, показываются в часовом поясе Владивостока.

var vlTime = time.FixedZone("UTC+10", 10*3600)

func plural(count int, forms [3]string) string {
	n := int(math.Abs(float64(count))) % 100
	n1 := n % 10
	switch {
	case n > 10 && n < 20:
		return forms[2]
	case n1 > 1 && n1 < 5:
		return forms[1]
	case n1 == 1:
		return forms[0]
	default:
		return forms[2]
	}
}

func countLabel(count int, forms [3]string) string {
	return fmt.Sprintf("%d %s", count, plural(count, forms))
}

func formatDistance(km float64) string {
	switch {
	case km < 1:
		meters := math.Max(50, math.Round(km*1000/50)*50)
		return fmt.Sprintf("%.0f м", meters)
	case km < 10:
		return fmt.Sprintf("%.1f км", math.Round(km*10)/10)
	default:
		return fmt.Sprintf("%.0f км", math.Round(km))
	}
}

func formatDuration(minutes float64) string {
	total := int(math.Max(1, math.Round(minutes)))
	if total < 60 {
		return fmt.Sprintf("%d мин", total)
	}
	hours := total / 60
	rest := total % 60
	if hours >= 24 {
		days := int(math.Round(float64(hours) / 24))
		return fmt.Sprintf("%d %s", days, plural(days, [3]string{"день", "дня", "дней"}))
	}
	if rest > 0 {
		return fmt.Sprintf("%d ч %d мин", hours, rest)
	}
	return fmt.Sprintf("%d ч", hours)
}

var monthsShort = [12]string{"янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"}
var monthsLong = [12]string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}
var weekdaysShort = [7]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

func parseISODate(iso string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if ts, err := time.Parse(layout, iso); err == nil {
			return ts, true
		}
	}

	return time.Time{}, false
}

// formatDay: «21 ноября» (+ год, если не текущий).
func formatDay(iso string) string {
	ts, ok := parseISODate(iso)
	if !ok {
		return ""
	}
	date := ts.In(vlTime)
	now := time.Now().In(vlTime)
	label := fmt.Sprintf("%d %s", date.Day(), monthsLong[date.Month()-1])
	if date.Year() != now.Year() {
		label += fmt.Sprintf(" %d", date.Year())
	}

	return label
}

// formatEventWhen: «Сб, 21 ноября · 19:30».
func formatEventWhen(iso string) string {
	ts, ok := parseISODate(iso)
	if !ok {
		return ""
	}
	date := ts.In(vlTime)
	weekday := weekdaysShort[int(date.Weekday())]
	capitalized := strings.ToUpper(weekday[:1]) + weekday[1:]

	return fmt.Sprintf("%s, %s · %02d:%02d", capitalized, formatDay(iso), date.Hour(), date.Minute())
}

// dateTileParts для плитки даты: { day: "21", month: "ноя" }.
func dateTileParts(iso string) (day, month string) {
	ts, ok := parseISODate(iso)
	if !ok {
		return "", ""
	}
	date := ts.In(vlTime)

	return strconv.Itoa(date.Day()), monthsShort[date.Month()-1]
}

func formatDayShort(iso string) string {
	day, month := dateTileParts(iso)
	if day == "" {
		return ""
	}

	return day + " " + month
}

func formatTimeHM(ts time.Time) string {
	return fmt.Sprintf("%02d:%02d", ts.Hour(), ts.Minute())
}

// pinTiming — статус метки относительно текущего времени.
func pinTiming(startsAt, endsAt string) (state, label, short string) {
	start, ok1 := parseISODate(startsAt)
	end, ok2 := parseISODate(endsAt)
	if !ok1 || !ok2 {
		return "ended", "завершилась", "всё"
	}
	now := time.Now()
	if now.Before(start) {
		minutes := int(math.Round(start.Sub(now).Minutes()))
		if minutes < 1 {
			return "soon", "начинается", "сейчас"
		}
		d := formatDuration(float64(minutes))
		return "soon", "через " + d, d
	}
	if now.Before(end) {
		minutes := int(math.Round(end.Sub(now).Minutes()))
		return "live", "идёт, ещё " + formatDuration(float64(minutes)), "идёт"
	}

	return "ended", "завершилась", "всё"
}

// formatAgo: «5 минут назад».
func formatAgo(iso string) string {
	ts, ok := parseISODate(iso)
	if !ok {
		return ""
	}
	minutes := int(math.Round(time.Since(ts).Minutes()))
	if minutes < 1 {
		return "только что"
	}
	if minutes < 60 {
		return fmt.Sprintf("%d %s назад", minutes, plural(minutes, [3]string{"минуту", "минуты", "минут"}))
	}
	hours := int(math.Round(float64(minutes) / 60))
	if hours < 24 {
		return fmt.Sprintf("%d %s назад", hours, plural(hours, [3]string{"час", "часа", "часов"}))
	}

	return formatDay(iso)
}

func initials(name string) string {
	parts := strings.Fields(name)
	out := ""
	for i, part := range parts {
		if i >= 2 {
			break
		}
		if part != "" {
			out += strings.ToUpper(part[:1])
		}
	}

	return out
}

// avatarStyle — стабильный цвет аватара по имени (порт avatarStyle из format.js).
func avatarStyle(name string) string {
	hash := uint32(0)
	for _, ch := range name {
		hash = hash*31 + uint32(ch)
	}
	tones := [6][2]string{
		{"var(--orange-100)", "var(--orange-800)"},
		{"var(--violet-100)", "var(--violet-700)"},
		{"var(--blue-100)", "var(--blue-600)"},
		{"var(--green-100)", "var(--green-600)"},
		{"var(--yellow-100)", "var(--yellow-500)"},
		{"var(--red-100)", "var(--red-500)"},
	}
	tone := tones[int(hash%uint32(len(tones)))]

	return fmt.Sprintf("--avatar-bg:%s;--avatar-fg:%s", tone[0], tone[1])
}
