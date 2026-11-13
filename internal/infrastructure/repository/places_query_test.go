package repository

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

var placeholderPattern = regexp.MustCompile(`\$([0-9]+)`)

func placeholders(query string) []int {
	seen := make(map[int]struct{})
	numbers := make([]int, 0)
	for _, match := range placeholderPattern.FindAllStringSubmatch(query, -1) {
		number, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		if _, exists := seen[number]; exists {
			continue
		}
		seen[number] = struct{}{}
		numbers = append(numbers, number)
	}

	return numbers
}

func radius(meters float64) *models.RadiusMeters {
	r := models.RadiusMeters(meters)

	return &r
}

func TestBuildPlaceListQueryPlaceholdersMatchArgs(t *testing.T) {
	t.Parallel()

	center := models.Point{Lat: 43.1155, Lon: 131.8855}

	tests := []struct {
		name   string
		filter models.PlaceFilter
	}{
		{name: "no filters", filter: models.PlaceFilter{}},
		{name: "limit only", filter: models.PlaceFilter{Limit: 10}},
		{name: "type filter", filter: models.PlaceFilter{Type: new(models.PlaceTypeSupSpot)}},
		{name: "layer filter", filter: models.PlaceFilter{Layer: new(models.LayerSea)}},
		{name: "radius only", filter: models.PlaceFilter{Center: &center, RadiusM: radius(1500)}},
		{
			name: "radius, type and layer",
			filter: models.PlaceFilter{
				Center:  &center,
				RadiusM: radius(25000),
				Type:    new(models.PlaceTypeTrailhead),
				Layer:   new(models.LayerNature),
				Limit:   50,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			query, args := buildPlaceListQuery(tc.filter)
			numbers := placeholders(query)

			if len(numbers) != len(args) {
				t.Fatalf("query uses %d placeholders but %d args were bound: %v", len(numbers), len(args), numbers)
			}
			for _, number := range numbers {
				if number < 1 || number > len(args) {
					t.Fatalf("placeholder $%d has no argument (args: %d)", number, len(args))
				}
			}
		})
	}
}

func TestBuildPlaceListQueryKeepsLimitPositional(t *testing.T) {
	t.Parallel()

	center := models.Point{Lat: 43.1155, Lon: 131.8855}
	query, args := buildPlaceListQuery(models.PlaceFilter{
		Center:  &center,
		RadiusM: radius(1000),
		Type:    new(models.PlaceTypePark),
		Limit:   7,
	})

	matches := regexp.MustCompile(`LIMIT \$([0-9]+)`).FindStringSubmatch(query)
	if matches == nil {
		t.Fatalf("query has no LIMIT placeholder: %s", query)
	}
	limitPos, err := strconv.Atoi(matches[1])
	if err != nil {
		t.Fatalf("parse limit placeholder: %v", err)
	}
	if limitPos < 1 || limitPos > len(args) {
		t.Fatalf("LIMIT $%d is out of range for %d args", limitPos, len(args))
	}
	if got, ok := args[limitPos-1].(int); !ok || got != 7 {
		t.Fatalf("LIMIT bound to %v, want 7", args[limitPos-1])
	}
}

func TestBuildPlaceListQueryPrefersCurrentActivityAndAlgorithm(t *testing.T) {
	t.Parallel()

	query, _ := buildPlaceListQuery(models.PlaceFilter{})

	for _, fragment := range []string{
		"CASE WHEN cc.activity = $1 THEN 0 ELSE 1 END",
		"CASE WHEN cc.algorithm_version = $2 THEN 0 ELSE 1 END",
	} {
		if !regexp.MustCompile(regexp.QuoteMeta(fragment)).MatchString(query) {
			t.Fatalf("query is missing %q", fragment)
		}
	}
}
