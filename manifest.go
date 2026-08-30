package depprism

import (
	"encoding/json"
	"io/fs"
)

type manifestDependency struct {
	Name  string
	Value string
	Scope Scope
}

type packageManifest struct {
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

func readPackageManifest(options ParseOptions, primaryPath string) ([]manifestDependency, string, error) {
	data, path, err := readCompanion(options, primaryPath, "package.json")
	if err != nil {
		return nil, path, err
	}
	var manifest packageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, path, err
	}
	byName := map[string]manifestDependency{}
	add := func(values map[string]string, scope Scope) {
		for name, value := range values {
			existing, ok := byName[name]
			if !ok || strongerScope(existing.Scope, scope) == scope {
				byName[name] = manifestDependency{Name: name, Value: value, Scope: scope}
			}
		}
	}
	add(manifest.DevDependencies, ScopeDevelopment)
	add(manifest.OptionalDependencies, ScopeOptional)
	add(manifest.Dependencies, ScopeProduction)
	result := make([]manifestDependency, 0, len(byName))
	for _, dependency := range byName {
		result = append(result, dependency)
	}
	return result, path, nil
}

func companionUnavailable(snapshot *Snapshot, err error, path string) bool {
	if err == nil {
		return false
	}
	if isNotExist(err) || err == fs.ErrNotExist {
		snapshot.Warnings = append(snapshot.Warnings, Warning{
			Code:    "manifest-unavailable",
			Message: "direct dependency evidence unavailable because " + path + " was not provided",
		})
		return true
	}
	return false
}
