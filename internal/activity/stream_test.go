package activity

import (
	"context"
	"net/http/httptest"
	"strings"
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
	if recent[0].Sequence != 2 || recent[1].Sequence != 3 || recent[2].Sequence != 4 || stream.LatestSequence() != 4 {
		t.Fatalf("sequences = %#v latest=%d", recent, stream.LatestSequence())
	}
	for _, event := range recent {
		if event.Timestamp.IsZero() || event.Timestamp.Location() != time.UTC {
			t.Fatalf("timestamp not normalized: %#v", event.Timestamp)
		}
	}
}

func TestSlowSubscriberReceivesOverflowSignal(t *testing.T) {
	stream := NewStream()
	sub, _ := stream.SubscribeDetailed(0)
	defer stream.UnsubscribeDetailed(sub)
	for index := 0; index <= defaultSubscriberBuffer; index++ {
		stream.Publish(Event{Kind: string(EventSystem), Message: "event"})
	}
	select {
	case overflow := <-sub.Overflow:
		if overflow.DroppedSequence != defaultSubscriberBuffer+1 {
			t.Fatalf("dropped sequence = %d", overflow.DroppedSequence)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for overflow signal")
	}
	before := len(sub.Events)
	stream.Publish(Event{Kind: string(EventSystem), Message: "ignored after overflow"})
	if len(sub.Events) != before {
		t.Fatalf("overflowed subscriber kept receiving events: before=%d after=%d", before, len(sub.Events))
	}
}

func TestSSEEmitsReadyHeartbeatAndEventIDs(t *testing.T) {
	stream := NewStream()
	stream.Publish(Event{Kind: string(EventSystem), Message: "before"})
	ctx, cancel := context.WithCancel(context.Background())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/?history=10", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		handlerWithHeartbeat(stream, 5*time.Millisecond).ServeHTTP(recorder, request)
		close(done)
	}()
	time.Sleep(25 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not stop after cancellation")
	}
	body := recorder.Body.String()
	for _, expected := range []string{"id: 1\n", "event: activity\n", "event: ready\n", "event: heartbeat\n", `"latest_sequence":1`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("SSE body missing %q: %q", expected, body)
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
