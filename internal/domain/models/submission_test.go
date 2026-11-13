package models_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

func TestCreateSubmissionInputValidate(t *testing.T) {
	city := models.LayerCity
	viewpoints := "viewpoints"
	beaches := "beaches"
	eventDate := time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		mutate  func(*models.CreateSubmissionInput)
		wantErr error
	}{
		{name: "place submission is valid", mutate: func(*models.CreateSubmissionInput) {}},
		{name: "empty title rejected", mutate: func(in *models.CreateSubmissionInput) {
			in.Title = " "
		}, wantErr: models.ErrInvalidArgument},
		{name: "fix without target rejected", mutate: func(in *models.CreateSubmissionInput) {
			in.Type = models.SubmissionTypeFix
			in.FixID = ""
		}, wantErr: models.ErrInvalidArgument},
		{name: "fix without reasons rejected", mutate: func(in *models.CreateSubmissionInput) {
			in.Type = models.SubmissionTypeFix
			in.FixReasons = nil
		}, wantErr: models.ErrInvalidArgument},
		{name: "event without date rejected", mutate: func(in *models.CreateSubmissionInput) {
			in.Type = models.SubmissionTypeEvent
			in.EventDate = nil
		}, wantErr: models.ErrInvalidArgument},
		{name: "category must match scenario", mutate: func(in *models.CreateSubmissionInput) {
			in.Category = &beaches
		}, wantErr: models.ErrInvalidArgument},
		{name: "matching category accepted", mutate: func(in *models.CreateSubmissionInput) {
			in.Category = &viewpoints
		}},
		{name: "too many photos rejected", mutate: func(in *models.CreateSubmissionInput) {
			in.Photos = make([]models.Photo, 11)
		}, wantErr: models.ErrInvalidArgument},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := models.CreateSubmissionInput{
				Type:      models.SubmissionTypePlace,
				Scenario:  &city,
				Title:     "Новая смотровая на сопке",
				EventDate: &eventDate,
				AuthorID:  "00000000-0000-0000-0000-000000000001",
			}
			tc.mutate(&in)

			err := in.Validate()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
