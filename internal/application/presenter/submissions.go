package presenter

import (
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// Submission is the HTTP view of one user proposal.
type Submission struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Status        string         `json:"status"`
	Scenario      string         `json:"scenario,omitempty"`
	Category      string         `json:"category,omitempty"`
	Title         string         `json:"title"`
	Address       string         `json:"address,omitempty"`
	Lat           float64        `json:"lat,omitempty"`
	Lon           float64        `json:"lon,omitempty"`
	Date          string         `json:"date,omitempty"`
	Time          string         `json:"time,omitempty"`
	Description   string         `json:"description,omitempty"`
	Hours         string         `json:"hours,omitempty"`
	Price         string         `json:"price,omitempty"`
	Link          string         `json:"link,omitempty"`
	FixID         string         `json:"fix_id,omitempty"`
	FixReasons    []string       `json:"fix_reasons,omitempty"`
	Contact       string         `json:"contact,omitempty"`
	Photos        []models.Photo `json:"photos"`
	ModeratorNote string         `json:"moderator_note,omitempty"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

func SubmissionToHTTP(in models.Submission) Submission {
	out := Submission{
		ID:            in.ID,
		Type:          string(in.Type),
		Status:        string(in.Status),
		Title:         in.Title,
		Address:       in.Address,
		Date:          "",
		Time:          in.EventTime,
		Description:   in.Description,
		Hours:         in.Hours,
		Price:         in.Price,
		Link:          in.Link,
		FixID:         in.FixID,
		FixReasons:    in.FixReasons,
		Contact:       in.Contact,
		Photos:        in.Photos,
		ModeratorNote: in.ModeratorNote,
		CreatedAt:     in.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     in.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if in.Scenario != nil {
		out.Scenario = string(*in.Scenario)
	}
	if in.Category != nil {
		out.Category = *in.Category
	}
	if in.Location != nil {
		out.Lat = in.Location.Lat
		out.Lon = in.Location.Lon
	}
	if in.EventDate != nil {
		out.Date = in.EventDate.UTC().Format("2006-01-02")
	}

	return out
}

func SubmissionsToHTTP(in []models.Submission) []Submission {
	out := make([]Submission, len(in))
	for i := range in {
		out[i] = SubmissionToHTTP(in[i])
	}

	return out
}

// PhotoUpload is the stub acknowledgement of an uploaded photo. Storage and
// moderation are not wired yet: Status stays "pending" and no URL is issued.
type PhotoUpload struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	URL    string `json:"url,omitempty"`
	Note   string `json:"note"`
}
