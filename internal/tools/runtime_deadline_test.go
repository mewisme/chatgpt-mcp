package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestToolCallContextReservesTunnelResponseTime(t *testing.T) {
	now := time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)
	parent, cancel := context.WithDeadline(context.Background(), now.Add(20*time.Second))
	defer cancel()
	ctx, cancelCall := toolCallContext(parent, "tunnel", now)
	defer cancelCall()
	deadline, ok := ctx.Deadline()
	if !ok || !deadline.Equal(now.Add(15*time.Second)) {
		t.Fatalf("deadline=%v ok=%t want=%v", deadline, ok, now.Add(15*time.Second))
	}
}

func TestToolCallContextCapsTunnelResponseReserve(t *testing.T) {
	now := time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)
	parent, cancel := context.WithDeadline(context.Background(), now.Add(time.Minute))
	defer cancel()
	ctx, cancelCall := toolCallContext(parent, "tunnel", now)
	defer cancelCall()
	deadline, ok := ctx.Deadline()
	if !ok || !deadline.Equal(now.Add(55*time.Second)) {
		t.Fatalf("deadline=%v ok=%t want=%v", deadline, ok, now.Add(55*time.Second))
	}
}

func TestToolCallContextLeavesNonTunnelDeadlineUntouched(t *testing.T) {
	now := time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)
	want := now.Add(20 * time.Second)
	parent, cancel := context.WithDeadline(context.Background(), want)
	defer cancel()
	ctx, cancelCall := toolCallContext(parent, "http", now)
	defer cancelCall()
	deadline, ok := ctx.Deadline()
	if !ok || !deadline.Equal(want) {
		t.Fatalf("deadline=%v ok=%t want=%v", deadline, ok, want)
	}
}

func TestTunnelResponseBudgetErrorGuidesLongRunCommand(t *testing.T) {
	err := tunnelResponseBudgetError("run_command")
	for _, want := range []string{"start_process", "process_status", "process_output"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

func TestToolCallContextUsesTunnelBudgetCause(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	ctx, cancelCall := toolCallContext(parent, "tunnel", time.Now())
	defer cancelCall()
	<-ctx.Done()
	if cause := context.Cause(ctx); cause != errTunnelResponseBudgetExceeded {
		t.Fatalf("cause=%v want=%v", cause, errTunnelResponseBudgetExceeded)
	}
}
