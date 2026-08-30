package depprism

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

// MaxInputBytes is the largest primary or companion dependency file accepted
// by the in-process parsers. The limit bounds CI memory exposure to untrusted
// committed input while accommodating large monorepo lockfiles.
const MaxInputBytes = 64 << 20

// ReadFile supplies companion manifests from the same logical revision as the
// lockfile. Paths are repository-relative and slash-separated.
type ReadFile func(path string) ([]byte, error)

// ParseOptions controls evidence available to a parser.
type ParseOptions struct {
	ReadFile ReadFile
}

type lockParser interface {
	Names() []string
	Parse(path string, data []byte, options ParseOptions) (*Snapshot, error)
}

var parsers = []lockParser{
	npmParser{},
	pnpmParser{},
	yarnParser{},
	uvParser{},
	poetryParser{},
	cargoParser{},
	goModParser{},
}

// SupportedFiles returns the primary lockfile or manifest basenames DepPrism
// recognizes. Companion files are read automatically when available.
func SupportedFiles() []string {
	var names []string
	for _, parser := range parsers {
		names = append(names, parser.Names()...)
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// Parse normalizes one supported dependency file.
func Parse(path string, data []byte, options ParseOptions) (*Snapshot, error) {
	if len(data) > MaxInputBytes {
		return nil, fmt.Errorf("dependency file exceeds %d MiB limit", MaxInputBytes>>20)
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	base := filepath.Base(clean)
	for _, parser := range parsers {
		if slices.Contains(parser.Names(), base) {
			snapshot, err := parser.Parse(clean, data, options)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", clean, err)
			}
			if err := snapshot.Validate(); err != nil {
				return nil, fmt.Errorf("normalize %s: %w", clean, err)
			}
			return snapshot, nil
		}
	}
	return nil, fmt.Errorf("unsupported dependency file %q", base)
}

func readCompanion(options ParseOptions, primaryPath, name string) ([]byte, string, error) {
	if options.ReadFile == nil {
		return nil, "", fs.ErrNotExist
	}
	path := filepath.ToSlash(filepath.Join(filepath.Dir(primaryPath), name))
	data, err := options.ReadFile(path)
	if err != nil {
		return nil, path, err
	}
	if len(data) > MaxInputBytes {
		return nil, path, fmt.Errorf("companion file exceeds %d MiB limit", MaxInputBytes>>20)
	}
	return data, path, nil
}

func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

func sourceKind(source string) string {
	lower := strings.ToLower(source)
	switch {
	case strings.HasPrefix(lower, "git+"),
		strings.HasPrefix(lower, "git://"),
		strings.HasPrefix(lower, "ssh://"),
		strings.HasPrefix(lower, "git@"),
		strings.Contains(lower, "github.com/") && strings.Contains(lower, "#"):
		return "git"
	case strings.HasPrefix(lower, "registry:"), strings.HasPrefix(lower, "sparse+"),
		strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return "registry"
	case strings.HasPrefix(lower, "file:"), strings.HasPrefix(lower, "link:"), strings.HasPrefix(lower, "workspace:"):
		return "local"
	case source == "":
		return "unknown"
	default:
		return "other"
	}
}
