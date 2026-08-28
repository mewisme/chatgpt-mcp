package activity

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestStreamRecentIsBoundedAndOrdered(t *testing.T) {
	stream := NewStream()
	stream.maxRecent = 3
	for _, message := range []string{"a", "b", "c", "d"} {
		stream.Publish(Event{Kind: string(EventSystem), Message: message})
	}
	recent := stream.Recent(10)
	if len(recent) != 3 || recent[0].Message != "b" || recent[2].Message != "d" {
		t.Fatalf("recent = %#v", recent)
	}
	for _, event := range recent {
		if event.Timestamp.IsZero() || event.Timestamp.Location() != time.UTC {
			t.Fatalf("timestamp not normalized: %#v", event.Timestamp)
		}
	}
}

func TestSubscribeWithRecentDoesNotReplayFutureEvent(t *testing.T) {
	stream := NewStream()
	stream.Publish(Event{Message: "before"})
	ch, recent := stream.SubscribeWithRecent(10)
	defer stream.Unsubscribe(ch)
	if len(recent) != 1 || recent[0].Message != "before" {
		t.Fatalf("recent = %#v", recent)
	}
	stream.Publish(Event{Message: "after"})
	select {
	case event := <-ch:
		if event.Message != "after" {
			t.Fatalf("event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live event")
	}
}

func TestHistoryLimit(t *testing.T) {
	for raw, want := range map[string]int{"": 100, "0": 0, "10": 10, "999": 200, "bad": 100, "-1": 100} {
		request := httptest.NewRequest("GET", "/?history="+raw, nil)
		if got := historyLimit(request); got != want {
			t.Fatalf("history=%q: got %d want %d", raw, got, want)
		}
	}
}
