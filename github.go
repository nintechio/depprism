package depprism

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// RenderGitHub emits safe workflow annotations and appends the full Markdown
// review to GitHub's step summary when that file is available.
func RenderGitHub(writer io.Writer, review Review) error {
	if _, err := io.WriteString(writer, Text(review)); err != nil {
		return err
	}
	for _, report := range review.Reports {
		for _, finding := range report.PolicyFindings {
			if err := githubAnnotation(writer, "error", finding.Code, report.Path+": "+finding.Message); err != nil {
				return err
			}
		}
		for _, change := range report.Changes {
			for _, finding := range change.Findings {
				level := "warning"
				if finding.Severity == SeverityHigh && !report.Passed {
					level = "error"
				}
				message := report.Path + ": " + change.Name + ": " + finding.Message
				if err := githubAnnotation(writer, level, finding.Code, message); err != nil {
					return err
				}
			}
		}
	}
	if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("open GitHub step summary: %w", err)
		}
		_, writeErr := io.WriteString(file, Markdown(review))
		closeErr := file.Close()
		if writeErr != nil {
			return fmt.Errorf("write GitHub step summary: %w", writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close GitHub step summary: %w", closeErr)
		}
	}
	if path := os.Getenv("GITHUB_OUTPUT"); path != "" {
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("open GitHub output: %w", err)
		}
		_, writeErr := fmt.Fprintf(file, "passed=%t\nadded=%d\nremoved=%d\nupdated=%d\nhigh-risk=%d\n", review.Passed, review.Summary.Added, review.Summary.Removed, review.Summary.Updated, review.Summary.HighRisk)
		closeErr := file.Close()
		if writeErr != nil {
			return fmt.Errorf("write GitHub output: %w", writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close GitHub output: %w", closeErr)
		}
	}
	return nil
}

func githubAnnotation(writer io.Writer, level, title, message string) error {
	title = escapeWorkflowCommand(title, true)
	message = escapeWorkflowCommand(message, false)
	_, err := fmt.Fprintf(writer, "::%s title=%s::%s\n", level, title, message)
	return err
}

func escapeWorkflowCommand(value string, property bool) string {
	value = strings.ReplaceAll(value, "%", "%25")
	value = strings.ReplaceAll(value, "\r", "%0D")
	value = strings.ReplaceAll(value, "\n", "%0A")
	if property {
		value = strings.ReplaceAll(value, ":", "%3A")
		value = strings.ReplaceAll(value, ",", "%2C")
	}
	return value
}
