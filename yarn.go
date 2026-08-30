package depprism

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type yarnParser struct{}

func (yarnParser) Names() []string { return []string{"yarn.lock"} }

type yarnRecord struct {
	Selectors    []string
	Version      string
	Source       string
	Integrity    string
	Dependencies map[string]string
}

type yarnBerryRecord struct {
	Version              string         `yaml:"version"`
	Resolution           string         `yaml:"resolution"`
	Checksum             string         `yaml:"checksum"`
	Dependencies         map[string]any `yaml:"dependencies"`
	OptionalDependencies map[string]any `yaml:"optionalDependencies"`
	LinkType             string         `yaml:"linkType"`
}

func (yarnParser) Parse(lockPath string, data []byte, options ParseOptions) (*Snapshot, error) {
	if strings.Contains(string(data), "__metadata:") {
		return parseYarnBerry(lockPath, data, options)
	}
	return parseYarnClassic(lockPath, data, options)
}

func parseYarnClassic(lockPath string, data []byte, options ParseOptions) (*Snapshot, error) {
	records, err := scanYarnClassic(data)
	if err != nil {
		return nil, err
	}
	return normalizeYarn(lockPath, "yarn-lock/v1", records, options)
}

func scanYarnClassic(data []byte) ([]yarnRecord, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var records []yarnRecord
	var current *yarnRecord
	inDependencies := false
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 && strings.HasSuffix(trimmed, ":") {
			selectors, err := splitYarnSelectors(strings.TrimSuffix(trimmed, ":"))
			if err != nil {
				return nil, fmt.Errorf("selector line %q: %w", trimmed, err)
			}
			records = append(records, yarnRecord{Selectors: selectors, Dependencies: map[string]string{}})
			current = &records[len(records)-1]
			inDependencies = false
			continue
		}
		if current == nil {
			continue
		}
		if indent == 2 {
			inDependencies = trimmed == "dependencies:" || trimmed == "optionalDependencies:"
			if inDependencies {
				continue
			}
			key, value := splitYarnField(trimmed)
			switch key {
			case "version":
				current.Version = value
			case "resolved":
				current.Source = value
			case "integrity":
				current.Integrity = value
			}
			continue
		}
		if indent >= 4 && inDependencies {
			name, value := splitYarnField(trimmed)
			if name != "" {
				current.Dependencies[name] = value
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no Yarn v1 package records found")
	}
	return records, nil
}

func parseYarnBerry(lockPath string, data []byte, options ParseOptions) (*Snapshot, error) {
	var raw map[string]yarnBerryRecord
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var records []yarnRecord
	keys := make([]string, 0, len(raw))
	for key := range raw {
		if key != "__metadata" {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		entry := raw[key]
		selectors, err := splitYarnSelectors(key)
		if err != nil {
			return nil, fmt.Errorf("selector line %q: %w", key, err)
		}
		dependencies := map[string]string{}
		for name, value := range entry.Dependencies {
			dependencies[name] = scalarString(value)
		}
		for name, value := range entry.OptionalDependencies {
			dependencies[name] = scalarString(value)
		}
		source := "registry:yarn"
		if berryExternalSource(entry.Resolution) {
			source = entry.Resolution
		}
		records = append(records, yarnRecord{
			Selectors:    selectors,
			Version:      entry.Version,
			Source:       source,
			Integrity:    entry.Checksum,
			Dependencies: dependencies,
		})
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no Yarn Berry package records found")
	}
	return normalizeYarn(lockPath, "yarn-lock/berry", records, options)
}

func normalizeYarn(lockPath, format string, records []yarnRecord, options ParseOptions) (*Snapshot, error) {
	snapshot := &Snapshot{
		Ecosystem: "yarn",
		Path:      lockPath,
		Format:    format,
		Packages:  map[string]Package{},
		Warnings: []Warning{{
			Code:    "build-evidence-unavailable",
			Message: "Yarn lockfiles do not record package lifecycle scripts",
		}},
	}
	selectorToID := map[string]string{}
	byName := map[string][]string{}
	pending := map[string]map[string]string{}
	for _, record := range records {
		if len(record.Selectors) == 0 {
			continue
		}
		name := yarnSelectorName(record.Selectors[0])
		if name == "" || record.Version == "" {
			snapshot.Warnings = append(snapshot.Warnings, Warning{Code: "yarn-record", Message: "skipped a record without a normalized name or version"})
			continue
		}
		source := record.Source
		if source == "" {
			source = "registry:yarn"
		}
		id := addPackage(snapshot, Package{Name: name, Version: record.Version, Source: source, Integrity: record.Integrity, Scope: ScopeUnknown})
		byName[name] = append(byName[name], id)
		pending[id] = record.Dependencies
		for _, selector := range record.Selectors {
			selectorToID[selector] = id
		}
	}
	for name := range byName {
		slices.Sort(byName[name])
	}
	for id, dependencies := range pending {
		pkg := snapshot.Packages[id]
		names := make([]string, 0, len(dependencies))
		for name := range dependencies {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			dependencyID := selectorToID[name+"@"+dependencies[name]]
			if dependencyID == "" && len(byName[name]) > 0 {
				dependencyID = byName[name][0]
			}
			if dependencyID != "" {
				pkg.Dependencies = append(pkg.Dependencies, dependencyID)
			}
		}
		slices.Sort(pkg.Dependencies)
		pkg.Dependencies = slices.Compact(pkg.Dependencies)
		snapshot.Packages[id] = pkg
	}
	manifest, manifestPath, err := readPackageManifest(options, lockPath)
	if err != nil && !companionUnavailable(snapshot, err, manifestPath) {
		return nil, fmt.Errorf("parse %s: %w", manifestPath, err)
	}
	snapshot.ManifestPath = manifestPath
	for _, dependency := range manifest {
		id := selectorToID[dependency.Name+"@"+dependency.Value]
		if id == "" && len(byName[dependency.Name]) > 0 {
			id = byName[dependency.Name][0]
		}
		if id == "" {
			continue
		}
		pkg := snapshot.Packages[id]
		pkg.Direct = true
		pkg.Scope = strongerScope(pkg.Scope, dependency.Scope)
		snapshot.Packages[id] = pkg
		snapshot.Roots = append(snapshot.Roots, id)
	}
	slices.Sort(snapshot.Roots)
	snapshot.Roots = slices.Compact(snapshot.Roots)
	return snapshot, nil
}

func splitYarnSelectors(value string) ([]string, error) {
	reader := csv.NewReader(strings.NewReader(value))
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true
	fields, err := reader.Read()
	if err != nil {
		return nil, err
	}
	for index := range fields {
		fields[index] = unquoteYarn(strings.TrimSpace(fields[index]))
	}
	return fields, nil
}

func splitYarnField(line string) (string, string) {
	index := strings.IndexAny(line, " \t")
	if index < 0 {
		return unquoteYarn(line), ""
	}
	return unquoteYarn(strings.TrimSpace(line[:index])), unquoteYarn(strings.TrimSpace(line[index+1:]))
}

func unquoteYarn(value string) string {
	if result, err := strconv.Unquote(value); err == nil {
		return result
	}
	return strings.Trim(value, "\"")
}

func yarnSelectorName(selector string) string {
	selector = strings.TrimSpace(selector)
	if strings.HasPrefix(selector, "@") {
		if slash := strings.Index(selector, "/"); slash >= 0 {
			if separator := strings.Index(selector[slash+1:], "@"); separator >= 0 {
				return selector[:slash+1+separator]
			}
		}
		return ""
	}
	if separator := strings.Index(selector, "@"); separator > 0 {
		return selector[:separator]
	}
	return ""
}

func berryExternalSource(resolution string) bool {
	lower := strings.ToLower(resolution)
	return strings.Contains(lower, "git") || strings.Contains(lower, "http://") || strings.Contains(lower, "https://") ||
		strings.Contains(lower, "file:") || strings.Contains(lower, "portal:") || strings.Contains(lower, "workspace:") || strings.Contains(lower, "link:")
}
