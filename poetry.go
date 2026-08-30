package depprism

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type poetryParser struct{}

func (poetryParser) Names() []string { return []string{"poetry.lock"} }

type poetryLock struct {
	Packages []poetryPackage `toml:"package"`
	Metadata struct {
		LockVersion string `toml:"lock-version"`
		ContentHash string `toml:"content-hash"`
	} `toml:"metadata"`
}

type poetryPackage struct {
	Name         string         `toml:"name"`
	Version      string         `toml:"version"`
	Category     string         `toml:"category"`
	Optional     bool           `toml:"optional"`
	Groups       []string       `toml:"groups"`
	Dependencies map[string]any `toml:"dependencies"`
	Files        []poetryFile   `toml:"files"`
	Source       poetrySource   `toml:"source"`
}

type poetryFile struct {
	File string `toml:"file"`
	Hash string `toml:"hash"`
}

type poetrySource struct {
	Type              string `toml:"type"`
	URL               string `toml:"url"`
	Reference         string `toml:"reference"`
	ResolvedReference string `toml:"resolved_reference"`
}

func (poetryParser) Parse(lockPath string, data []byte, options ParseOptions) (*Snapshot, error) {
	var lock poetryLock
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	if len(lock.Packages) == 0 {
		return nil, fmt.Errorf("no Poetry package records found")
	}
	format := "poetry-lock"
	if lock.Metadata.LockVersion != "" {
		format += "/v" + lock.Metadata.LockVersion
	}
	snapshot := &Snapshot{
		Ecosystem: "poetry",
		Path:      lockPath,
		Format:    format,
		Packages:  map[string]Package{},
		Warnings: []Warning{{
			Code:    "build-evidence-unavailable",
			Message: "poetry.lock does not prove whether installation will execute build code on the target platform",
		}},
	}
	byName := map[string][]string{}
	pending := map[string]map[string]any{}
	for _, record := range lock.Packages {
		name := normalizePythonName(record.Name)
		scope := poetryScope(record)
		hashes := make([]string, 0, len(record.Files))
		for _, file := range record.Files {
			if file.Hash != "" {
				hashes = append(hashes, file.Hash)
			}
		}
		slices.Sort(hashes)
		id := addPackage(snapshot, Package{
			Name:      name,
			Version:   record.Version,
			Source:    poetrySourceString(record.Source),
			Integrity: strings.Join(slices.Compact(hashes), ","),
			Scope:     scope,
			Metadata:  map[string]string{"artifact_count": fmt.Sprintf("%d", len(record.Files))},
		})
		byName[name] = append(byName[name], id)
		pending[id] = record.Dependencies
	}
	for name := range byName {
		slices.Sort(byName[name])
	}
	for id, dependencies := range pending {
		pkg := snapshot.Packages[id]
		names := make([]string, 0, len(dependencies))
		for name := range dependencies {
			names = append(names, normalizePythonName(name))
		}
		slices.Sort(names)
		for _, name := range names {
			if ids := byName[name]; len(ids) > 0 {
				pkg.Dependencies = append(pkg.Dependencies, ids[0])
				if len(ids) > 1 {
					snapshot.Warnings = append(snapshot.Warnings, Warning{Code: "ambiguous-dependency-edge", Message: "Poetry dependency " + name + " has multiple locked candidates; the deterministic first candidate was used"})
				}
			}
		}
		slices.Sort(pkg.Dependencies)
		pkg.Dependencies = slices.Compact(pkg.Dependencies)
		snapshot.Packages[id] = pkg
	}
	manifest, manifestPath, err := readPythonManifest(options, lockPath)
	if err != nil && !companionUnavailable(snapshot, err, manifestPath) {
		return nil, fmt.Errorf("parse %s: %w", manifestPath, err)
	}
	snapshot.ManifestPath = manifestPath
	index := map[string][]string{}
	for name, ids := range byName {
		index[pythonCoordinate(name, "")] = ids
	}
	markPythonDirect(snapshot, manifest, index)
	slices.SortFunc(snapshot.Warnings, func(left, right Warning) int {
		if result := strings.Compare(left.Code, right.Code); result != 0 {
			return result
		}
		return strings.Compare(left.Message, right.Message)
	})
	snapshot.Warnings = slices.Compact(snapshot.Warnings)
	return snapshot, nil
}

func poetryScope(record poetryPackage) Scope {
	if record.Optional {
		return ScopeOptional
	}
	if record.Category == "dev" || (len(record.Groups) > 0 && !slices.Contains(record.Groups, "main")) {
		return ScopeDevelopment
	}
	return ScopeProduction
}

func poetrySourceString(source poetrySource) string {
	if source.URL == "" {
		return "registry:pypi"
	}
	result := source.URL
	reference := source.ResolvedReference
	if reference == "" {
		reference = source.Reference
	}
	if reference != "" {
		result += "#" + reference
	}
	if source.Type == "directory" || source.Type == "file" {
		return "file:" + result
	}
	return result
}
