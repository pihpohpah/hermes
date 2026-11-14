package models

// RunStatus is the lifecycle state of one event-agent scraping run, mirrored
// from the /runs/{id} contract of the importing service.
type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusPartial   RunStatus = "partial"
	RunStatusFailed    RunStatus = "failed"
)

func NewRunStatus(raw string) (RunStatus, error) {
	status := RunStatus(raw)
	switch status {
	case RunStatusRunning, RunStatusCompleted, RunStatusPartial, RunStatusFailed:
		return status, nil
	default:
		return "", Invalidf("unexpected run status `%s`", raw)
	}
}

// Terminal reports whether the run has finished (successfully or not).
func (s RunStatus) Terminal() bool {
	return s != RunStatusRunning
}

// ImportedRun is the domain view of one event-agent run: once the run reaches
// a terminal status, Events carries what the agent has not delivered before.
type ImportedRun struct {
	ID             string
	Status         RunStatus
	AlreadyRunning bool
	Error          *string
	Events         []Event
}
