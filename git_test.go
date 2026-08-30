package depprism

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReviewGitUsesPolicyFromTrustedBase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repository := t.TempDir()
	gitTest(t, repository, "init", "-q")
	gitTest(t, repository, "config", "user.name", "DepPrism Test")
	gitTest(t, repository, "config", "user.email", "test@example.invalid")
	writeTestFile(t, repository, ".depprism.json", `{"fail_on":["integrity-drift"]}`)
	writeTestFile(t, repository, "package-lock.json", npmIntegrityLock("sha512:old"))
	gitTest(t, repository, "add", ".")
	gitTest(t, repository, "commit", "-qm", "base")
	base := gitTest(t, repository, "rev-parse", "HEAD")

	writeTestFile(t, repository, ".depprism.json", `{"fail_on":[]}`)
	writeTestFile(t, repository, "package-lock.json", npmIntegrityLock("sha512:new"))
	gitTest(t, repository, "add", ".")
	gitTest(t, repository, "commit", "-qm", "head")
	head := gitTest(t, repository, "rev-parse", "HEAD")

	review, err := ReviewGit(GitOptions{Repository: repository, Base: base, Head: head, PolicyPath: ".depprism.json"})
	if err != nil {
		t.Fatal(err)
	}
	if review.Passed || review.Summary.PolicyFails != 1 || len(review.Reports) != 1 {
		t.Fatalf("trusted-base review = %+v", review)
	}
}

func TestReviewGitHandlesAddedLockfile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repository := t.TempDir()
	gitTest(t, repository, "init", "-q")
	gitTest(t, repository, "config", "user.name", "DepPrism Test")
	gitTest(t, repository, "config", "user.email", "test@example.invalid")
	writeTestFile(t, repository, "README.md", "base")
	gitTest(t, repository, "add", ".")
	gitTest(t, repository, "commit", "-qm", "base")
	base := gitTest(t, repository, "rev-parse", "HEAD")
	writeTestFile(t, repository, "package-lock.json", npmIntegrityLock("sha512:new"))
	gitTest(t, repository, "add", ".")
	gitTest(t, repository, "commit", "-qm", "head")

	review, err := ReviewGit(GitOptions{Repository: repository, Base: base})
	if err != nil {
		t.Fatal(err)
	}
	if !review.Passed || review.Summary.Added != 1 {
		t.Fatalf("added lockfile review = %+v", review)
	}
}

func npmIntegrityLock(integrity string) string {
	return `{"name":"demo","lockfileVersion":3,"packages":{"":{"dependencies":{"alpha":"1.0.0"}},"node_modules/alpha":{"version":"1.0.0","resolved":"https://registry.npmjs.org/alpha/-/alpha-1.0.0.tgz","integrity":"` + integrity + `"}}}`
}

func gitTest(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repository}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return string(output)
}

func writeTestFile(t *testing.T, repository, path, content string) {
	t.Helper()
	fullPath := filepath.Join(repository, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
