package depprism

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

var errGitPathMissing = errors.New("path does not exist in Git revision")

// GitOptions controls repository-level review. PolicyPath is always read from
// the base commit, never from the untrusted head commit.
type GitOptions struct {
	Repository string
	Base       string
	Head       string
	PolicyPath string
}

// ReviewGit compares supported dependency files directly from Git objects.
func ReviewGit(options GitOptions) (Review, error) {
	repository := options.Repository
	if repository == "" {
		repository = "."
	}
	repository, err := filepath.Abs(repository)
	if err != nil {
		return Review{}, fmt.Errorf("resolve repository path: %w", err)
	}
	rootBytes, err := runGit(repository, "rev-parse", "--show-toplevel")
	if err != nil {
		return Review{}, fmt.Errorf("locate repository: %w", err)
	}
	repository = strings.TrimSpace(string(rootBytes))
	baseRef := strings.TrimSpace(options.Base)
	if baseRef == "" {
		baseRef, err = inferBaseRef()
		if err != nil {
			return Review{}, err
		}
	}
	headRef := strings.TrimSpace(options.Head)
	if headRef == "" {
		headRef = "HEAD"
	}
	base, err := resolveCommit(repository, baseRef)
	if err != nil {
		return Review{}, fmt.Errorf("resolve base %q: %w", baseRef, err)
	}
	head, err := resolveCommit(repository, headRef)
	if err != nil {
		return Review{}, fmt.Errorf("resolve head %q: %w", headRef, err)
	}
	policy, err := readGitPolicy(repository, base, options.PolicyPath)
	if err != nil {
		return Review{}, err
	}
	paths, err := changedDependencyPaths(repository, base, head)
	if err != nil {
		return Review{}, err
	}
	review := NewReview()
	review.Base = base
	review.Head = head
	for _, path := range paths {
		report, ok, err := compareGitPath(repository, base, head, path, policy)
		if err != nil {
			return Review{}, err
		}
		if ok {
			review.Add(report)
		}
	}
	return review, nil
}

func compareGitPath(repository, base, head, path string, policy Policy) (Report, bool, error) {
	beforeData, beforeErr := readGitFile(repository, base, path)
	afterData, afterErr := readGitFile(repository, head, path)
	if beforeErr != nil && !errors.Is(beforeErr, errGitPathMissing) {
		return Report{}, false, fmt.Errorf("read %s at base: %w", path, beforeErr)
	}
	if afterErr != nil && !errors.Is(afterErr, errGitPathMissing) {
		return Report{}, false, fmt.Errorf("read %s at head: %w", path, afterErr)
	}
	if errors.Is(beforeErr, errGitPathMissing) && errors.Is(afterErr, errGitPathMissing) {
		return Report{}, false, nil
	}
	var before, after *Snapshot
	var err error
	if beforeErr == nil {
		before, err = Parse(path, beforeData, ParseOptions{ReadFile: gitReader(repository, base)})
		if err != nil {
			return Report{}, false, fmt.Errorf("base %s: %w", path, err)
		}
	}
	if afterErr == nil {
		after, err = Parse(path, afterData, ParseOptions{ReadFile: gitReader(repository, head)})
		if err != nil {
			return Report{}, false, fmt.Errorf("head %s: %w", path, err)
		}
	}
	if before == nil {
		before = EmptySnapshot(after)
	}
	if after == nil {
		after = EmptySnapshot(before)
	}
	report, err := Compare(before, after, policy)
	if err != nil {
		return Report{}, false, fmt.Errorf("compare %s: %w", path, err)
	}
	return report, true, nil
}

func changedDependencyPaths(repository, base, head string) ([]string, error) {
	output, err := runGit(repository, "diff", "--name-only", "-z", "--diff-filter=ACDMRTUXB", base, head, "--")
	if err != nil {
		return nil, fmt.Errorf("list changed paths: %w", err)
	}
	supported := map[string]bool{}
	for _, name := range SupportedFiles() {
		supported[name] = true
	}
	var result []string
	for _, raw := range bytes.Split(output, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		path := filepath.ToSlash(string(raw))
		if err := validateGitPath(path); err != nil {
			return nil, fmt.Errorf("unsafe changed path: %w", err)
		}
		if supported[filepath.Base(path)] {
			result = append(result, path)
		}
	}
	slices.Sort(result)
	return slices.Compact(result), nil
}

func readGitPolicy(repository, base, policyPath string) (Policy, error) {
	if policyPath == "" {
		policyPath = ".depprism.json"
	}
	policyPath = filepath.ToSlash(filepath.Clean(policyPath))
	if err := validateGitPath(policyPath); err != nil {
		return Policy{}, fmt.Errorf("policy path: %w", err)
	}
	data, err := readGitFile(repository, base, policyPath)
	if errors.Is(err, errGitPathMissing) {
		return DefaultPolicy(), nil
	}
	if err != nil {
		return Policy{}, fmt.Errorf("read trusted policy %s at base: %w", policyPath, err)
	}
	policy, err := DecodePolicy(data)
	if err != nil {
		return Policy{}, fmt.Errorf("decode trusted policy %s at base: %w", policyPath, err)
	}
	return policy, nil
}

func resolveCommit(repository, ref string) (string, error) {
	if strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, "\x00\r\n") {
		return "", fmt.Errorf("unsafe Git ref")
	}
	output, err := runGit(repository, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	commit := strings.TrimSpace(string(output))
	if len(commit) != 40 && len(commit) != 64 {
		return "", fmt.Errorf("Git returned an invalid object ID")
	}
	return commit, nil
}

func inferBaseRef() (string, error) {
	if value := strings.TrimSpace(os.Getenv("DEPPRISM_BASE_SHA")); value != "" {
		return value, nil
	}
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath == "" {
		return "", errors.New("base revision is required (pass --base or set DEPPRISM_BASE_SHA)")
	}
	info, err := os.Stat(eventPath)
	if err != nil {
		return "", fmt.Errorf("inspect GitHub event: %w", err)
	}
	if info.Size() > 2*1024*1024 {
		return "", errors.New("GitHub event payload exceeds 2 MiB")
	}
	data, err := os.ReadFile(eventPath)
	if err != nil {
		return "", fmt.Errorf("read GitHub event: %w", err)
	}
	var event struct {
		PullRequest struct {
			Base struct {
				SHA string `json:"sha"`
			} `json:"base"`
		} `json:"pull_request"`
		Before string `json:"before"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return "", fmt.Errorf("decode GitHub event: %w", err)
	}
	if event.PullRequest.Base.SHA != "" {
		return event.PullRequest.Base.SHA, nil
	}
	if event.Before != "" && strings.Trim(event.Before, "0") != "" {
		return event.Before, nil
	}
	return "", errors.New("GitHub event does not identify a base revision; pass --base")
}

func gitReader(repository, commit string) ReadFile {
	return func(path string) ([]byte, error) {
		data, err := readGitFile(repository, commit, path)
		if errors.Is(err, errGitPathMissing) {
			return nil, os.ErrNotExist
		}
		return data, err
	}
}

func readGitFile(repository, commit, path string) ([]byte, error) {
	if err := validateGitPath(path); err != nil {
		return nil, err
	}
	object := commit + ":" + path
	sizeCommand := exec.Command("git", "-c", "safe.directory="+repository, "-C", repository, "cat-file", "-s", object)
	var sizeStderr bytes.Buffer
	sizeCommand.Stderr = &sizeStderr
	sizeOutput, err := sizeCommand.Output()
	if err != nil {
		message := strings.ToLower(sizeStderr.String())
		if strings.Contains(message, "does not exist") || strings.Contains(message, "not a valid object name") || strings.Contains(message, "path '") {
			return nil, errGitPathMissing
		}
		return nil, fmt.Errorf("git cat-file size: %s", strings.TrimSpace(sizeStderr.String()))
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(sizeOutput)), 10, 64)
	if err != nil || size < 0 {
		return nil, errors.New("git cat-file returned an invalid blob size")
	}
	if size > MaxInputBytes {
		return nil, fmt.Errorf("Git blob exceeds %d MiB limit", MaxInputBytes>>20)
	}
	command := exec.Command("git", "-c", "safe.directory="+repository, "-C", repository, "cat-file", "blob", object)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git cat-file: %s", strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

func validateGitPath(path string) error {
	if path == "" || strings.ContainsAny(path, "\x00\r\n") || filepath.IsAbs(path) {
		return fmt.Errorf("invalid repository path %q", path)
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == ".." || strings.HasPrefix(clean, "../") || clean != path {
		return fmt.Errorf("invalid repository path %q", path)
	}
	return nil
}

func runGit(repository string, arguments ...string) ([]byte, error) {
	command := exec.Command("git", append([]string{"-c", "safe.directory=" + repository, "-C", repository}, arguments...)...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, errors.New(message)
	}
	return output, nil
}
