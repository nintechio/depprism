package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/nintechio/depprism"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) == 0 {
		usage(stderr)
		return 2
	}
	var passed bool
	var err error
	switch arguments[0] {
	case "diff":
		passed, err = runDiff(arguments[1:], stdout, stderr)
	case "git":
		passed, err = runGit(arguments[1:], stdout, stderr)
	case "inspect":
		passed, err = runInspect(arguments[1:], stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprintln(stdout, buildVersion())
		return 0
	case "help", "--help", "-h":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "depprism: unknown command %q\n\n", arguments[0])
		usage(stderr)
		return 2
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "depprism: %v\n", err)
		return 2
	}
	if !passed {
		return 1
	}
	return 0
}

func runDiff(arguments []string, stdout, stderr io.Writer) (bool, error) {
	flags := flag.NewFlagSet("diff", flag.ContinueOnError)
	flags.SetOutput(stderr)
	formatName := flags.String("format", "text", "output format: text, markdown, json, or github")
	policyPath := flags.String("policy", "", "policy JSON file (default policy when omitted)")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: depprism diff [options] BEFORE AFTER")
		flags.PrintDefaults()
	}
	if err := flags.Parse(arguments); err != nil {
		return false, err
	}
	if flags.NArg() != 2 {
		return false, errors.New("diff requires BEFORE and AFTER dependency files")
	}
	policy, err := localPolicy(*policyPath)
	if err != nil {
		return false, err
	}
	before, err := parseLocal(flags.Arg(0))
	if err != nil {
		return false, fmt.Errorf("before: %w", err)
	}
	after, err := parseLocal(flags.Arg(1))
	if err != nil {
		return false, fmt.Errorf("after: %w", err)
	}
	report, err := depprism.Compare(before, after, policy)
	if err != nil {
		return false, err
	}
	review := depprism.NewReview()
	review.Add(report)
	if err := render(stdout, review, *formatName); err != nil {
		return false, err
	}
	return review.Passed, nil
}

func runGit(arguments []string, stdout, stderr io.Writer) (bool, error) {
	flags := flag.NewFlagSet("git", flag.ContinueOnError)
	flags.SetOutput(stderr)
	base := flags.String("base", "", "trusted base commit (inferred in GitHub Actions)")
	head := flags.String("head", "HEAD", "head commit")
	repository := flags.String("repo", ".", "repository working tree")
	policy := flags.String("policy", ".depprism.json", "repository-relative policy path, read from base")
	formatName := flags.String("format", "text", "output format: text, markdown, json, or github")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: depprism git [options]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(arguments); err != nil {
		return false, err
	}
	if flags.NArg() != 0 {
		return false, errors.New("git does not accept positional arguments")
	}
	review, err := depprism.ReviewGit(depprism.GitOptions{
		Repository: *repository,
		Base:       *base,
		Head:       *head,
		PolicyPath: *policy,
	})
	if err != nil {
		return false, err
	}
	if err := render(stdout, review, *formatName); err != nil {
		return false, err
	}
	return review.Passed, nil
}

func runInspect(arguments []string, stdout, stderr io.Writer) (bool, error) {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	flags.SetOutput(stderr)
	formatName := flags.String("format", "json", "output format: json or text")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: depprism inspect [--format json|text] FILE")
		flags.PrintDefaults()
	}
	if err := flags.Parse(arguments); err != nil {
		return false, err
	}
	if flags.NArg() != 1 {
		return false, errors.New("inspect requires one dependency file")
	}
	snapshot, err := parseLocal(flags.Arg(0))
	if err != nil {
		return false, err
	}
	switch strings.ToLower(*formatName) {
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(true)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(snapshot); err != nil {
			return false, err
		}
	case "text":
		direct := 0
		for _, pkg := range snapshot.Packages {
			if pkg.Direct {
				direct++
			}
		}
		fmt.Fprintf(stdout, "%s · %s · %d packages · %d direct · %d warnings\n", snapshot.Path, snapshot.Ecosystem, len(snapshot.Packages), direct, len(snapshot.Warnings))
	default:
		return false, fmt.Errorf("unsupported inspect format %q (use json or text)", *formatName)
	}
	return true, nil
}

func parseLocal(path string) (*depprism.Snapshot, error) {
	if err := checkLocalSize(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	reader := func(companion string) ([]byte, error) {
		if filepath.IsAbs(companion) {
			if err := checkLocalSize(companion); err != nil {
				return nil, err
			}
			return os.ReadFile(companion)
		}
		companion = filepath.Join(filepath.Dir(absolute), filepath.Base(companion))
		if err := checkLocalSize(companion); err != nil {
			return nil, err
		}
		return os.ReadFile(companion)
	}
	return depprism.Parse(filepath.ToSlash(absolute), data, depprism.ParseOptions{ReadFile: reader})
}

func checkLocalSize(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	if info.Size() > depprism.MaxInputBytes {
		return fmt.Errorf("%s exceeds %d MiB limit", path, depprism.MaxInputBytes>>20)
	}
	return nil
}

func localPolicy(path string) (depprism.Policy, error) {
	if path == "" {
		return depprism.DefaultPolicy(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return depprism.Policy{}, fmt.Errorf("read policy: %w", err)
	}
	policy, err := depprism.DecodePolicy(data)
	if err != nil {
		return depprism.Policy{}, fmt.Errorf("decode policy: %w", err)
	}
	return policy, nil
}

func render(writer io.Writer, review depprism.Review, formatName string) error {
	if strings.EqualFold(formatName, "github") {
		return depprism.RenderGitHub(writer, review)
	}
	format, err := depprism.ParseFormat(formatName)
	if err != nil {
		return err
	}
	return depprism.Render(writer, review, format)
}

func buildVersion() string {
	if version != "dev" {
		return "depprism " + version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return "depprism " + info.Main.Version
	}
	return "depprism dev"
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, `DepPrism turns lockfile noise into dependency evidence.

Usage:
  depprism diff [options] BEFORE AFTER
  depprism git [options]
  depprism inspect [options] FILE
  depprism version

Exit codes:
  0  review passed
  1  review completed and policy failed
  2  input, configuration, or operational error`)
}
