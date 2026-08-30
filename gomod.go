package depprism

import (
	"bufio"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

type goModParser struct{}

func (goModParser) Names() []string { return []string{"go.mod"} }

type goRequirement struct {
	Path     string
	Version  string
	Indirect bool
}

type goReplacement struct {
	OldPath    string
	OldVersion string
	NewPath    string
	NewVersion string
}

func (goModParser) Parse(modPath string, data []byte, options ParseOptions) (*Snapshot, error) {
	requirements, replacements, err := scanGoMod(data)
	if err != nil {
		return nil, err
	}
	snapshot := &Snapshot{
		Ecosystem: "go",
		Path:      modPath,
		Format:    "go-mod",
		Packages:  map[string]Package{},
		Warnings: []Warning{{
			Code:    "dependency-edges-unavailable",
			Message: "go.mod records the selected module set but not the full module graph",
		}, {
			Code:    "build-evidence-unavailable",
			Message: "go.mod does not record package initialization or code-generation behavior",
		}},
	}
	sums, sumPath, err := readGoSums(options, modPath)
	if err != nil && !isNotExist(err) && err != fs.ErrNotExist {
		return nil, fmt.Errorf("parse %s: %w", sumPath, err)
	}
	if isNotExist(err) || err == fs.ErrNotExist {
		snapshot.Warnings = append(snapshot.Warnings, Warning{Code: "integrity-evidence-unavailable", Message: "go.sum was not provided"})
	}
	snapshot.ManifestPath = modPath
	for _, requirement := range requirements {
		replacement, replaced := replacements[goCoordinate(requirement.Path, requirement.Version)]
		if !replaced {
			replacement, replaced = replacements[goCoordinate(requirement.Path, "")]
		}
		source := "https://proxy.golang.org/" + requirement.Path
		metadata := map[string]string{}
		version := requirement.Version
		integrityPath := requirement.Path
		integrityVersion := requirement.Version
		if replaced {
			metadata["replaced_from"] = requirement.Path + "@" + requirement.Version
			if replacement.NewVersion == "" {
				source = "file:" + replacement.NewPath
			} else {
				source = "https://proxy.golang.org/" + replacement.NewPath
				version = replacement.NewVersion
				integrityPath = replacement.NewPath
				integrityVersion = replacement.NewVersion
			}
		}
		pkg := Package{
			Name:      requirement.Path,
			Version:   version,
			Source:    source,
			Integrity: sums[goCoordinate(integrityPath, integrityVersion)],
			Scope:     ScopeProduction,
			Direct:    !requirement.Indirect,
			Metadata:  metadata,
		}
		id := addPackage(snapshot, pkg)
		if pkg.Direct {
			snapshot.Roots = append(snapshot.Roots, id)
		}
	}
	slices.Sort(snapshot.Roots)
	snapshot.Roots = slices.Compact(snapshot.Roots)
	return snapshot, nil
}

func scanGoMod(data []byte) ([]goRequirement, map[string]goReplacement, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var requirements []goRequirement
	replacements := map[string]goReplacement{}
	block := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if line == ")" {
			block = ""
			continue
		}
		if strings.HasSuffix(line, "(") {
			block = strings.TrimSpace(strings.TrimSuffix(line, "("))
			continue
		}
		keyword := block
		value := line
		if block == "" {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			keyword = fields[0]
			value = strings.TrimSpace(strings.TrimPrefix(line, keyword))
		}
		switch keyword {
		case "require":
			fields := strings.Fields(value)
			if len(fields) < 2 {
				return nil, nil, fmt.Errorf("invalid require directive %q", line)
			}
			requirements = append(requirements, goRequirement{Path: fields[0], Version: fields[1], Indirect: strings.Contains(value, "// indirect")})
		case "replace":
			parts := strings.Split(value, "=>")
			if len(parts) != 2 {
				return nil, nil, fmt.Errorf("invalid replace directive %q", line)
			}
			oldFields := strings.Fields(parts[0])
			newFields := strings.Fields(parts[1])
			if len(oldFields) == 0 || len(newFields) == 0 {
				return nil, nil, fmt.Errorf("invalid replace directive %q", line)
			}
			replacement := goReplacement{OldPath: oldFields[0], NewPath: newFields[0]}
			if len(oldFields) > 1 {
				replacement.OldVersion = oldFields[1]
			}
			if len(newFields) > 1 {
				replacement.NewVersion = newFields[1]
			}
			replacements[goCoordinate(replacement.OldPath, replacement.OldVersion)] = replacement
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return requirements, replacements, nil
}

func readGoSums(options ParseOptions, modPath string) (map[string]string, string, error) {
	data, path, err := readCompanion(options, modPath, "go.sum")
	if err != nil {
		return nil, path, err
	}
	result := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || strings.HasSuffix(fields[1], "/go.mod") {
			continue
		}
		result[goCoordinate(fields[0], fields[1])] = fields[2]
	}
	return result, path, scanner.Err()
}

func goCoordinate(path, version string) string { return path + "\x00" + version }
