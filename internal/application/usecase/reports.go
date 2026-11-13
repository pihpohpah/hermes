package usecase

import (
	"context"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// ListReportsUseCase returns recent crowd reports for a place.
type ListReportsUseCase struct {
	reports reportLister
}

type reportLister interface {
	ListByPlace(ctx context.Context, filter models.ReportFilter) ([]models.Report, error)
}

func NewListReportsUseCase(reports reportLister) *ListReportsUseCase {
	return &ListReportsUseCase{reports: reports}
}

func (u *ListReportsUseCase) Do(ctx context.Context, filter models.ReportFilter) ([]models.Report, error) {
	placeID, err := models.ValidateUUID("place_id", filter.PlaceID)
	if err != nil {
		return nil, err
	}

	filter.PlaceID = placeID

	return u.reports.ListByPlace(ctx, filter)
}

// CreateReportUseCase stores a new report about current conditions.
type CreateReportUseCase struct {
	reports reportCreator
}

type reportCreator interface {
	Create(ctx context.Context, in models.CreateReportInput) (models.Report, error)
}

func NewCreateReportUseCase(reports reportCreator) *CreateReportUseCase {
	return &CreateReportUseCase{reports: reports}
}

func (u *CreateReportUseCase) Do(ctx context.Context, in models.CreateReportInput) (models.Report, error) {
	placeID, err := models.ValidateUUID("place_id", in.PlaceID)
	if err != nil {
		return models.Report{}, err
	}

	in.PlaceID = placeID

	if in.UserID != nil {
		userID, userErr := models.ValidateUUID("user_id", *in.UserID)
		if userErr != nil {
			return models.Report{}, userErr
		}

		in.UserID = &userID
	}

	if !in.Type.Valid() {
		return models.Report{}, models.Invalid("unexpected report type `" + string(in.Type) + "`")
	}

	return u.reports.Create(ctx, in)
}
