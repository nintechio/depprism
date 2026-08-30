package depprism

import (
	"encoding/json"
	"encoding/xml"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestRepositoryPoliciesDecode(t *testing.T) {
	for _, path := range []string{".depprism.json", ".depprism.example.json"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodePolicy(data); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	data, err := os.ReadFile("policy.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("policy schema: %v", err)
	}
	if schema["$schema"] == "" || schema["additionalProperties"] != false {
		t.Fatalf("policy schema lacks strict root metadata")
	}
}

func TestRepositoryYAMLParses(t *testing.T) {
	paths := []string{
		"action.yml",
		".github/dependabot.yml",
		".github/workflows/ci.yml",
		".github/workflows/release.yml",
		".github/workflows/changefence.yml",
		".github/workflows/depprism.yml",
		".github/ISSUE_TEMPLATE/bug.yml",
		".github/ISSUE_TEMPLATE/ecosystem.yml",
		".github/ISSUE_TEMPLATE/config.yml",
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := yaml.Unmarshal(data, &value); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
}

func TestRepositorySVGAndSocialPreview(t *testing.T) {
	paths, err := filepath.Glob(".github/assets/*.svg")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 4 {
		t.Fatalf("found %d SVG assets", len(paths))
	}
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		decoder := xml.NewDecoder(file)
		for {
			_, decodeErr := decoder.Token()
			if decodeErr == io.EOF {
				break
			}
			if decodeErr != nil {
				file.Close()
				t.Fatalf("%s: %v", path, decodeErr)
			}
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	file, err := os.Open(".github/assets/social-preview.png")
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(file)
	closeErr := file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if config.Width != 1280 || config.Height != 640 {
		t.Fatalf("social preview is %dx%d", config.Width, config.Height)
	}
}

func TestReadmeReferencesExistingLocalFiles(t *testing.T) {
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		".github/assets/banner.svg",
		".github/assets/demo.svg",
		".github/assets/evidence-flow.svg",
		"docs/ecosystem-evidence.md",
		"docs/policy.md",
		"docs/security-model.md",
		"CONTRIBUTING.md",
		"CODE_OF_CONDUCT.md",
		"SECURITY.md",
	} {
		if !strings.Contains(string(data), path) {
			t.Fatalf("README does not reference %s", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("README target %s: %v", path, err)
		}
	}
}

func FuzzParseDoesNotPanic(f *testing.F) {
	files := SupportedFiles()
	for index, seed := range [][]byte{
		[]byte(`{"lockfileVersion":3,"packages":{}}`),
		[]byte("lockfileVersion: '9.0'\npackages: {}\n"),
		[]byte("version = 1\n"),
		[]byte("module example.com/test\n"),
	} {
		f.Add(uint8(index), seed)
	}
	f.Fuzz(func(t *testing.T, index uint8, data []byte) {
		_, _ = Parse(files[int(index)%len(files)], data, ParseOptions{})
	})
}
