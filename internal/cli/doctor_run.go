package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fatih/color"
)

type doctorStatus string

const (
	doctorPass doctorStatus = "pass"
	doctorWarn doctorStatus = "warn"
	doctorFail doctorStatus = "fail"
	doctorSkip doctorStatus = "skip"
)

type doctorResult struct {
	ID         string        `json:"id"`
	Label      string        `json:"label,omitempty"`
	Section    string        `json:"section"`
	Status     doctorStatus  `json:"status"`
	Summary    string        `json:"summary"`
	Details    []string      `json:"details,omitempty"`
	Error      string        `json:"error,omitempty"`
	Hint       string        `json:"hint,omitempty"`
	Duration   time.Duration `json:"-"`
	DurationMS int64         `json:"duration_ms,omitempty"`
}

type doctorCheck struct {
	ID       string
	Label    string
	Section  string
	Requires []string
	Timeout  time.Duration
	Run      func(context.Context) doctorResult
}

type doctorReport struct {
	Results []doctorResult `json:"results"`
	Pass    int            `json:"pass"`
	Warn    int            `json:"warn"`
	Fail    int            `json:"fail"`
	Skip    int            `json:"skip"`
}

func runDoctorChecks(ctx context.Context, checks []doctorCheck) doctorReport {
	if ctx == nil {
		ctx = context.Background()
	}
	byID := make(map[string]doctorResult, len(checks))
	report := doctorReport{Results: make([]doctorResult, 0, len(checks))}
	for _, check := range checks {
		started := time.Now()
		result := doctorResult{ID: check.ID, Label: check.Label, Section: check.Section}
		skipReason := ""
		for _, req := range check.Requires {
			prev, ok := byID[req]
			if !ok || prev.Status == doctorFail || prev.Status == doctorSkip {
				skipReason = "prerequisite " + req
				break
			}
		}
		if skipReason != "" {
			result.Status = doctorSkip
			result.Summary = "skipped: " + skipReason
		} else {
			runCtx, cancel := ctx, func() {}
			if check.Timeout > 0 {
				runCtx, cancel = context.WithTimeout(ctx, check.Timeout)
			}
			result = check.Run(runCtx)
			cancel()
			result.ID = check.ID
			if result.Label == "" {
				result.Label = check.Label
			}
			if result.Section == "" {
				result.Section = check.Section
			}
		}
		result.Duration = time.Since(started)
		result.DurationMS = result.Duration.Milliseconds()
		byID[result.ID] = result
		report.Results = append(report.Results, result)
		switch result.Status {
		case doctorPass:
			report.Pass++
		case doctorWarn:
			report.Warn++
		case doctorFail:
			report.Fail++
		default:
			report.Skip++
		}
	}
	return report
}

func renderDoctorReport(out io.Writer, report doctorReport, verbose bool) error {
	current := ""
	for _, item := range report.Results {
		if !verbose && item.Status == doctorSkip {
			continue
		}
		if item.Section != current {
			if current != "" {
				fmt.Fprintln(out)
			}
			fmt.Fprintln(out, cliHeading(item.Section))
			current = item.Section
		}
		label := item.Label
		if label == "" {
			label = item.ID
		}
		fmt.Fprintf(out, "  %s  %-28s  %s\n", doctorStatusText(item.Status), label, item.Summary)
		if verbose {
			fmt.Fprintf(out, "        %s · duration %dms\n", item.ID, item.DurationMS)
		}
		if verbose || item.Status != doctorPass {
			for _, detail := range item.Details {
				fmt.Fprintf(out, "        %s\n", detail)
			}
			if item.Error != "" {
				fmt.Fprintf(out, "        %s\n", item.Error)
			}
			if item.Hint != "" {
				fmt.Fprintf(out, "        %s\n", item.Hint)
			}
		}
	}
	fmt.Fprintf(out, "\nSummary: %d passed, %d warnings, %d failed, %d skipped\n", report.Pass, report.Warn, report.Fail, report.Skip)
	return nil
}

func doctorStatusText(status doctorStatus) string {
	text := fmt.Sprintf("%-4s", strings.ToUpper(string(status)))
	switch status {
	case doctorPass:
		return cliStyled(color.FgHiGreen, color.Bold).Sprint(text)
	case doctorWarn:
		return cliStyled(color.FgHiYellow, color.Bold).Sprint(text)
	case doctorFail:
		return cliStyled(color.FgHiRed, color.Bold).Sprint(text)
	default:
		return cliDim(text)
	}
}

func renderDoctorJSON(out io.Writer, report doctorReport) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
