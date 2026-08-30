package depprism

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
)

type npmParser struct{}

func (npmParser) Names() []string {
	return []string{"npm-shrinkwrap.json", "package-lock.json"}
}

type npmLock struct {
	Name            string                  `json:"name"`
	LockfileVersion int                     `json:"lockfileVersion"`
	Packages        map[string]npmPackage   `json:"packages"`
	Dependencies    map[string]npmV1Package `json:"dependencies"`
}

type npmPackage struct {
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Resolved         string            `json:"resolved"`
	Integrity        string            `json:"integrity"`
	Dev              bool              `json:"dev"`
	Optional         bool              `json:"optional"`
	DevOptional      bool              `json:"devOptional"`
	Link             bool              `json:"link"`
	HasInstallScript bool              `json:"hasInstallScript"`
	Dependencies     map[string]string `json:"dependencies"`
	DevDependencies  map[string]string `json:"devDependencies"`
	OptionalDeps     map[string]string `json:"optionalDependencies"`
}

type npmV1Package struct {
	Version          string                  `json:"version"`
	Resolved         string                  `json:"resolved"`
	Integrity        string                  `json:"integrity"`
	Dev              bool                    `json:"dev"`
	Optional         bool                    `json:"optional"`
	HasInstallScript bool                    `json:"hasInstallScript"`
	Requires         map[string]string       `json:"requires"`
	Dependencies     map[string]npmV1Package `json:"dependencies"`
}

func (npmParser) Parse(lockPath string, data []byte, _ ParseOptions) (*Snapshot, error) {
	var lock npmLock
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(&lock); err != nil {
		return nil, err
	}
	if lock.LockfileVersion < 1 || lock.LockfileVersion > 3 {
		return nil, fmt.Errorf("unsupported npm lockfileVersion %d", lock.LockfileVersion)
	}
	snapshot := &Snapshot{
		Ecosystem: "npm",
		Path:      lockPath,
		Format:    fmt.Sprintf("package-lock/v%d", lock.LockfileVersion),
		Packages:  map[string]Package{},
	}
	if lock.LockfileVersion == 1 || len(lock.Packages) == 0 {
		parseNPMV1(snapshot, lock.Dependencies)
		return snapshot, nil
	}
	parseNPMModern(snapshot, lock.Packages)
	return snapshot, nil
}

func parseNPMModern(snapshot *Snapshot, packages map[string]npmPackage) {
	pathToID := map[string]string{}
	dependencyNames := map[string][]string{}
	root := packages[""]
	directScopes := map[string]Scope{}
	for name := range root.Dependencies {
		directScopes[name] = ScopeProduction
	}
	for name := range root.DevDependencies {
		directScopes[name] = ScopeDevelopment
	}
	for name := range root.OptionalDeps {
		directScopes[name] = ScopeOptional
	}

	paths := make([]string, 0, len(packages))
	for packagePath := range packages {
		if packagePath != "" {
			paths = append(paths, packagePath)
		}
	}
	slices.Sort(paths)
	for _, packagePath := range paths {
		record := packages[packagePath]
		name := record.Name
		if name == "" {
			name = npmNameFromPath(packagePath)
		}
		if name == "" {
			snapshot.Warnings = append(snapshot.Warnings, Warning{Code: "npm-path-name", Message: "could not derive a package name from " + packagePath})
			continue
		}
		source := record.Resolved
		if record.Link {
			source = "link:" + packagePath
		}
		scope := ScopeProduction
		switch {
		case record.Optional:
			scope = ScopeOptional
		case record.Dev || record.DevOptional:
			scope = ScopeDevelopment
		}
		_, direct := directScopes[name]
		pkg := Package{
			Name:      name,
			Version:   record.Version,
			Source:    source,
			Integrity: record.Integrity,
			Scope:     scope,
			Direct:    direct && packagePath == path.Join("node_modules", name),
			Build:     record.HasInstallScript,
			Metadata: map[string]string{
				"lock_path": packagePath,
			},
		}
		id := addPackage(snapshot, pkg)
		pathToID[packagePath] = id
		for dependency := range record.Dependencies {
			dependencyNames[packagePath] = append(dependencyNames[packagePath], dependency)
		}
		for dependency := range record.OptionalDeps {
			dependencyNames[packagePath] = append(dependencyNames[packagePath], dependency)
		}
	}

	for packagePath, names := range dependencyNames {
		id, ok := pathToID[packagePath]
		if !ok {
			continue
		}
		pkg := snapshot.Packages[id]
		for _, name := range names {
			if dependencyPath := resolveNPMPath(packagePath, name, pathToID); dependencyPath != "" {
				pkg.Dependencies = append(pkg.Dependencies, pathToID[dependencyPath])
			}
		}
		slices.Sort(pkg.Dependencies)
		pkg.Dependencies = slices.Compact(pkg.Dependencies)
		snapshot.Packages[id] = pkg
	}

	for name, scope := range directScopes {
		packagePath := path.Join("node_modules", name)
		if id, ok := pathToID[packagePath]; ok {
			pkg := snapshot.Packages[id]
			pkg.Direct = true
			pkg.Scope = strongerScope(pkg.Scope, scope)
			snapshot.Packages[id] = pkg
			snapshot.Roots = append(snapshot.Roots, id)
		}
	}
	slices.Sort(snapshot.Roots)
	snapshot.Roots = slices.Compact(snapshot.Roots)
}

func parseNPMV1(snapshot *Snapshot, dependencies map[string]npmV1Package) {
	byName := map[string][]string{}
	pending := map[string][]string{}
	var walk func(name string, record npmV1Package, direct bool, ancestry string) string
	walk = func(name string, record npmV1Package, direct bool, ancestry string) string {
		scope := ScopeProduction
		if record.Optional {
			scope = ScopeOptional
		} else if record.Dev {
			scope = ScopeDevelopment
		}
		pkg := Package{
			Name:      name,
			Version:   record.Version,
			Source:    record.Resolved,
			Integrity: record.Integrity,
			Scope:     scope,
			Direct:    direct,
			Build:     record.HasInstallScript,
			Metadata:  map[string]string{"lock_path": ancestry + name},
		}
		id := addPackage(snapshot, pkg)
		byName[name] = append(byName[name], id)
		for required := range record.Requires {
			pending[id] = append(pending[id], required)
		}
		childNames := make([]string, 0, len(record.Dependencies))
		for child := range record.Dependencies {
			childNames = append(childNames, child)
		}
		slices.Sort(childNames)
		for _, child := range childNames {
			childID := walk(child, record.Dependencies[child], false, ancestry+name+" > ")
			current := snapshot.Packages[id]
			current.Dependencies = append(current.Dependencies, childID)
			snapshot.Packages[id] = current
		}
		return id
	}

	names := make([]string, 0, len(dependencies))
	for name := range dependencies {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		snapshot.Roots = append(snapshot.Roots, walk(name, dependencies[name], true, ""))
	}
	for id, names := range pending {
		pkg := snapshot.Packages[id]
		for _, name := range names {
			ids := byName[name]
			slices.Sort(ids)
			if len(ids) > 0 {
				pkg.Dependencies = append(pkg.Dependencies, ids[0])
			}
		}
		slices.Sort(pkg.Dependencies)
		pkg.Dependencies = slices.Compact(pkg.Dependencies)
		snapshot.Packages[id] = pkg
	}
	slices.Sort(snapshot.Roots)
	snapshot.Roots = slices.Compact(snapshot.Roots)
}

func npmNameFromPath(packagePath string) string {
	const marker = "node_modules/"
	index := strings.LastIndex(packagePath, marker)
	if index < 0 {
		return ""
	}
	return strings.TrimPrefix(packagePath[index+len(marker):], "/")
}

func resolveNPMPath(packagePath, name string, pathToID map[string]string) string {
	directory := packagePath
	for {
		candidate := path.Clean(path.Join(directory, "node_modules", name))
		if _, ok := pathToID[candidate]; ok {
			return candidate
		}
		if directory == "." || directory == "" {
			break
		}
		next := path.Dir(directory)
		if next == directory {
			break
		}
		directory = next
	}
	candidate := path.Join("node_modules", name)
	if _, ok := pathToID[candidate]; ok {
		return candidate
	}
	return ""
}
