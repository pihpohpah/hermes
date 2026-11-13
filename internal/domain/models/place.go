package models

import (
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

type Place struct {
	ID              string
	Name            string
	Description     string
	Type            PlaceType
	Layer           LayerCode
	Location        Point
	DistanceM       *float64
	Elevation       *int
	ElevationSource ElevationSource
	Difficulty      *Difficulty
	OSMID           *int64
	LatestScore     *ConditionScore
	CreatedAt       time.Time
}

type PlaceFilter struct {
	Center  *Point
	RadiusM *RadiusMeters
	Type    *PlaceType
	Layer   *LayerCode
	Limit   int
}

type CreatePlaceInput struct {
	Name            string
	Description     string
	Type            PlaceType
	Layer           LayerCode
	Location        Point
	Elevation       *int
	ElevationSource ElevationSource
	Difficulty      *Difficulty
	OSMID           *int64
}
