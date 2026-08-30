package depprism

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// ChangeKind is the structural relationship between package records.
type ChangeKind string

const (
	ChangeAdded    ChangeKind = "added"
	ChangeRemoved  ChangeKind = "removed"
	ChangeUpdated  ChangeKind = "updated"
	ChangeMetadata ChangeKind = "metadata"
)

// Severity communicates review priority without claiming a package is
// vulnerable or malicious.
type Severity string

const (
	SeverityInfo   Severity = "info"
	SeverityLow    Severity = "low"
	SeverityMedium Severity = "medium"
	SeverityHigh   Severity = "high"
)

// Change describes one normalized package transition.
type Change struct {
	Kind     ChangeKind `json:"kind"`
	Name     string     `json:"name"`
	Before   *Package   `json:"before,omitempty"`
	After    *Package   `json:"after,omitempty"`
	Path     []string   `json:"path,omitempty"`
	Findings []Finding  `json:"findings,omitempty"`
}

// Finding identifies evidence that deserves review. Findings never claim that
// a dependency is malicious.
type Finding struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

// Report is the stable machine-readable result of comparing two snapshots.
type Report struct {
	SchemaVersion  int       `json:"schema_version"`
	Ecosystem      string    `json:"ecosystem"`
	Path           string    `json:"path"`
	BeforeFormat   string    `json:"before_format"`
	AfterFormat    string    `json:"after_format"`
	Changes        []Change  `json:"changes"`
	Warnings       []Warning `json:"warnings"`
	PolicyFindings []Finding `json:"policy_findings,omitempty"`
	Summary        Summary   `json:"summary"`
	Passed         bool      `json:"passed"`
}

// Summary contains deterministic aggregate counts.
type Summary struct {
	Added       int `json:"added"`
	Removed     int `json:"removed"`
	Updated     int `json:"updated"`
	Metadata    int `json:"metadata"`
	Direct      int `json:"direct"`
	HighRisk    int `json:"high_risk"`
	PolicyFails int `json:"policy_failures"`
}

// Compare computes a deterministic semantic diff between normalized graphs.
func Compare(before, after *Snapshot, policy Policy) (Report, error) {
	if err := before.Validate(); err != nil {
		return Report{}, fmt.Errorf("before snapshot: %w", err)
	}
	if err := after.Validate(); err != nil {
		return Report{}, fmt.Errorf("after snapshot: %w", err)
	}
	if before.Ecosystem != after.Ecosystem {
		return Report{}, fmt.Errorf("ecosystem changed from %s to %s", before.Ecosystem, after.Ecosystem)
	}

	report := Report{
		SchemaVersion: 1,
		Ecosystem:     after.Ecosystem,
		Path:          after.Path,
		BeforeFormat:  before.Format,
		AfterFormat:   after.Format,
		Warnings:      append(slices.Clone(before.Warnings), after.Warnings...),
		Changes:       []Change{},
		Passed:        true,
	}
	slices.SortFunc(report.Warnings, func(left, right Warning) int {
		if result := strings.Compare(left.Code, right.Code); result != 0 {
			return result
		}
		return strings.Compare(left.Message, right.Message)
	})
	report.Warnings = slices.Compact(report.Warnings)

	beforeByName := groupPackages(before)
	afterByName := groupPackages(after)
	names := make([]string, 0, len(beforeByName)+len(afterByName))
	for name := range beforeByName {
		names = append(names, name)
	}
	for name := range afterByName {
		if _, ok := beforeByName[name]; !ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)

	for _, name := range names {
		changes := comparePackageGroup(name, beforeByName[name], afterByName[name], after)
		report.Changes = append(report.Changes, changes...)
	}
	for index := range report.Changes {
		applyPolicy(&report.Changes[index], policy, &report)
		change := report.Changes[index]
		switch change.Kind {
		case ChangeAdded:
			report.Summary.Added++
		case ChangeRemoved:
			report.Summary.Removed++
		case ChangeUpdated:
			report.Summary.Updated++
		case ChangeMetadata:
			report.Summary.Metadata++
		}
		if (change.After != nil && change.After.Direct) || (change.Before != nil && change.Before.Direct) {
			report.Summary.Direct++
		}
		if hasSeverity(change.Findings, SeverityHigh) {
			report.Summary.HighRisk++
		}
	}
	if policy.MaxAdded > 0 && report.Summary.Added > policy.MaxAdded {
		report.Passed = false
		report.Summary.PolicyFails++
		report.PolicyFindings = append(report.PolicyFindings, Finding{
			Code:     "max-added-exceeded",
			Severity: SeverityHigh,
			Message:  fmt.Sprintf("%d dependencies were added; policy allows at most %d", report.Summary.Added, policy.MaxAdded),
		})
	}
	return report, nil
}

func groupPackages(snapshot *Snapshot) map[string][]Package {
	result := map[string][]Package{}
	for _, pkg := range snapshot.Packages {
		result[pkg.Name] = append(result[pkg.Name], pkg)
	}
	for name := range result {
		slices.SortFunc(result[name], comparePackage)
	}
	return result
}

func comparePackage(left, right Package) int {
	return cmp.Or(
		strings.Compare(left.Version, right.Version),
		strings.Compare(left.Source, right.Source),
		strings.Compare(left.Integrity, right.Integrity),
	)
}

func comparePackageGroup(name string, before, after []Package, graph *Snapshot) []Change {
	matchedBefore := make([]bool, len(before))
	matchedAfter := make([]bool, len(after))
	var changes []Change

	for oldIndex := range before {
		for newIndex := range after {
			if matchedAfter[newIndex] {
				continue
			}
			if before[oldIndex].Version == after[newIndex].Version && before[oldIndex].Source == after[newIndex].Source {
				matchedBefore[oldIndex] = true
				matchedAfter[newIndex] = true
				if !packageMetadataEqual(before[oldIndex], after[newIndex]) {
					oldPackage, newPackage := before[oldIndex], after[newIndex]
					change := Change{Kind: ChangeMetadata, Name: name, Before: &oldPackage, After: &newPackage}
					change.Findings = findingsFor(change)
					change.Path = shortestPath(graph, newPackage.ID)
					changes = append(changes, change)
				}
				break
			}
		}
	}

	var remainingBefore, remainingAfter []Package
	for index, pkg := range before {
		if !matchedBefore[index] {
			remainingBefore = append(remainingBefore, pkg)
		}
	}
	for index, pkg := range after {
		if !matchedAfter[index] {
			remainingAfter = append(remainingAfter, pkg)
		}
	}
	pairs := min(len(remainingBefore), len(remainingAfter))
	for index := 0; index < pairs; index++ {
		oldPackage, newPackage := remainingBefore[index], remainingAfter[index]
		change := Change{Kind: ChangeUpdated, Name: name, Before: &oldPackage, After: &newPackage}
		change.Findings = findingsFor(change)
		change.Path = shortestPath(graph, newPackage.ID)
		changes = append(changes, change)
	}
	for _, pkg := range remainingBefore[pairs:] {
		oldPackage := pkg
		change := Change{Kind: ChangeRemoved, Name: name, Before: &oldPackage}
		change.Findings = findingsFor(change)
		changes = append(changes, change)
	}
	for _, pkg := range remainingAfter[pairs:] {
		newPackage := pkg
		change := Change{Kind: ChangeAdded, Name: name, After: &newPackage}
		change.Findings = findingsFor(change)
		change.Path = shortestPath(graph, newPackage.ID)
		changes = append(changes, change)
	}
	return changes
}

func packageMetadataEqual(left, right Package) bool {
	return left.Integrity == right.Integrity &&
		left.Scope == right.Scope &&
		left.Direct == right.Direct &&
		left.Build == right.Build &&
		slices.Equal(left.Dependencies, right.Dependencies)
}

func findingsFor(change Change) []Finding {
	var findings []Finding
	if change.After != nil && sourceKind(change.After.Source) == "git" &&
		(change.Before == nil || sourceKind(change.Before.Source) != "git" || change.Before.Source != change.After.Source) {
		findings = append(findings, Finding{Code: "new-git-source", Severity: SeverityMedium, Message: "dependency now resolves from a Git source"})
	}
	if change.Before != nil && change.After != nil {
		if change.Before.Version == change.After.Version && change.Before.Source != change.After.Source {
			findings = append(findings, Finding{Code: "source-drift", Severity: SeverityHigh, Message: "source changed without a version change"})
		}
		if change.Before.Version == change.After.Version && change.Before.Integrity != change.After.Integrity && (change.Before.Integrity != "" || change.After.Integrity != "") {
			findings = append(findings, Finding{Code: "integrity-drift", Severity: SeverityHigh, Message: "integrity evidence changed without a version change"})
		}
		if !change.Before.Build && change.After.Build {
			findings = append(findings, Finding{Code: "build-enabled", Severity: SeverityHigh, Message: "install-time or build behavior is newly enabled"})
		}
	}
	if change.Before == nil && change.After != nil && change.After.Build {
		findings = append(findings, Finding{Code: "new-build-package", Severity: SeverityMedium, Message: "new dependency declares install-time or build behavior"})
	}
	slices.SortFunc(findings, func(left, right Finding) int { return strings.Compare(left.Code, right.Code) })
	return findings
}

func shortestPath(snapshot *Snapshot, target string) []string {
	if slices.Contains(snapshot.Roots, target) {
		return []string{snapshot.Packages[target].Name}
	}
	type node struct {
		id   string
		path []string
	}
	queue := make([]node, 0, len(snapshot.Roots))
	seen := map[string]bool{}
	for _, root := range snapshot.Roots {
		pkg := snapshot.Packages[root]
		queue = append(queue, node{id: root, path: []string{pkg.Name}})
		seen[root] = true
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		pkg := snapshot.Packages[current.id]
		for _, dependency := range pkg.Dependencies {
			if seen[dependency] {
				continue
			}
			seen[dependency] = true
			next := append(slices.Clone(current.path), snapshot.Packages[dependency].Name)
			if dependency == target {
				return next
			}
			queue = append(queue, node{id: dependency, path: next})
		}
	}
	if pkg, ok := snapshot.Packages[target]; ok {
		return []string{pkg.Name}
	}
	return nil
}

func hasSeverity(findings []Finding, severity Severity) bool {
	return slices.ContainsFunc(findings, func(finding Finding) bool { return finding.Severity == severity })
}
