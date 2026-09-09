package trace

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoHTTPTracesResponseBytesStatusAndRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirect" {
			http.Redirect(writer, request, "/final", http.StatusFound)
			return
		}
		_, _ = io.WriteString(writer, "hello")
	}))
	defer server.Close()
	events := []Event{}
	ctx := WithObserver(context.Background(), func(event Event) { events = append(events, event) })
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/redirect?access_token=secret&safe=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := DoHTTP(server.Client(), request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if string(data) != "hello" || len(events) != 2 {
		t.Fatalf("data=%q events=%#v", data, events)
	}
	completed := events[1]
	if value, _ := fieldValue(completed, "status"); value != http.StatusOK {
		t.Fatalf("status=%v", value)
	}
	if value, _ := fieldValue(completed, "bytes_read"); value != int64(5) {
		t.Fatalf("bytes_read=%v", value)
	}
	if value, _ := fieldValue(completed, "redirect_count"); value != 1 {
		t.Fatalf("redirect_count=%v", value)
	}
	if _, ok := fieldValue(completed, "duration_ms"); !ok {
		t.Fatal("response missing duration_ms")
	}
	if _, ok := fieldValue(completed, "ttfb_ms"); !ok {
		t.Fatal("response missing ttfb_ms")
	}
	startURL, _ := fieldValue(events[0], "url")
	if strings.Contains(startURL.(string), "secret") {
		t.Fatalf("start URL leaked credential: %s", startURL)
	}
}

func TestDoHTTPDoesNotTraceSensitiveHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(writer, "ok") }))
	defer server.Close()
	events := []Event{}
	ctx := WithObserver(context.Background(), func(event Event) { events = append(events, event) })
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	request.Header.Set("Authorization", "Bearer super-secret")
	response, err := DoHTTP(server.Client(), request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	for _, event := range events {
		for _, field := range event.Fields {
			if strings.Contains(strings.ToLower(field.Key), "authorization") || strings.Contains(strings.ToLower(strings.TrimSpace(fieldString(field.Value))), "super-secret") {
				t.Fatalf("trace leaked header: %#v", event)
			}
		}
	}
}

func fieldString(value any) string {
	text, _ := value.(string)
	return text
}

func TestDoHTTPFailureIncludesTiming(t *testing.T) {
	events := []Event{}
	ctx := WithObserver(context.Background(), func(event Event) { events = append(events, event) })
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:1/fail?access_token=secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{}
	if _, err := DoHTTP(client, request); err == nil {
		t.Fatal("expected request failure")
	}
	if len(events) != 2 || events[1].Phase != PhaseError {
		t.Fatalf("events=%#v", events)
	}
	if _, ok := fieldValue(events[1], "duration_ms"); !ok {
		t.Fatal("failure missing duration_ms")
	}
	if method, _ := fieldValue(events[1], "method"); method != http.MethodGet {
		t.Fatalf("failure method=%v", method)
	}
	if destination, ok := fieldValue(events[1], "url"); !ok || !strings.Contains(destination.(string), "/fail") || strings.Contains(destination.(string), "secret") {
		t.Fatalf("failure destination=%v", destination)
	}
}

func TestDoHTTPReportsConnectionReuse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(writer, "ok") }))
	defer server.Close()
	client := server.Client()
	for attempt := 0; attempt < 2; attempt++ {
		events := []Event{}
		ctx := WithObserver(context.Background(), func(event Event) { events = append(events, event) })
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		response, err := DoHTTP(client, request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if attempt == 1 {
			value, _ := fieldValue(events[1], "reused_connection")
			if value != true {
				t.Fatalf("expected reused connection, got %v", value)
			}
		}
	}
}
