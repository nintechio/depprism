package depprism

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type cargoParser struct{}

func (cargoParser) Names() []string { return []string{"Cargo.lock"} }

type cargoLock struct {
	Version  int            `toml:"version"`
	Packages []cargoPackage `toml:"package"`
}

type cargoPackage struct {
	Name         string   `toml:"name"`
	Version      string   `toml:"version"`
	Source       string   `toml:"source"`
	Checksum     string   `toml:"checksum"`
	Dependencies []string `toml:"dependencies"`
}

func (cargoParser) Parse(lockPath string, data []byte, options ParseOptions) (*Snapshot, error) {
	var lock cargoLock
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	if len(lock.Packages) == 0 {
		return nil, fmt.Errorf("no Cargo package records found")
	}
	if lock.Version == 0 {
		lock.Version = 1
	}
	snapshot := &Snapshot{
		Ecosystem: "cargo",
		Path:      lockPath,
		Format:    fmt.Sprintf("cargo-lock/v%d", lock.Version),
		Packages:  map[string]Package{},
		Warnings: []Warning{{
			Code:    "build-evidence-unavailable",
			Message: "Cargo.lock does not record which crates contain build scripts",
		}},
	}
	coordinateToIDs := map[string][]string{}
	pending := map[string][]string{}
	for _, record := range lock.Packages {
		source := strings.TrimPrefix(record.Source, "registry+")
		if source == "" {
			source = "workspace:cargo"
		}
		id := addPackage(snapshot, Package{
			Name:      record.Name,
			Version:   record.Version,
			Source:    source,
			Integrity: record.Checksum,
			Scope:     ScopeUnknown,
		})
		coordinateToIDs[cargoCoordinate(record.Name, record.Version, record.Source)] = append(coordinateToIDs[cargoCoordinate(record.Name, record.Version, record.Source)], id)
		pending[id] = record.Dependencies
	}
	for coordinate := range coordinateToIDs {
		slices.Sort(coordinateToIDs[coordinate])
	}
	for id, dependencies := range pending {
		pkg := snapshot.Packages[id]
		for _, dependency := range dependencies {
			name, version, source := parseCargoDependency(dependency)
			ids := coordinateToIDs[cargoCoordinate(name, version, source)]
			if len(ids) == 0 {
				ids = cargoIDsByName(coordinateToIDs, name)
			}
			if len(ids) > 0 {
				pkg.Dependencies = append(pkg.Dependencies, ids[0])
			}
		}
		slices.Sort(pkg.Dependencies)
		pkg.Dependencies = slices.Compact(pkg.Dependencies)
		snapshot.Packages[id] = pkg
	}
	manifest, manifestPath, err := readCargoManifest(options, lockPath)
	if err != nil && !companionUnavailable(snapshot, err, manifestPath) {
		return nil, fmt.Errorf("parse %s: %w", manifestPath, err)
	}
	snapshot.ManifestPath = manifestPath
	for name, scope := range manifest {
		ids := cargoIDsByName(coordinateToIDs, name)
		if len(ids) == 0 {
			continue
		}
		id := ids[0]
		pkg := snapshot.Packages[id]
		pkg.Direct = true
		pkg.Scope = strongerScope(pkg.Scope, scope)
		snapshot.Packages[id] = pkg
		snapshot.Roots = append(snapshot.Roots, id)
	}
	slices.Sort(snapshot.Roots)
	snapshot.Roots = slices.Compact(snapshot.Roots)
	return snapshot, nil
}

func cargoCoordinate(name, version, source string) string {
	return name + "\x00" + version + "\x00" + source
}

func parseCargoDependency(value string) (name, version, source string) {
	fields := strings.Fields(value)
	if len(fields) > 0 {
		name = fields[0]
	}
	if len(fields) > 1 {
		version = fields[1]
	}
	if len(fields) > 2 {
		source = strings.Trim(strings.Join(fields[2:], " "), "()")
	}
	return name, version, source
}

func cargoIDsByName(index map[string][]string, name string) []string {
	var ids []string
	prefix := name + "\x00"
	for coordinate, candidates := range index {
		if strings.HasPrefix(coordinate, prefix) {
			ids = append(ids, candidates...)
		}
	}
	slices.Sort(ids)
	return ids
}

type cargoManifest struct {
	Dependencies      map[string]any `toml:"dependencies"`
	DevDependencies   map[string]any `toml:"dev-dependencies"`
	BuildDependencies map[string]any `toml:"build-dependencies"`
	Target            map[string]struct {
		Dependencies      map[string]any `toml:"dependencies"`
		DevDependencies   map[string]any `toml:"dev-dependencies"`
		BuildDependencies map[string]any `toml:"build-dependencies"`
	} `toml:"target"`
	Workspace struct {
		Dependencies map[string]any `toml:"dependencies"`
	} `toml:"workspace"`
}

func readCargoManifest(options ParseOptions, primaryPath string) (map[string]Scope, string, error) {
	data, path, err := readCompanion(options, primaryPath, "Cargo.toml")
	if err != nil {
		return nil, path, err
	}
	var manifest cargoManifest
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return nil, path, err
	}
	result := map[string]Scope{}
	add := func(values map[string]any, scope Scope) {
		for alias, raw := range values {
			name := alias
			if object, ok := raw.(map[string]any); ok {
				if packageName, ok := object["package"].(string); ok && packageName != "" {
					name = packageName
				}
			}
			result[name] = strongerScope(result[name], scope)
		}
	}
	add(manifest.DevDependencies, ScopeDevelopment)
	add(manifest.BuildDependencies, ScopeProduction)
	add(manifest.Dependencies, ScopeProduction)
	add(manifest.Workspace.Dependencies, ScopeProduction)
	for _, target := range manifest.Target {
		add(target.DevDependencies, ScopeDevelopment)
		add(target.BuildDependencies, ScopeProduction)
		add(target.Dependencies, ScopeProduction)
	}
	return result, path, nil
}
