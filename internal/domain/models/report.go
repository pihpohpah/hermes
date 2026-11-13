package models

import "time"

// ReportType is a short crowd-sourced observation about a place.
type ReportType string

const (
	ReportClear   ReportType = "clear"
	ReportFog     ReportType = "fog"
	ReportWaves   ReportType = "waves"
	ReportCrowded ReportType = "crowded"
	ReportClosed  ReportType = "closed"
)

func NewReportType(raw string) (ReportType, error) {
	t := ReportType(raw)
	if !t.Valid() {
		return "", Invalid("unexpected report type `" + raw + "`, expected: clear, fog, waves, crowded, closed")
	}

	return t, nil
}

func (t ReportType) Valid() bool {
	switch t {
	case ReportClear, ReportFog, ReportWaves, ReportCrowded, ReportClosed:
		return true
	default:
		return false
	}
}

// Report is a user-submitted note about current conditions at a place.
type Report struct {
	ID        string
	PlaceID   string
	UserID    *string
	Type      ReportType
	Comment   string
	CreatedAt time.Time
}

// CreateReportInput is the parsed payload of POST /reports.
type CreateReportInput struct {
	PlaceID string
	UserID  *string
	Type    ReportType
	Comment string
}

// ReportFilter narrows reports to one place.
type ReportFilter struct {
	PlaceID string
	Limit   int
}
