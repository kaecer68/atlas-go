package schemas

import "github.com/kaecer68/atlas-go/internal/llm"

// CodeReviewAnnotationInput carries a diff text, PR URL, and data
// classification for the code review annotation capability.
type CodeReviewAnnotationInput struct {
	DiffText  string       `json:"diff_text"`
	PRURL     string       `json:"pr_url"`
	DataClass llm.DataClass `json:"data_class"`
}

// CodeReviewAnnotationResponse is the output of the code review
// annotation capability. It contains a list of annotations found
// during code review.
type CodeReviewAnnotationResponse struct {
	Annotations []CodeAnnotation `json:"annotations"`
}

// CodeAnnotation describes a single annotation found during code
// review, including its file location, severity, and message.
type CodeAnnotation struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}
