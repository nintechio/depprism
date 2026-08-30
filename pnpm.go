package depprism

import (
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

type pnpmParser struct{}

func (pnpmParser) Names() []string {
	return []string{"pnpm-lock.yaml"}
}

type pnpmLock struct {
	LockfileVersion any                     `yaml:"lockfileVersion"`
	Importers       map[string]pnpmImporter `yaml:"importers"`
	Packages        map[string]pnpmRecord   `yaml:"packages"`
	Snapshots       map[string]pnpmRecord   `yaml:"snapshots"`
}

type pnpmImporter struct {
	Dependencies         map[string]any `yaml:"dependencies"`
	DevDependencies      map[string]any `yaml:"devDependencies"`
	OptionalDependencies map[string]any `yaml:"optionalDependencies"`
}

type pnpmRecord struct {
	Resolution           map[string]any `yaml:"resolution"`
	Dependencies         map[string]any `yaml:"dependencies"`
	OptionalDependencies map[string]any `yaml:"optionalDependencies"`
	Dev                  bool           `yaml:"dev"`
	Optional             bool           `yaml:"optional"`
	HasBin               bool           `yaml:"hasBin"`
	RequiresBuild        bool           `yaml:"requiresBuild"`
}

func (pnpmParser) Parse(lockPath string, data []byte, _ ParseOptions) (*Snapshot, error) {
	var lock pnpmLock
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	version := scalarString(lock.LockfileVersion)
	if version == "" {
		return nil, fmt.Errorf("missing lockfileVersion")
	}
	major := strings.SplitN(version, ".", 2)[0]
	if !slices.Contains([]string{"5", "6", "7", "8", "9", "10"}, major) {
		return nil, fmt.Errorf("unsupported pnpm lockfileVersion %q", version)
	}
	snapshot := &Snapshot{
		Ecosystem: "pnpm",
		Path:      lockPath,
		Format:    "pnpm-lock/v" + version,
		Packages:  map[string]Package{},
	}

	keyToID := map[string]string{}
	coordinateToIDs := map[string][]string{}
	keys := make([]string, 0, len(lock.Packages))
	for key := range lock.Packages {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		record := lock.Packages[key]
		name, packageVersion := parsePNPMKey(key)
		if name == "" {
			snapshot.Warnings = append(snapshot.Warnings, Warning{Code: "pnpm-key", Message: "could not normalize package key " + key})
			continue
		}
		source := "registry:npm"
		if tarball := scalarString(record.Resolution["tarball"]); tarball != "" {
			source = tarball
		}
		if directory := scalarString(record.Resolution["directory"]); directory != "" {
			source = "file:" + directory
		}
		integrity := scalarString(record.Resolution["integrity"])
		scope := ScopeProduction
		if record.Optional {
			scope = ScopeOptional
		} else if record.Dev {
			scope = ScopeDevelopment
		}
		id := addPackage(snapshot, Package{
			Name:      name,
			Version:   packageVersion,
			Source:    source,
			Integrity: integrity,
			Scope:     scope,
			Build:     record.RequiresBuild,
			Metadata: map[string]string{
				"lock_key": key,
				"has_bin":  fmt.Sprintf("%t", record.HasBin),
			},
		})
		keyToID[key] = id
		coordinate := pnpmCoordinate(name, packageVersion)
		coordinateToIDs[coordinate] = append(coordinateToIDs[coordinate], id)
	}
	for coordinate := range coordinateToIDs {
		slices.Sort(coordinateToIDs[coordinate])
	}

	graphRecords := lock.Packages
	if len(lock.Snapshots) > 0 {
		graphRecords = lock.Snapshots
	}
	graphKeys := make([]string, 0, len(graphRecords))
	for key := range graphRecords {
		graphKeys = append(graphKeys, key)
	}
	slices.Sort(graphKeys)
	for _, key := range graphKeys {
		id := keyToID[key]
		if id == "" {
			name, packageVersion := parsePNPMKey(key)
			ids := coordinateToIDs[pnpmCoordinate(name, packageVersion)]
			if len(ids) > 0 {
				id = ids[0]
			}
		}
		if id == "" {
			continue
		}
		pkg := snapshot.Packages[id]
		record := graphRecords[key]
		appendPNPMDependencies(&pkg, record.Dependencies, coordinateToIDs)
		appendPNPMDependencies(&pkg, record.OptionalDependencies, coordinateToIDs)
		slices.Sort(pkg.Dependencies)
		pkg.Dependencies = slices.Compact(pkg.Dependencies)
		snapshot.Packages[id] = pkg
	}

	importerNames := make([]string, 0, len(lock.Importers))
	for importer := range lock.Importers {
		importerNames = append(importerNames, importer)
	}
	slices.Sort(importerNames)
	for _, importerName := range importerNames {
		importer := lock.Importers[importerName]
		markPNPMDirect(snapshot, importer.Dependencies, ScopeProduction, coordinateToIDs)
		markPNPMDirect(snapshot, importer.DevDependencies, ScopeDevelopment, coordinateToIDs)
		markPNPMDirect(snapshot, importer.OptionalDependencies, ScopeOptional, coordinateToIDs)
	}
	slices.Sort(snapshot.Roots)
	snapshot.Roots = slices.Compact(snapshot.Roots)
	return snapshot, nil
}

func appendPNPMDependencies(pkg *Package, dependencies map[string]any, index map[string][]string) {
	names := make([]string, 0, len(dependencies))
	for name := range dependencies {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		version := pnpmReferenceVersion(dependencies[name])
		ids := index[pnpmCoordinate(name, version)]
		if len(ids) == 0 {
			ids = pnpmIDsByName(index, name)
		}
		if len(ids) > 0 {
			pkg.Dependencies = append(pkg.Dependencies, ids[0])
		}
	}
}

func markPNPMDirect(snapshot *Snapshot, dependencies map[string]any, scope Scope, index map[string][]string) {
	names := make([]string, 0, len(dependencies))
	for name := range dependencies {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		version := pnpmReferenceVersion(dependencies[name])
		ids := index[pnpmCoordinate(name, version)]
		if len(ids) == 0 {
			ids = pnpmIDsByName(index, name)
		}
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
}

func parsePNPMKey(key string) (string, string) {
	clean := strings.TrimPrefix(strings.TrimSpace(key), "/")
	if index := strings.Index(clean, "("); index >= 0 {
		clean = clean[:index]
	}
	if strings.HasPrefix(clean, "@") {
		slash := strings.Index(clean, "/")
		if slash < 0 {
			return "", ""
		}
		separator := strings.LastIndex(clean[slash+1:], "@")
		if separator >= 0 {
			separator += slash + 1
			return clean[:separator], clean[separator+1:]
		}
		separator = strings.LastIndex(clean, "/")
		if separator > slash {
			return clean[:separator], clean[separator+1:]
		}
		return "", ""
	}
	if separator := strings.LastIndex(clean, "@"); separator > 0 {
		return clean[:separator], clean[separator+1:]
	}
	if separator := strings.LastIndex(clean, "/"); separator > 0 {
		return clean[:separator], clean[separator+1:]
	}
	return "", ""
}

func pnpmReferenceVersion(value any) string {
	if object, ok := value.(map[string]any); ok {
		value = object["version"]
	}
	version := scalarString(value)
	if index := strings.Index(version, "("); index >= 0 {
		version = version[:index]
	}
	if strings.HasPrefix(version, "npm:") {
		alias := strings.TrimPrefix(version, "npm:")
		if index := strings.LastIndex(alias, "@"); index >= 0 {
			version = alias[index+1:]
		}
	}
	return version
}

func pnpmCoordinate(name, version string) string {
	return name + "\x00" + version
}

func pnpmIDsByName(index map[string][]string, name string) []string {
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

func scalarString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case float64:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%f", typed), "0"), ".")
	default:
		return fmt.Sprint(typed)
	}
}
