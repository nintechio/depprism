package depprism

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
)

// Policy controls which evidence should fail an automated review.
type Policy struct {
	Schema          string   `json:"$schema,omitempty"`
	FailOn          []string `json:"fail_on"`
	MaxAdded        int      `json:"max_added"`
	DenyNewGit      bool     `json:"deny_new_git_sources"`
	DenyNewBuild    bool     `json:"deny_new_build_behavior"`
	AllowedSources  []string `json:"allowed_source_prefixes"`
	AllowedPackages []string `json:"allowed_packages"`
}

// DefaultPolicy fails only on same-version source or integrity drift. New Git
// and build-capable dependencies remain visible findings unless enabled.
func DefaultPolicy() Policy {
	return Policy{
		FailOn: []string{"integrity-drift", "source-drift"},
	}
}

// DecodePolicy strictly decodes one JSON policy.
func DecodePolicy(data []byte) (Policy, error) {
	policy := DefaultPolicy()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Policy{}, errors.New("trailing JSON content")
	}
	if err := policy.Validate(); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

// Validate checks policy values and known finding codes.
func (policy Policy) Validate() error {
	if policy.MaxAdded < 0 {
		return errors.New("max_added cannot be negative")
	}
	known := []string{"build-enabled", "integrity-drift", "new-build-package", "new-git-source", "source-drift"}
	for _, code := range policy.FailOn {
		if !slices.Contains(known, code) {
			return fmt.Errorf("unknown fail_on code %q", code)
		}
	}
	if hasDuplicates(policy.FailOn) || hasDuplicates(policy.AllowedSources) || hasDuplicates(policy.AllowedPackages) {
		return errors.New("policy arrays cannot contain duplicate values")
	}
	for _, prefix := range policy.AllowedSources {
		if prefix == "" {
			return errors.New("allowed_source_prefixes cannot contain an empty value")
		}
	}
	for _, name := range policy.AllowedPackages {
		if name == "" {
			return errors.New("allowed_packages cannot contain an empty value")
		}
	}
	return nil
}

func hasDuplicates(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
}

func applyPolicy(change *Change, policy Policy, report *Report) {
	if change.After != nil && slices.Contains(policy.AllowedPackages, change.After.Name) {
		return
	}
	failed := false
	for _, finding := range change.Findings {
		if slices.Contains(policy.FailOn, finding.Code) ||
			(policy.DenyNewGit && finding.Code == "new-git-source") ||
			(policy.DenyNewBuild && (finding.Code == "build-enabled" || finding.Code == "new-build-package")) {
			failed = true
		}
	}
	if change.After != nil && len(policy.AllowedSources) > 0 && !hasAllowedPrefix(change.After.Source, policy.AllowedSources) {
		change.Findings = append(change.Findings, Finding{Code: "untrusted-source", Severity: SeverityHigh, Message: "dependency source is outside the policy allowlist"})
		failed = true
	}
	if failed {
		report.Passed = false
		report.Summary.PolicyFails++
	}
}

func hasAllowedPrefix(source string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if len(source) >= len(prefix) && source[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
