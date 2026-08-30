package depprism

import (
	"io/fs"
	"slices"
	"testing"
)

func TestSupportedFiles(t *testing.T) {
	want := []string{"Cargo.lock", "go.mod", "npm-shrinkwrap.json", "package-lock.json", "pnpm-lock.yaml", "poetry.lock", "uv.lock", "yarn.lock"}
	if got := SupportedFiles(); !slices.Equal(got, want) {
		t.Fatalf("SupportedFiles() = %v, want %v", got, want)
	}
}

func TestParseNPMModern(t *testing.T) {
	snapshot := mustParse(t, "package-lock.json", `{
  "name": "demo", "lockfileVersion": 3,
  "packages": {
    "": {"dependencies":{"alpha":"^1.0.0"},"devDependencies":{"tester":"^2.0.0"}},
    "node_modules/alpha": {"version":"1.2.0","resolved":"https://registry.npmjs.org/alpha/-/alpha-1.2.0.tgz","integrity":"sha512-alpha","dependencies":{"child":"^3.0.0"}},
    "node_modules/child": {"version":"3.1.0","resolved":"https://registry.npmjs.org/child/-/child-3.1.0.tgz","integrity":"sha512-child"},
    "node_modules/tester": {"version":"2.0.0","resolved":"https://registry.npmjs.org/tester/-/tester-2.0.0.tgz","integrity":"sha512-tester","dev":true,"hasInstallScript":true}
  }
}`, nil)
	if len(snapshot.Packages) != 3 || len(snapshot.Roots) != 2 {
		t.Fatalf("unexpected graph size: packages=%d roots=%d", len(snapshot.Packages), len(snapshot.Roots))
	}
	tester := packageNamed(t, snapshot, "tester")
	if !tester.Direct || tester.Scope != ScopeDevelopment || !tester.Build {
		t.Fatalf("tester evidence = %+v", tester)
	}
	alpha := packageNamed(t, snapshot, "alpha")
	if len(alpha.Dependencies) != 1 || snapshot.Packages[alpha.Dependencies[0]].Name != "child" {
		t.Fatalf("alpha graph = %+v", alpha.Dependencies)
	}
}

func TestParsePNPM(t *testing.T) {
	snapshot := mustParse(t, "pnpm-lock.yaml", `
lockfileVersion: '9.0'
importers:
  .:
    dependencies:
      alpha:
        specifier: ^1.0.0
        version: 1.2.0
packages:
  alpha@1.2.0:
    resolution: {integrity: sha512-alpha}
    requiresBuild: true
  child@2.0.0:
    resolution: {integrity: sha512-child}
snapshots:
  alpha@1.2.0:
    dependencies:
      child: 2.0.0
  child@2.0.0: {}
`, nil)
	alpha := packageNamed(t, snapshot, "alpha")
	if !alpha.Direct || !alpha.Build || len(alpha.Dependencies) != 1 {
		t.Fatalf("alpha evidence = %+v", alpha)
	}
}

func TestParseYarnClassic(t *testing.T) {
	snapshot := mustParse(t, "yarn.lock", `
# yarn lockfile v1

alpha@^1.0.0:
  version "1.2.0"
  resolved "https://registry.yarnpkg.com/alpha/-/alpha-1.2.0.tgz"
  integrity sha512-alpha
  dependencies:
    child "^2.0.0"

child@^2.0.0:
  version "2.1.0"
  resolved "https://registry.yarnpkg.com/child/-/child-2.1.0.tgz"
  integrity sha512-child
`, map[string]string{"package.json": `{"dependencies":{"alpha":"^1.0.0"}}`})
	alpha := packageNamed(t, snapshot, "alpha")
	if !alpha.Direct || len(alpha.Dependencies) != 1 || snapshot.Packages[alpha.Dependencies[0]].Name != "child" {
		t.Fatalf("alpha evidence = %+v", alpha)
	}
}

func TestParseYarnBerry(t *testing.T) {
	snapshot := mustParse(t, "yarn.lock", `
__metadata:
  version: 8
  cacheKey: 10c0
"alpha@npm:^1.0.0":
  version: 1.3.0
  resolution: "alpha@npm:1.3.0"
  checksum: 10c0/alpha
  languageName: node
  linkType: hard
`, map[string]string{"package.json": `{"dependencies":{"alpha":"npm:^1.0.0"}}`})
	alpha := packageNamed(t, snapshot, "alpha")
	if !alpha.Direct || alpha.Integrity != "10c0/alpha" {
		t.Fatalf("alpha evidence = %+v", alpha)
	}
}

func TestParseUV(t *testing.T) {
	snapshot := mustParse(t, "uv.lock", `
version = 1
revision = 2

[[package]]
name = "alpha_lib"
version = "1.2.0"
source = { registry = "https://pypi.org/simple" }
sdist = { url = "https://files.pythonhosted.org/alpha.tar.gz", hash = "sha256:alpha" }
dependencies = [{ name = "child", version = "2.0.0", source = { registry = "https://pypi.org/simple" } }]

[[package]]
name = "child"
version = "2.0.0"
source = { registry = "https://pypi.org/simple" }
wheels = [{ url = "https://files.pythonhosted.org/child.whl", hash = "sha256:child" }]
`, map[string]string{"pyproject.toml": `[project]
dependencies = ["alpha_lib>=1"]
`})
	alpha := packageNamed(t, snapshot, "alpha-lib")
	if !alpha.Direct || len(alpha.Dependencies) != 1 || alpha.Metadata["sdist"] != "true" {
		t.Fatalf("alpha evidence = %+v", alpha)
	}
}

func TestParsePoetry(t *testing.T) {
	snapshot := mustParse(t, "poetry.lock", `
[[package]]
name = "alpha"
version = "1.0.0"
optional = false
python-versions = ">=3.11"
groups = ["main"]
files = [{file = "alpha.whl", hash = "sha256:alpha"}]
[package.dependencies]
child = "^2.0"

[[package]]
name = "child"
version = "2.0.0"
optional = false
python-versions = ">=3.11"
groups = ["main"]
files = [{file = "child.whl", hash = "sha256:child"}]

[metadata]
lock-version = "2.1"
content-hash = "abc"
`, map[string]string{"pyproject.toml": `[tool.poetry.dependencies]
python = "^3.11"
alpha = "^1.0"
`})
	alpha := packageNamed(t, snapshot, "alpha")
	if !alpha.Direct || len(alpha.Dependencies) != 1 {
		t.Fatalf("alpha evidence = %+v", alpha)
	}
}

func TestParseCargo(t *testing.T) {
	snapshot := mustParse(t, "Cargo.lock", `
version = 4

[[package]]
name = "alpha"
version = "1.0.0"
source = "registry+https://github.com/rust-lang/crates.io-index"
checksum = "alpha"
dependencies = ["child 2.0.0 (registry+https://github.com/rust-lang/crates.io-index)"]

[[package]]
name = "child"
version = "2.0.0"
source = "registry+https://github.com/rust-lang/crates.io-index"
checksum = "child"
`, map[string]string{"Cargo.toml": `[package]
name = "demo"
version = "0.1.0"
[dependencies]
renamed = { package = "alpha", version = "1" }
`})
	alpha := packageNamed(t, snapshot, "alpha")
	if !alpha.Direct || len(alpha.Dependencies) != 1 {
		t.Fatalf("alpha evidence = %+v", alpha)
	}
}

func TestParseGoMod(t *testing.T) {
	snapshot := mustParse(t, "go.mod", `module example.com/demo

go 1.22

require (
  example.com/alpha v1.2.0
  example.com/child v2.0.0 // indirect
)

replace example.com/alpha => example.com/fork v1.2.1
`, map[string]string{"go.sum": "example.com/fork v1.2.1 h1:fork\nexample.com/child v2.0.0 h1:child\n"})
	alpha := packageNamed(t, snapshot, "example.com/alpha")
	if !alpha.Direct || alpha.Version != "v1.2.1" || alpha.Integrity != "h1:fork" || alpha.Metadata["replaced_from"] == "" {
		t.Fatalf("alpha evidence = %+v", alpha)
	}
	child := packageNamed(t, snapshot, "example.com/child")
	if child.Direct {
		t.Fatalf("indirect child marked direct: %+v", child)
	}
}

func mustParse(t *testing.T, path, primary string, companions map[string]string) *Snapshot {
	t.Helper()
	reader := func(path string) ([]byte, error) {
		if value, ok := companions[path]; ok {
			return []byte(value), nil
		}
		return nil, fs.ErrNotExist
	}
	snapshot, err := Parse(path, []byte(primary), ParseOptions{ReadFile: reader})
	if err != nil {
		t.Fatalf("Parse(%s): %v", path, err)
	}
	return snapshot
}

func packageNamed(t *testing.T, snapshot *Snapshot, name string) Package {
	t.Helper()
	for _, pkg := range snapshot.Packages {
		if pkg.Name == name {
			return pkg
		}
	}
	t.Fatalf("package %q not found in %+v", name, snapshot.Packages)
	return Package{}
}
