package presenter

import (
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// Pin is the HTTP view of a radar pin, decorated for the current user.
type Pin struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Note       string    `json:"note"`
	PlaceLabel string    `json:"place_label"`
	Lat        float64   `json:"lat"`
	Lon        float64   `json:"lon"`
	StartsAt   string    `json:"starts_at"`
	EndsAt     string    `json:"ends_at"`
	Need       *int      `json:"need"`
	Going      int       `json:"going"`
	GoingTotal int       `json:"going_total"`
	Joined     bool      `json:"joined"`
	Mine       bool      `json:"mine"`
	Author     PinAuthor `json:"author"`
	CreatedAt  string    `json:"created_at"`
}

type PinAuthor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func PinToHTTP(in models.Pin) Pin {
	return Pin{
		ID:         in.ID,
		Kind:       string(in.Kind),
		Title:      in.Title,
		Note:       in.Note,
		PlaceLabel: in.PlaceLabel,
		Lat:        in.Location.Lat,
		Lon:        in.Location.Lon,
		StartsAt:   in.StartsAt.UTC().Format(time.RFC3339),
		EndsAt:     in.EndsAt.UTC().Format(time.RFC3339),
		Need:       in.Need,
		Going:      in.Going,
		GoingTotal: in.GoingTotal,
		Joined:     in.Joined,
		Mine:       in.Mine,
		Author:     PinAuthor{ID: in.AuthorID, Name: in.AuthorName},
		CreatedAt:  in.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func PinsToHTTP(in []models.Pin) []Pin {
	out := make([]Pin, len(in))
	for i := range in {
		out[i] = PinToHTTP(in[i])
	}

	return out
}

// PinReport is the HTTP view of one complaint about a pin.
type PinReport struct {
	ID        string `json:"id"`
	PinID     string `json:"pin_id"`
	UserID    string `json:"user_id"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

func PinReportToHTTP(in models.PinReport) PinReport {
	return PinReport{
		ID:        in.ID,
		PinID:     in.PinID,
		UserID:    in.UserID,
		Reason:    in.Reason,
		CreatedAt: in.CreatedAt.UTC().Format(time.RFC3339),
	}
}
