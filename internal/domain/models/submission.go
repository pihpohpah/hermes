package models

import (
	"fmt"
	"strings"
	"time"
)

// SubmissionType is the kind of a user proposal: a new place, a new event,
// or a fix to an existing catalog entry.
type SubmissionType string

const (
	SubmissionTypePlace SubmissionType = "place"
	SubmissionTypeEvent SubmissionType = "event"
	SubmissionTypeFix   SubmissionType = "fix"
)

func NewSubmissionType(raw string) (SubmissionType, error) {
	submissionType := SubmissionType(strings.TrimSpace(raw))
	switch submissionType {
	case SubmissionTypePlace, SubmissionTypeEvent, SubmissionTypeFix:
		return submissionType, nil
	default:
		return "", Invalid(fmt.Sprintf(
			"unexpected submission type `%s`, expected: place, event, fix", raw,
		))
	}
}

// SubmissionStatus is the editorial lifecycle of a proposal.
type SubmissionStatus string

const (
	SubmissionStatusReview    SubmissionStatus = "review"
	SubmissionStatusPublished SubmissionStatus = "published"
	SubmissionStatusRejected  SubmissionStatus = "rejected"
)

func NewSubmissionStatus(raw string) (SubmissionStatus, error) {
	status := SubmissionStatus(strings.TrimSpace(raw))
	switch status {
	case SubmissionStatusReview, SubmissionStatusPublished, SubmissionStatusRejected:
		return status, nil
	default:
		return "", Invalid(fmt.Sprintf(
			"unexpected submission status `%s`, expected: review, published, rejected", raw,
		))
	}
}

const (
	maxSubmissionTitleLength   = 200
	maxSubmissionContactLength = 140
	maxSubmissionPhotos        = 10
)

// Submission is one user proposal awaiting (or having passed) editorial review.
type Submission struct {
	ID            string
	AuthorID      string
	Type          SubmissionType
	Status        SubmissionStatus
	Scenario      *LayerCode
	Category      *string
	Title         string
	Address       string
	Location      *Point
	EventDate     *time.Time
	EventTime     string
	Description   string
	Hours         string
	Price         string
	Link          string
	FixID         string
	FixReasons    []string
	Contact       string
	Photos        []Photo
	ModeratorNote string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// CreateSubmissionInput is the parsed payload of POST /submissions plus the
// author from the session and the idempotency contract (empty key is allowed).
type CreateSubmissionInput struct {
	Type      SubmissionType
	Scenario  *LayerCode
	Category  *string
	Title     string
	Address   string
	Location  *Point
	EventDate *time.Time
	EventTime string

	Description string
	Hours       string
	Price       string
	Link        string
	FixID       string
	FixReasons  []string
	Contact     string
	Photos      []Photo

	AuthorID           string
	IdempotencyKey     string
	RequestFingerprint string
}

// Validate enforces the per-type shape: a fix must point at an entry and say
// what is wrong, an event carries a date, everything has a title.
func (in CreateSubmissionInput) Validate() error {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Invalid("`title` is required")
	}
	if len(title) > maxSubmissionTitleLength {
		return Invalidf("`title` must be at most %d characters", maxSubmissionTitleLength)
	}
	if len(in.Contact) > maxSubmissionContactLength {
		return Invalidf("`contact` must be at most %d characters", maxSubmissionContactLength)
	}
	if len(in.Photos) > maxSubmissionPhotos {
		return Invalidf("at most %d photos per submission", maxSubmissionPhotos)
	}

	switch in.Type {
	case SubmissionTypeFix:
		if strings.TrimSpace(in.FixID) == "" {
			return Invalid("`fix_id` is required for a fix submission")
		}
		if len(in.FixReasons) == 0 {
			return Invalid("`fix_reasons` is required for a fix submission")
		}
	case SubmissionTypeEvent:
		if in.EventDate == nil {
			return Invalid("`date` is required for an event submission")
		}
	}

	if in.Scenario != nil && in.Category != nil {
		category, err := NewCategoryCode(*in.Category)
		if err != nil {
			return err
		}
		if category.Scenario() != *in.Scenario {
			return Invalidf("category `%s` does not belong to scenario `%s`", *in.Category, *in.Scenario)
		}
	}

	return nil
}

// SubmissionFilter narrows the author's own list.
type SubmissionFilter struct {
	AuthorID string
	Status   *SubmissionStatus
	Limit    int
}
