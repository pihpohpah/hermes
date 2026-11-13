package usecase

import (
	"context"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

type photoCreator interface {
	Create(ctx context.Context, in models.CreatePhotoInput) (models.StoredPhoto, error)
}

type CreatePhotoUseCase struct {
	photos photoCreator
}

func NewCreatePhotoUseCase(photos photoCreator) *CreatePhotoUseCase {
	return &CreatePhotoUseCase{photos: photos}
}

func (u *CreatePhotoUseCase) Do(ctx context.Context, in models.CreatePhotoInput) (models.StoredPhoto, error) {
	if err := in.Validate(); err != nil {
		return models.StoredPhoto{}, err
	}

	photo, err := u.photos.Create(ctx, in)
	if err != nil {
		return models.StoredPhoto{}, err
	}

	return photo, nil
}
