package depprism

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type uvParser struct{}

func (uvParser) Names() []string { return []string{"uv.lock"} }

type uvLock struct {
	Version  int         `toml:"version"`
	Revision int         `toml:"revision"`
	Packages []uvPackage `toml:"package"`
}

type uvPackage struct {
	Name         string         `toml:"name"`
	Version      string         `toml:"version"`
	Source       uvSource       `toml:"source"`
	Dependencies []uvDependency `toml:"dependencies"`
	SDist        uvArtifact     `toml:"sdist"`
	Wheels       []uvArtifact   `toml:"wheels"`
}

type uvSource struct {
	Registry string `toml:"registry"`
	Git      string `toml:"git"`
	URL      string `toml:"url"`
	Path     string `toml:"path"`
	Editable string `toml:"editable"`
	Virtual  string `toml:"virtual"`
}

type uvDependency struct {
	Name    string   `toml:"name"`
	Version string   `toml:"version"`
	Source  uvSource `toml:"source"`
}

type uvArtifact struct {
	URL  string `toml:"url"`
	Hash string `toml:"hash"`
}

func (uvParser) Parse(lockPath string, data []byte, options ParseOptions) (*Snapshot, error) {
	var lock uvLock
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	if lock.Version < 1 {
		return nil, fmt.Errorf("unsupported uv lock version %d", lock.Version)
	}
	snapshot := &Snapshot{
		Ecosystem: "uv",
		Path:      lockPath,
		Format:    fmt.Sprintf("uv-lock/v%d", lock.Version),
		Packages:  map[string]Package{},
		Warnings: []Warning{{
			Code:    "build-evidence-unavailable",
			Message: "uv.lock records artifacts but does not prove whether installation will execute build code on the target platform",
		}},
	}
	coordinateToIDs := map[string][]string{}
	pending := map[string][]uvDependency{}
	for _, record := range lock.Packages {
		name := normalizePythonName(record.Name)
		if name == "" {
			continue
		}
		metadata := map[string]string{}
		if record.SDist.URL != "" || record.SDist.Hash != "" {
			metadata["sdist"] = "true"
		}
		if len(record.Wheels) > 0 {
			metadata["wheels"] = fmt.Sprintf("%d", len(record.Wheels))
		}
		id := addPackage(snapshot, Package{
			Name:      name,
			Version:   record.Version,
			Source:    uvSourceString(record.Source),
			Integrity: uvIntegrity(record),
			Scope:     ScopeUnknown,
			Metadata:  metadata,
		})
		coordinate := pythonCoordinate(name, record.Version)
		coordinateToIDs[coordinate] = append(coordinateToIDs[coordinate], id)
		pending[id] = record.Dependencies
	}
	for coordinate := range coordinateToIDs {
		slices.Sort(coordinateToIDs[coordinate])
	}
	for id, dependencies := range pending {
		pkg := snapshot.Packages[id]
		for _, dependency := range dependencies {
			name := normalizePythonName(dependency.Name)
			ids := coordinateToIDs[pythonCoordinate(name, dependency.Version)]
			if len(ids) == 0 {
				ids = pythonIDsByName(coordinateToIDs, name)
			}
			if len(ids) > 0 {
				pkg.Dependencies = append(pkg.Dependencies, ids[0])
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
	markPythonDirect(snapshot, manifest, coordinateToIDs)
	return snapshot, nil
}

func uvSourceString(source uvSource) string {
	switch {
	case source.Git != "":
		return source.Git
	case source.URL != "":
		return source.URL
	case source.Editable != "":
		return "file:" + source.Editable
	case source.Path != "":
		return "file:" + source.Path
	case source.Virtual != "":
		return "workspace:" + source.Virtual
	case source.Registry != "":
		return source.Registry
	default:
		return "registry:pypi"
	}
}

func uvIntegrity(record uvPackage) string {
	var hashes []string
	if record.SDist.Hash != "" {
		hashes = append(hashes, record.SDist.Hash)
	}
	for _, wheel := range record.Wheels {
		if wheel.Hash != "" {
			hashes = append(hashes, wheel.Hash)
		}
	}
	slices.Sort(hashes)
	return strings.Join(slices.Compact(hashes), ",")
}

type pythonManifestDependency struct {
	Name  string
	Scope Scope
}

type pythonManifest struct {
	Project struct {
		Dependencies         []string            `toml:"dependencies"`
		OptionalDependencies map[string][]string `toml:"optional-dependencies"`
	} `toml:"project"`
	DependencyGroups map[string][]any `toml:"dependency-groups"`
	Tool             struct {
		UV struct {
			DevDependencies []string `toml:"dev-dependencies"`
		} `toml:"uv"`
		Poetry struct {
			Dependencies map[string]any `toml:"dependencies"`
			Group        map[string]struct {
				Dependencies map[string]any `toml:"dependencies"`
			} `toml:"group"`
			DevDependencies map[string]any `toml:"dev-dependencies"`
		} `toml:"poetry"`
	} `toml:"tool"`
}

func readPythonManifest(options ParseOptions, primaryPath string) ([]pythonManifestDependency, string, error) {
	data, path, err := readCompanion(options, primaryPath, "pyproject.toml")
	if err != nil {
		return nil, path, err
	}
	var manifest pythonManifest
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return nil, path, err
	}
	byName := map[string]Scope{}
	add := func(requirement string, scope Scope) {
		name := pythonRequirementName(requirement)
		if name == "" || name == "python" {
			return
		}
		byName[name] = strongerScope(byName[name], scope)
	}
	for _, requirement := range manifest.Project.Dependencies {
		add(requirement, ScopeProduction)
	}
	for _, requirements := range manifest.Project.OptionalDependencies {
		for _, requirement := range requirements {
			add(requirement, ScopeOptional)
		}
	}
	for _, requirement := range manifest.Tool.UV.DevDependencies {
		add(requirement, ScopeDevelopment)
	}
	for _, requirements := range manifest.DependencyGroups {
		for _, raw := range requirements {
			if requirement, ok := raw.(string); ok {
				add(requirement, ScopeDevelopment)
			}
		}
	}
	for name := range manifest.Tool.Poetry.Dependencies {
		add(name, ScopeProduction)
	}
	for name := range manifest.Tool.Poetry.DevDependencies {
		add(name, ScopeDevelopment)
	}
	for _, group := range manifest.Tool.Poetry.Group {
		for name := range group.Dependencies {
			add(name, ScopeDevelopment)
		}
	}
	result := make([]pythonManifestDependency, 0, len(byName))
	for name, scope := range byName {
		result = append(result, pythonManifestDependency{Name: name, Scope: scope})
	}
	slices.SortFunc(result, func(left, right pythonManifestDependency) int { return strings.Compare(left.Name, right.Name) })
	return result, path, nil
}

func markPythonDirect(snapshot *Snapshot, manifest []pythonManifestDependency, index map[string][]string) {
	for _, dependency := range manifest {
		ids := pythonIDsByName(index, dependency.Name)
		if len(ids) == 0 {
			continue
		}
		id := ids[0]
		pkg := snapshot.Packages[id]
		pkg.Direct = true
		pkg.Scope = strongerScope(pkg.Scope, dependency.Scope)
		snapshot.Packages[id] = pkg
		snapshot.Roots = append(snapshot.Roots, id)
	}
	slices.Sort(snapshot.Roots)
	snapshot.Roots = slices.Compact(snapshot.Roots)
}

var pythonNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*`)

func pythonRequirementName(requirement string) string {
	return normalizePythonName(pythonNamePattern.FindString(strings.TrimSpace(requirement)))
}

func normalizePythonName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.NewReplacer("_", "-", ".", "-").Replace(name)
}

func pythonCoordinate(name, version string) string {
	return normalizePythonName(name) + "\x00" + version
}

func pythonIDsByName(index map[string][]string, name string) []string {
	var ids []string
	prefix := normalizePythonName(name) + "\x00"
	for coordinate, candidates := range index {
		if strings.HasPrefix(coordinate, prefix) {
			ids = append(ids, candidates...)
		}
	}
	slices.Sort(ids)
	return ids
}
