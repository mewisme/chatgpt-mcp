package trace

import (
	"context"
	"errors"
	"testing"
)

func TestSpanSuccessIncludesDuration(t *testing.T) {
	events := []Event{}
	ctx := WithObserver(context.Background(), func(event Event) { events = append(events, event) })
	span := Start(ctx, "UPDATE", "update.release.request", "Fetching release metadata", String("method", "GET"))
	span.End(Int("status", 200))
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Name != "update.release.request.started" || events[1].Name != "update.release.request.completed" {
		t.Fatalf("unexpected event names: %q %q", events[0].Name, events[1].Name)
	}
	if _, ok := fieldValue(events[1], "duration_ms"); !ok {
		t.Fatal("completion event missing duration_ms")
	}
}

func TestSpanFailureIncludesDurationAndError(t *testing.T) {
	events := []Event{}
	span := StartObserver(func(event Event) { events = append(events, event) }, "UPDATE", "update.download", "Downloading release")
	span.Fail(errors.New("network failed"))
	if len(events) != 2 || events[1].Phase != PhaseError {
		t.Fatalf("unexpected events: %#v", events)
	}
	if value, ok := fieldValue(events[1], "error"); !ok || value != "network failed" {
		t.Fatalf("unexpected error field: %v %t", value, ok)
	}
	if _, ok := fieldValue(events[1], "duration_ms"); !ok {
		t.Fatal("failure event missing duration_ms")
	}
}

func TestNoopObserverIsSafe(t *testing.T) {
	span := Start(context.Background(), "TEST", "noop", "No-op")
	span.End()
	Emit(context.Background(), "TEST", "noop.info", "No-op")
}

func TestSensitiveFieldsAreRedacted(t *testing.T) {
	events := []Event{}
	EmitObserver(func(event Event) { events = append(events, event) }, "AUTH", "auth.test", "test", String("access_token", "secret-value"), String("name", "safe"))
	if value, _ := fieldValue(events[0], "access_token"); value != "configured" {
		t.Fatalf("secret was not redacted: %v", value)
	}
	if value, _ := fieldValue(events[0], "name"); value != "safe" {
		t.Fatalf("safe value changed: %v", value)
	}
}

func TestSanitizeURL(t *testing.T) {
	value := SanitizeURL("https://user:pass@example.com/file?token=abc&x=1&X-Amz-Signature=secret")
	if value != "https://example.com/file?X-Amz-Signature=%3Credacted%3E&token=%3Credacted%3E&x=1" {
		t.Fatalf("unexpected sanitized URL: %s", value)
	}
}

func TestURLFieldSanitizesCredentialQuery(t *testing.T) {
	events := []Event{}
	EmitObserver(func(event Event) { events = append(events, event) }, "HTTP", "http.test", "test", URL("url", "https://example.com/callback?access_token=abc&state=def&safe=1"))
	value, _ := fieldValue(events[0], "url")
	if value == "https://example.com/callback?access_token=abc&state=def&safe=1" {
		t.Fatalf("URL was not sanitized: %v", value)
	}
}

func fieldValue(event Event, key string) (any, bool) {
	for _, field := range event.Fields {
		if field.Key == key {
			return field.Value, true
		}
	}
	return nil, false
}
