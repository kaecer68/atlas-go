package experiment

import "fmt"

func CanTransitionExperimentStatus(from, to ExperimentStatus) bool {
	if from == to {
		return true
	}

	switch from {
	case "":
		return to == ExperimentPlanned || to == ExperimentRunning
	case ExperimentPlanned:
		return to == ExperimentRunning || to == ExperimentRejected || to == ExperimentExpired
	case ExperimentRunning:
		return to == ExperimentAccepted || to == ExperimentRejected || to == ExperimentExpired
	case ExperimentAccepted:
		return false
	case ExperimentRejected:
		return false
	case ExperimentExpired:
		return false
	default:
		return false
	}
}

func TransitionExperimentStatus(record *ExperimentRecord, next ExperimentStatus) error {
	if record == nil {
		return fmt.Errorf("experiment record is nil")
	}
	if !CanTransitionExperimentStatus(record.Status, next) {
		return fmt.Errorf("invalid experiment status transition: %q -> %q", record.Status, next)
	}
	record.Status = next
	return nil
}
