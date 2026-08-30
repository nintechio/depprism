package depprism

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkdownEscapesUntrustedLockfileText(t *testing.T) {
	review := NewReview()
	report := Report{
		SchemaVersion: 1,
		Ecosystem:     "npm",
		Path:          "odd|path\npackage-lock.json",
		Changes: []Change{{
			Kind:  ChangeAdded,
			Name:  "<script>|bad\nname",
			After: &Package{Name: "bad", Version: "1", Scope: ScopeProduction},
		}},
		Summary: Summary{Added: 1},
		Passed:  true,
	}
	review.Add(report)
	markdown := Markdown(review)
	if strings.Contains(markdown, "<script>") || strings.Contains(markdown, "bad\nname") || !strings.Contains(markdown, "\\|") {
		t.Fatalf("unsafe markdown:\n%s", markdown)
	}
}

func TestRenderGitHubWritesEscapedChannels(t *testing.T) {
	directory := t.TempDir()
	summaryPath := filepath.Join(directory, "summary.md")
	outputPath := filepath.Join(directory, "output.txt")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)
	t.Setenv("GITHUB_OUTPUT", outputPath)
	review := NewReview()
	review.Add(Report{
		SchemaVersion: 1,
		Ecosystem:     "npm",
		Path:          "package-lock.json",
		Changes: []Change{{
			Kind:     ChangeAdded,
			Name:     "bad%name\n::error::",
			After:    &Package{Name: "bad", Version: "1", Scope: ScopeProduction},
			Findings: []Finding{{Code: "new-git-source", Severity: SeverityMedium, Message: "line\nbreak%"}},
		}},
		Summary: Summary{Added: 1},
		Passed:  true,
	})
	var console bytes.Buffer
	if err := RenderGitHub(&console, review); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(console.String(), "line\nbreak") || !strings.Contains(console.String(), "%0A") || !strings.Contains(console.String(), "%25") {
		t.Fatalf("workflow command was not escaped: %q", console.String())
	}
	summary, err := os.ReadFile(summaryPath)
	if err != nil || !strings.Contains(string(summary), "DepPrism dependency evidence") {
		t.Fatalf("summary=%q err=%v", summary, err)
	}
	output, err := os.ReadFile(outputPath)
	if err != nil || !strings.Contains(string(output), "passed=true") {
		t.Fatalf("output=%q err=%v", output, err)
	}
}

func TestRenderJSONIsStableSchema(t *testing.T) {
	review := NewReview()
	var output bytes.Buffer
	if err := Render(&output, review, FormatJSON); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"schema_version": 1`) || !strings.Contains(output.String(), `"reports": []`) {
		t.Fatalf("unexpected JSON: %s", output.String())
	}
}
