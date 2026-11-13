package models

import (
	"fmt"
	"math"
)

// Coordinate bounds of the WGS84 latitude/longitude pair used by the service.
const (
	minLatitude  = -90.0
	maxLatitude  = 90.0
	minLongitude = -180.0
	maxLongitude = 180.0
)

// Point is a geographic coordinate in WGS84 degrees (EPSG:4326).
type Point struct {
	Lat float64
	Lon float64
}

func NewPoint(lat, lon float64) (Point, error) {
	if math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) {
		return Point{}, Invalid("coordinates must be finite numbers")
	}

	if lat < minLatitude || lat > maxLatitude {
		return Point{}, Invalid(fmt.Sprintf("latitude `%g` must be between -90 and 90", lat))
	}

	if lon < minLongitude || lon > maxLongitude {
		return Point{}, Invalid(fmt.Sprintf("longitude `%g` must be between -180 and 180", lon))
	}

	return Point{Lat: lat, Lon: lon}, nil
}

// RadiusMeters is a search radius in meters, used by PostGIS ST_DWithin.
type RadiusMeters float64

func NewRadiusMeters(raw float64) (RadiusMeters, error) {
	if math.IsNaN(raw) || math.IsInf(raw, 0) {
		return 0, Invalid("radius must be a finite number")
	}

	if raw <= 0 {
		return 0, Invalid(fmt.Sprintf("radius `%g` must be positive", raw))
	}

	return RadiusMeters(raw), nil
}
