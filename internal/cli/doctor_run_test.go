package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
)

func TestRunDoctorChecksContinuesAfterFailure(t *testing.T) {
	report := runDoctorChecks(context.Background(), []doctorCheck{
		{ID: "a", Section: "One", Run: func(context.Context) doctorResult {
			return doctorResult{Status: doctorFail, Summary: "broken"}
		}},
		{ID: "b", Section: "Two", Run: func(context.Context) doctorResult {
			return doctorResult{Status: doctorPass, Summary: "ok"}
		}},
	})
	if report.Fail != 1 || report.Pass != 1 || len(report.Results) != 2 {
		t.Fatalf("report = %#v", report)
	}
	if report.Results[1].ID != "b" || report.Results[1].Status != doctorPass {
		t.Fatalf("later check dropped: %#v", report.Results)
	}
}

func TestRunDoctorChecksSkipsFailedPrerequisite(t *testing.T) {
	report := runDoctorChecks(context.Background(), []doctorCheck{
		{ID: "a", Section: "One", Run: func(context.Context) doctorResult {
			return doctorResult{Status: doctorFail, Summary: "broken"}
		}},
		{ID: "b", Section: "One", Requires: []string{"a"}, Run: func(context.Context) doctorResult {
			t.Fatal("dependent check ran")
			return doctorResult{}
		}},
	})
	if report.Skip != 1 || report.Results[1].Status != doctorSkip || !strings.Contains(report.Results[1].Summary, "prerequisite a") {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunDoctorChecksWarningDoesNotFail(t *testing.T) {
	report := runDoctorChecks(context.Background(), []doctorCheck{
		{ID: "a", Section: "One", Run: func(context.Context) doctorResult {
			return doctorResult{Status: doctorWarn, Summary: "degraded"}
		}},
	})
	if report.Fail != 0 || report.Warn != 1 {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunDoctorChecksTimeout(t *testing.T) {
	report := runDoctorChecks(context.Background(), []doctorCheck{
		{ID: "slow", Section: "One", Timeout: 10 * time.Millisecond, Run: func(ctx context.Context) doctorResult {
			<-ctx.Done()
			return doctorResult{Status: doctorWarn, Summary: "timed out", Error: ctx.Err().Error()}
		}},
	})
	if report.Warn != 1 || report.Results[0].Error == "" {
		t.Fatalf("report = %#v", report)
	}
}

func TestRenderDoctorReportSummary(t *testing.T) {
	var out bytes.Buffer
	err := renderDoctorReport(&out, doctorReport{
		Results: []doctorResult{
			{ID: "config.source", Section: "System", Status: doctorPass, Summary: "configuration loaded"},
			{ID: "plugin.lock", Section: "Plugins", Status: doctorFail, Summary: "broken", Error: "nope", Hint: "run cgm plugin verify"},
			{ID: "tunnel.collection", Section: "Integrations", Status: doctorSkip, Summary: "no Secure MCP tunnels configured"},
		},
		Pass: 1, Fail: 1, Skip: 1,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, expected := range []string{"System", "Plugins", "PASS", "FAIL", "config.source", "Summary: 1 passed, 0 warnings, 1 failed, 1 skipped", "nope", "run cgm plugin verify"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
	if strings.Contains(text, "tunnel.collection") || strings.Contains(text, "no Secure MCP tunnels") {
		t.Fatalf("skip shown without verbose: %q", text)
	}
}

func TestRenderDoctorReportBrightStatusColors(t *testing.T) {
	previous := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = previous })
	t.Setenv("NO_COLOR", "")
	var out bytes.Buffer
	err := renderDoctorReport(&out, doctorReport{
		Results: []doctorResult{
			{ID: "a", Section: "System", Status: doctorPass, Summary: "ok"},
			{ID: "b", Section: "System", Status: doctorWarn, Summary: "degraded"},
			{ID: "c", Section: "System", Status: doctorFail, Summary: "broken"},
		},
		Pass: 1, Warn: 1, Fail: 1,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, expected := range []string{"\x1b[92;1mPASS", "\x1b[93;1mWARN", "\x1b[91;1mFAIL"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
}

func TestRenderDoctorReportVerboseShowsSkipAndID(t *testing.T) {
	var out bytes.Buffer
	err := renderDoctorReport(&out, doctorReport{
		Results: []doctorResult{
			{ID: "tunnel.collection", Label: "Secure MCP tunnels", Section: "Integrations", Status: doctorSkip, Summary: "no Secure MCP tunnels configured"},
		},
		Skip: 1,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, expected := range []string{"SKIP", "Secure MCP tunnels", "tunnel.collection", "no Secure MCP tunnels configured"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
}

func TestErrDoctorFailed(t *testing.T) {
	if !errors.Is(errDoctorFailed, errDoctorFailed) {
		t.Fatal("sentinel")
	}
}
