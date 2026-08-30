package depprism

import (
	"slices"
	"testing"
)

func TestCompareFindsIntegrityAndBuildDrift(t *testing.T) {
	before := testSnapshot(Package{Name: "alpha", Version: "1.0.0", Source: "https://registry.example/alpha", Integrity: "sha256:old", Scope: ScopeProduction, Direct: true})
	after := testSnapshot(Package{Name: "alpha", Version: "1.0.0", Source: "https://registry.example/alpha", Integrity: "sha256:new", Scope: ScopeProduction, Direct: true, Build: true})
	report, err := Compare(before, after, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || report.Summary.PolicyFails != 1 || report.Summary.HighRisk != 1 {
		t.Fatalf("report summary = %+v passed=%t", report.Summary, report.Passed)
	}
	codes := findingCodes(report.Changes[0].Findings)
	if !slices.Equal(codes, []string{"build-enabled", "integrity-drift"}) {
		t.Fatalf("finding codes = %v", codes)
	}
}

func TestCompareMaxAddedAfterFullCount(t *testing.T) {
	before := testSnapshot()
	after := testSnapshot(
		Package{Name: "alpha", Version: "1", Scope: ScopeProduction},
		Package{Name: "beta", Version: "1", Scope: ScopeProduction},
	)
	report, err := Compare(before, after, Policy{MaxAdded: 1})
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || report.Summary.Added != 2 || report.Summary.PolicyFails != 1 || len(report.PolicyFindings) != 1 {
		t.Fatalf("max-added result = %+v findings=%+v", report.Summary, report.PolicyFindings)
	}
}

func TestCompareCausalPath(t *testing.T) {
	after := testSnapshot(
		Package{Name: "root", Version: "1", Scope: ScopeProduction, Direct: true},
		Package{Name: "middle", Version: "1", Scope: ScopeProduction},
		Package{Name: "leaf", Version: "1", Scope: ScopeProduction},
	)
	root := packageNamed(t, after, "root")
	middle := packageNamed(t, after, "middle")
	leaf := packageNamed(t, after, "leaf")
	root.Dependencies = []string{middle.ID}
	middle.Dependencies = []string{leaf.ID}
	after.Packages[root.ID] = root
	after.Packages[middle.ID] = middle
	after.Roots = []string{root.ID}
	report, err := Compare(EmptySnapshot(after), after, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range report.Changes {
		if change.Name == "leaf" && !slices.Equal(change.Path, []string{"root", "middle", "leaf"}) {
			t.Fatalf("leaf path = %v", change.Path)
		}
	}
}

func TestPolicyValidation(t *testing.T) {
	for _, input := range []string{
		`{"fail_on":["unknown"]}`,
		`{"allowed_source_prefixes":[""]}`,
		`{"max_added":-1}`,
		`{"unexpected":true}`,
	} {
		if _, err := DecodePolicy([]byte(input)); err == nil {
			t.Fatalf("DecodePolicy(%s) unexpectedly succeeded", input)
		}
	}
}

func testSnapshot(packages ...Package) *Snapshot {
	snapshot := &Snapshot{Ecosystem: "test", Path: "test.lock", Format: "test/v1", Packages: map[string]Package{}}
	for _, pkg := range packages {
		id := addPackage(snapshot, pkg)
		if pkg.Direct {
			snapshot.Roots = append(snapshot.Roots, id)
		}
	}
	slices.Sort(snapshot.Roots)
	return snapshot
}

func findingCodes(findings []Finding) []string {
	result := make([]string, 0, len(findings))
	for _, finding := range findings {
		result = append(result, finding.Code)
	}
	return result
}
