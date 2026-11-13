package models

import (
	"fmt"
	"strings"
)

// ElevationSource records where a place elevation came from, so it is possible
// to tell a hand-entered value from a DEM-derived one.
type ElevationSource string

const (
	ElevationSourceUnknown   ElevationSource = ""
	ElevationSourceManual    ElevationSource = "manual"
	ElevationSourceOpenMeteo ElevationSource = "open_meteo"
	ElevationSourceDEM       ElevationSource = "dem"
)

// NewElevationSource parses a stored elevation source.
func NewElevationSource(raw string) (ElevationSource, error) {
	switch ElevationSource(strings.TrimSpace(raw)) {
	case ElevationSourceUnknown:
		return ElevationSourceUnknown, nil
	case ElevationSourceManual:
		return ElevationSourceManual, nil
	case ElevationSourceOpenMeteo:
		return ElevationSourceOpenMeteo, nil
	case ElevationSourceDEM:
		return ElevationSourceDEM, nil
	default:
		return "", Invalid(
			fmt.Sprintf("unexpected elevation source `%s`, expected: manual, open_meteo, dem", raw),
		)
	}
}

const (
	minElevationM = -500
	maxElevationM = 9000
)

// Elevation is a place altitude with provenance.
type Elevation struct {
	Meters int
	Source ElevationSource
}

func NewElevation(meters int, source ElevationSource) (Elevation, error) {
	if meters < minElevationM || meters > maxElevationM {
		return Elevation{}, Invalid(fmt.Sprintf("elevation `%d` m is out of range", meters))
	}

	return Elevation{Meters: meters, Source: source}, nil
}

// ElevationTerrainClass tells how much a forecast at this point can be trusted
// given that the weather model cell may be much coarser than the terrain.
type ElevationTerrainClass string

const (
	// TerrainFlat means the model cell is likely representative.
	TerrainFlat ElevationTerrainClass = "flat"
	// TerrainHilly means elevation error meaningfully shifts temperature.
	TerrainHilly ElevationTerrainClass = "hilly"
	// TerrainMountainous means the model cell almost certainly misses the relief.
	TerrainMountainous ElevationTerrainClass = "mountainous"
)

const (
	hillyElevationM       = 100
	mountainousElevationM = 300
)

func ClassifyTerrain(meters int) ElevationTerrainClass {
	switch {
	case meters >= mountainousElevationM:
		return TerrainMountainous
	case meters >= hillyElevationM:
		return TerrainHilly
	default:
		return TerrainFlat
	}
}

// TemperatureLapseRateCPer100M is the standard tropospheric lapse rate used to
// warn that a coarse model cell is warmer than the actual summit.
const TemperatureLapseRateCPer100M = 0.65

// LapseCorrection estimates how much colder the real point is than a model cell
// whose elevation is modelElevationM. It is a diagnostic, not a substitution for
// a real forecast at that elevation.
func LapseCorrection(placeElevationM int, modelElevationM float64) float64 {
	delta := float64(placeElevationM) - modelElevationM

	return -delta / 100 * TemperatureLapseRateCPer100M
}
