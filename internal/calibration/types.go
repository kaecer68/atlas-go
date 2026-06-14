package calibration

// CalibratedParameter holds before/after values with calibration metadata.
type CalibratedParameter struct {
	Path             string
	Before           float64
	After            float64
	Method           string
	Confidence       float64
	Significant      bool
	SampleSize       int
	CalibrationNotes string
}

// CalibrationResult holds the outcome of calibrating one module.
type CalibrationResult struct {
	Module     string
	Parameters []CalibratedParameter
	Errors     []string
}
