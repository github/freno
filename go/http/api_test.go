package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLbCheck(t *testing.T) {
	api := new(APIImpl)
	recorder := httptest.NewRecorder()
	api.LbCheck(recorder, nil, nil)

	code, body := recorder.Code, recorder.Body.String()

	if code != http.StatusOK {
		t.Errorf("Expected LbCheck to respond with %d status code, but responded with %d", http.StatusOK, code)
	}

	if len(body) > 0 {
		t.Errorf("Expected LbCheck to respond with empty body, but responded with %s", body)
	}
}

// TestRoutes applies an end-to-end canary test over each of the different routes
func TestRoutes(t *testing.T) {
	router := ConfigureRoutes(new(APIImpl))

	expectedRoutes := []struct {
		verb string
		path string
		code int
	}{
		{http.MethodGet, "/lb-check", http.StatusOK},
	}
	for _, route := range expectedRoutes {
		r, _ := http.NewRequest(route.verb, route.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if !(w.Code == route.code) {
			t.Errorf("Route %s failed: code {expected=%d, actual=%d}", route.path, route.code, w.Code)
		}
	}
}
