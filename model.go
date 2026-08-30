package depprism

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// Scope describes how a package participates in a dependency graph.
type Scope string

const (
	ScopeUnknown     Scope = "unknown"
	ScopeProduction  Scope = "production"
	ScopeDevelopment Scope = "development"
	ScopeOptional    Scope = "optional"
)

// Package is DepPrism's package-manager-neutral dependency record.
type Package struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Version      string            `json:"version,omitempty"`
	Source       string            `json:"source,omitempty"`
	Integrity    string            `json:"integrity,omitempty"`
	Scope        Scope             `json:"scope"`
	Direct       bool              `json:"direct"`
	Build        bool              `json:"build"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Snapshot is a normalized dependency graph parsed from one lockfile family.
type Snapshot struct {
	Ecosystem    string             `json:"ecosystem"`
	Path         string             `json:"path"`
	Format       string             `json:"format"`
	Packages     map[string]Package `json:"packages"`
	Roots        []string           `json:"roots,omitempty"`
	Warnings     []Warning          `json:"warnings,omitempty"`
	ManifestPath string             `json:"manifest_path,omitempty"`
}

// Warning records a limitation in source evidence without silently guessing.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Validate checks the normalized graph invariants expected by comparison and
// rendering code.
func (s *Snapshot) Validate() error {
	if s == nil {
		return fmt.Errorf("snapshot is nil")
	}
	if strings.TrimSpace(s.Ecosystem) == "" {
		return fmt.Errorf("snapshot ecosystem is empty")
	}
	if strings.TrimSpace(s.Path) == "" {
		return fmt.Errorf("snapshot path is empty")
	}
	if s.Packages == nil {
		return fmt.Errorf("snapshot packages are nil")
	}
	for id, pkg := range s.Packages {
		if id == "" || pkg.ID != id {
			return fmt.Errorf("package map key %q does not match package ID %q", id, pkg.ID)
		}
		if strings.TrimSpace(pkg.Name) == "" {
			return fmt.Errorf("package %q has an empty name", id)
		}
		for _, dependency := range pkg.Dependencies {
			if _, ok := s.Packages[dependency]; !ok {
				return fmt.Errorf("package %q references missing dependency %q", id, dependency)
			}
		}
	}
	for _, root := range s.Roots {
		if _, ok := s.Packages[root]; !ok {
			return fmt.Errorf("root references missing package %q", root)
		}
	}
	return nil
}

func packageID(ecosystem, name, version, source string) string {
	canonical := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(ecosystem)),
		strings.TrimSpace(name),
		strings.TrimSpace(version),
		normalizeSource(source),
	}, "\x00")
	digest := sha256.Sum256([]byte(canonical))
	return strings.TrimSpace(name) + "@" + strings.TrimSpace(version) + "#" + hex.EncodeToString(digest[:6])
}

func normalizePackage(pkg Package, ecosystem string) Package {
	pkg.Name = strings.TrimSpace(pkg.Name)
	pkg.Version = strings.TrimSpace(pkg.Version)
	pkg.Source = normalizeSource(pkg.Source)
	pkg.Integrity = strings.TrimSpace(pkg.Integrity)
	if pkg.Scope == "" {
		pkg.Scope = ScopeUnknown
	}
	if pkg.Metadata == nil {
		pkg.Metadata = map[string]string{}
	}
	slices.Sort(pkg.Dependencies)
	pkg.Dependencies = slices.Compact(pkg.Dependencies)
	if pkg.ID == "" {
		pkg.ID = packageID(ecosystem, pkg.Name, pkg.Version, pkg.Source)
	}
	return pkg
}

func normalizeSource(source string) string {
	source = strings.TrimSpace(source)
	source = strings.TrimSuffix(source, "/")
	return source
}

func addPackage(snapshot *Snapshot, pkg Package) string {
	pkg = normalizePackage(pkg, snapshot.Ecosystem)
	if existing, ok := snapshot.Packages[pkg.ID]; ok {
		existing.Direct = existing.Direct || pkg.Direct
		existing.Build = existing.Build || pkg.Build
		existing.Scope = strongerScope(existing.Scope, pkg.Scope)
		existing.Dependencies = append(existing.Dependencies, pkg.Dependencies...)
		slices.Sort(existing.Dependencies)
		existing.Dependencies = slices.Compact(existing.Dependencies)
		for key, value := range pkg.Metadata {
			existing.Metadata[key] = value
		}
		snapshot.Packages[pkg.ID] = existing
		return pkg.ID
	}
	snapshot.Packages[pkg.ID] = pkg
	return pkg.ID
}

func strongerScope(left, right Scope) Scope {
	rank := map[Scope]int{
		ScopeUnknown:     0,
		ScopeDevelopment: 1,
		ScopeOptional:    2,
		ScopeProduction:  3,
	}
	if rank[right] > rank[left] {
		return right
	}
	return left
}
