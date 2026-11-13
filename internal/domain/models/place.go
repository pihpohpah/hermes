package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type PlaceType string

const (
	PlaceTypePark      PlaceType = "park"
	PlaceTypeViewpoint PlaceType = "viewpoint"
	PlaceTypeSupSpot   PlaceType = "sup_spot"
	PlaceTypeTrailhead PlaceType = "trailhead"
	PlaceTypeBeach     PlaceType = "beach"
)

func NewPlaceType(raw string) (PlaceType, error) {
	switch PlaceType(strings.TrimSpace(raw)) {
	case PlaceTypePark:
		return PlaceTypePark, nil
	case PlaceTypeViewpoint:
		return PlaceTypeViewpoint, nil
	case PlaceTypeSupSpot:
		return PlaceTypeSupSpot, nil
	case PlaceTypeTrailhead:
		return PlaceTypeTrailhead, nil
	case PlaceTypeBeach:
		return PlaceTypeBeach, nil
	default:
		return "", Invalid(
			fmt.Sprintf("unexpected place type `%s`, expected: park, viewpoint, sup_spot, trailhead, beach", raw),
		)
	}
}

type LayerCode string

const (
	LayerCity   LayerCode = "city"
	LayerNature LayerCode = "nature"
	LayerSea    LayerCode = "sea"
)

func NewLayerCode(raw string) (LayerCode, error) {
	if strings.TrimSpace(raw) == "" {
		return LayerNature, nil
	}

	switch LayerCode(strings.TrimSpace(raw)) {
	case LayerCity:
		return LayerCity, nil
	case LayerNature:
		return LayerNature, nil
	case LayerSea:
		return LayerSea, nil
	default:
		return "", Invalid(fmt.Sprintf("unexpected layer `%s`, expected: city, nature, sea", raw))
	}
}

func DefaultLayerForType(pt PlaceType) LayerCode {
	switch pt {
	case PlaceTypeSupSpot, PlaceTypeBeach:
		return LayerSea
	case PlaceTypePark:
		return LayerCity
	case PlaceTypeViewpoint, PlaceTypeTrailhead:
		return LayerNature
	default:
		return LayerNature
	}
}

type Difficulty int

const (
	minDifficulty = 1
	maxDifficulty = 5
)

func NewDifficulty(raw int) (Difficulty, error) {
	if raw < minDifficulty || raw > maxDifficulty {
		return 0, Invalid(fmt.Sprintf("difficulty `%d` must be between 1 and 5", raw))
	}

	return Difficulty(raw), nil
}

// Place is the catalog card of a point on the map. The first fields are the
// original outdoor-service core; the rest is the guide enrichment from the
// design prototype (category, address, hours, price, photos and so on).
type Place struct {
	ID              string
	Slug            *string
	Name            string
	Subtitle        string
	Short           string
	Description     string
	Type            PlaceType
	Layer           LayerCode
	Category        *CategoryCode
	District        string
	Address         string
	Location        Point
	Hours           string
	PriceFree       *bool
	PriceLabel      string
	Duration        string
	Season          string
	Access          []AccessMode
	HowToGet        string
	Facts           []string
	Tags            []string
	Photos          []Photo
	Beach           *BeachType
	SwimSeason      string
	DistanceM       *float64
	Elevation       *int
	ElevationSource ElevationSource
	Difficulty      *Difficulty
	OSMID           *int64
	LatestScore     *ConditionScore
	CreatedAt       time.Time
}

type PlaceFilter struct {
	Center   *Point
	RadiusM  *RadiusMeters
	Type     *PlaceType
	Layer    *LayerCode
	Category *CategoryCode
	Q        *string
	Limit    int
}

type CreatePlaceInput struct {
	Name            string
	Slug            *string
	Subtitle        string
	Short           string
	Description     string
	Type            PlaceType
	Layer           LayerCode
	Category        *CategoryCode
	District        string
	Address         string
	Location        Point
	Hours           string
	PriceFree       *bool
	PriceLabel      string
	Duration        string
	Season          string
	Access          []AccessMode
	HowToGet        string
	Facts           []string
	Tags            []string
	Photos          []Photo
	Beach           *BeachType
	SwimSeason      string
	Elevation       *int
	ElevationSource ElevationSource
	Difficulty      *Difficulty
	OSMID           *int64
}

// SlugOrDefault returns the explicit slug or derives a stable one from the
// name so every catalog card has a human-readable URL part.
func (in CreatePlaceInput) SlugOrDefault() string {
	if in.Slug != nil && strings.TrimSpace(*in.Slug) != "" {
		return Slugify(*in.Slug)
	}

	return Slugify(in.Name)
}

var cyrillicTransliteration = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

// Slugify converts a free-text title into a lowercase ASCII slug. Characters
// outside a-z, 0-9 collapse into single dashes. An empty result falls back to
// a short content hash so the slug column stays unique and non-empty.
func Slugify(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))

	var b strings.Builder
	lastDash := false
	flushDash := func() {
		if b.Len() > 0 && !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}

	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			flushDash()
		default:
			if mapped, ok := cyrillicTransliteration[r]; ok {
				b.WriteString(mapped)
				lastDash = false
			} else {
				flushDash()
			}
		}
	}

	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		sum := sha256.Sum256([]byte(raw))
		slug = "x" + hex.EncodeToString(sum[:])[:8]
	}

	return slug
}
