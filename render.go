package depprism

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"
)

// Format is a stable output format accepted by Render.
type Format string

const (
	FormatText     Format = "text"
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
)

// ParseFormat validates a user-facing format name.
func ParseFormat(value string) (Format, error) {
	format := Format(strings.ToLower(strings.TrimSpace(value)))
	switch format {
	case FormatText, FormatMarkdown, FormatJSON:
		return format, nil
	default:
		return "", fmt.Errorf("unsupported format %q (use text, markdown, json, or github)", value)
	}
}

// Render writes a deterministic repository review.
func Render(writer io.Writer, review Review, format Format) error {
	switch format {
	case FormatJSON:
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(true)
		encoder.SetIndent("", "  ")
		return encoder.Encode(review)
	case FormatMarkdown:
		_, err := io.WriteString(writer, Markdown(review))
		return err
	case FormatText:
		_, err := io.WriteString(writer, Text(review))
		return err
	default:
		return fmt.Errorf("unsupported render format %q", format)
	}
}

// Text returns a compact terminal report.
func Text(review Review) string {
	status := "PASS"
	if !review.Passed {
		status = "FAIL"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "DepPrism %s — %d added, %d removed, %d updated, %d metadata\n", status, review.Summary.Added, review.Summary.Removed, review.Summary.Updated, review.Summary.Metadata)
	if len(review.Reports) == 0 {
		builder.WriteString("No supported dependency changes.\n")
		return builder.String()
	}
	for _, report := range review.Reports {
		fmt.Fprintf(&builder, "\n%s (%s)\n", safeText(report.Path), safeText(report.Ecosystem))
		for _, finding := range report.PolicyFindings {
			fmt.Fprintf(&builder, "  ! %-6s %s: %s\n", strings.ToUpper(string(finding.Severity)), safeText(finding.Code), safeText(finding.Message))
		}
		for _, change := range report.Changes {
			marker := map[ChangeKind]string{ChangeAdded: "+", ChangeRemoved: "-", ChangeUpdated: "~", ChangeMetadata: "!"}[change.Kind]
			fmt.Fprintf(&builder, "  %s %s %s", marker, safeText(change.Name), versionTransition(change))
			if isDirect(change) {
				builder.WriteString(" [direct]")
			}
			builder.WriteByte('\n')
			if len(change.Path) > 1 {
				fmt.Fprintf(&builder, "      via %s\n", safeText(strings.Join(change.Path, " > ")))
			}
			for _, finding := range change.Findings {
				fmt.Fprintf(&builder, "      %s %s: %s\n", strings.ToUpper(string(finding.Severity)), safeText(finding.Code), safeText(finding.Message))
			}
		}
	}
	return builder.String()
}

// Markdown returns a GitHub-flavored review summary. It caps display rows but
// never changes the machine-readable report or policy result.
func Markdown(review Review) string {
	status := "✅ Passed"
	if !review.Passed {
		status = "❌ Policy failed"
	}
	var builder strings.Builder
	builder.WriteString("## DepPrism dependency evidence\n\n")
	fmt.Fprintf(&builder, "%s · **%d** added · **%d** removed · **%d** updated · **%d** metadata · **%d** high-risk evidence\n\n", status, review.Summary.Added, review.Summary.Removed, review.Summary.Updated, review.Summary.Metadata, review.Summary.HighRisk)
	if len(review.Reports) == 0 {
		builder.WriteString("No supported dependency changes detected.\n")
		return builder.String()
	}
	const rowLimit = 200
	rows := 0
	for _, report := range review.Reports {
		fmt.Fprintf(&builder, "### `%s` · %s\n\n", markdownCell(report.Path), markdownCell(report.Ecosystem))
		for _, finding := range report.PolicyFindings {
			fmt.Fprintf(&builder, "> **%s · %s:** %s\n\n", markdownCell(strings.ToUpper(string(finding.Severity))), markdownCell(finding.Code), markdownCell(finding.Message))
		}
		builder.WriteString("| Change | Dependency | Version | Scope | Evidence | Path |\n|---|---|---|---|---|---|\n")
		for _, change := range report.Changes {
			if rows >= rowLimit {
				continue
			}
			rows++
			evidence := "—"
			if len(change.Findings) > 0 {
				items := make([]string, 0, len(change.Findings))
				for _, finding := range change.Findings {
					items = append(items, strings.ToUpper(string(finding.Severity))+" "+finding.Code)
				}
				evidence = strings.Join(items, "<br>")
			}
			path := "—"
			if len(change.Path) > 1 {
				path = strings.Join(change.Path, " → ")
			}
			fmt.Fprintf(&builder, "| %s | `%s` | %s | %s | %s | %s |\n",
				markdownCell(string(change.Kind)), markdownCell(change.Name), markdownCell(versionTransition(change)), markdownCell(changeScope(change)), markdownCell(evidence), markdownCell(path))
		}
		builder.WriteByte('\n')
	}
	total := review.Summary.Added + review.Summary.Removed + review.Summary.Updated + review.Summary.Metadata
	if total > rowLimit {
		fmt.Fprintf(&builder, "_Showing the first %d of %d changes. JSON output contains the full report._\n", rowLimit, total)
	}
	return builder.String()
}

func versionTransition(change Change) string {
	before, after := "∅", "∅"
	if change.Before != nil && change.Before.Version != "" {
		before = change.Before.Version
	}
	if change.After != nil && change.After.Version != "" {
		after = change.After.Version
	}
	if before == after {
		return safeText(after)
	}
	return safeText(before + " → " + after)
}

func changeScope(change Change) string {
	if change.After != nil {
		return string(change.After.Scope)
	}
	if change.Before != nil {
		return string(change.Before.Scope)
	}
	return string(ScopeUnknown)
}

func isDirect(change Change) bool {
	return (change.After != nil && change.After.Direct) || (change.Before != nil && change.Before.Direct)
}

func markdownCell(value string) string {
	value = safeText(value)
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "`", "\\`")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	return value
}

func safeText(value string) string {
	value = strings.Map(func(character rune) rune {
		if character == '\n' || character == '\r' || character == '\t' || unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
	return strings.TrimSpace(value)
}

func sortedReports(reports []Report) []Report {
	result := slices.Clone(reports)
	slices.SortFunc(result, func(left, right Report) int { return strings.Compare(left.Path, right.Path) })
	return result
}
