package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSPAHandlerServesIndexWithoutRedirect(t *testing.T) {
	handler := Handler()
	for _, target := range []string{"/", "/index.html", "/settings"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", target, recorder.Code)
		}
		if location := recorder.Header().Get("Location"); location != "" {
			t.Fatalf("%s: unexpected redirect to %s", target, location)
		}
	}
}
