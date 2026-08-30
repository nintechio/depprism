package depprism

// Review combines one or more lockfile reports into a repository-level result.
type Review struct {
	SchemaVersion int       `json:"schema_version"`
	Base          string    `json:"base,omitempty"`
	Head          string    `json:"head,omitempty"`
	Reports       []Report  `json:"reports"`
	Summary       Summary   `json:"summary"`
	Passed        bool      `json:"passed"`
	Warnings      []Warning `json:"warnings,omitempty"`
}

// NewReview returns an empty passing review.
func NewReview() Review {
	return Review{SchemaVersion: 1, Reports: []Report{}, Passed: true}
}

// Add includes a report and updates aggregate counts.
func (review *Review) Add(report Report) {
	review.Reports = append(review.Reports, report)
	review.Summary.Added += report.Summary.Added
	review.Summary.Removed += report.Summary.Removed
	review.Summary.Updated += report.Summary.Updated
	review.Summary.Metadata += report.Summary.Metadata
	review.Summary.Direct += report.Summary.Direct
	review.Summary.HighRisk += report.Summary.HighRisk
	review.Summary.PolicyFails += report.Summary.PolicyFails
	if !report.Passed {
		review.Passed = false
	}
}

// EmptySnapshot constructs the absent side of an added or removed lockfile.
func EmptySnapshot(example *Snapshot) *Snapshot {
	if example == nil {
		return nil
	}
	return &Snapshot{
		Ecosystem: example.Ecosystem,
		Path:      example.Path,
		Format:    "absent",
		Packages:  map[string]Package{},
	}
}
