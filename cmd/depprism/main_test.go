package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHelpAndErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"help"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "dependency evidence") {
		t.Fatalf("help: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"unknown"}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("unknown: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunDemoReturnsPolicyExit(t *testing.T) {
	root := filepath.Join("..", "..")
	before := filepath.Join(root, "testdata", "demo", "before", "package-lock.json")
	after := filepath.Join(root, "testdata", "demo", "after", "package-lock.json")
	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "--format", "text", before, after}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stdout.String(), "integrity-drift") || stderr.Len() != 0 {
		t.Fatalf("demo: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunInspectAndStrictPolicyError(t *testing.T) {
	root := filepath.Join("..", "..")
	lockfile := filepath.Join(root, "testdata", "demo", "after", "package-lock.json")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"inspect", "--format", "text", lockfile}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "3 packages") {
		t.Fatalf("inspect: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	policy := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(policy, []byte(`{"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"diff", "--policy", policy, lockfile, lockfile}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "unknown field") {
		t.Fatalf("policy: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
