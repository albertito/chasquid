package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/config"
)

func TestMonitoringHandler(t *testing.T) {
	conf := &config.Config{Hostname: "testhost"}
	h := monitoringHandler(http.NewServeMux(), &http.Server{}, conf)

	cases := []struct {
		method string
		path   string
		header map[string]string
		want   int
	}{
		{"GET", "/", nil, http.StatusOK},
		{"HEAD", "/", nil, http.StatusOK},
		{"POST", "/", nil, http.StatusMethodNotAllowed},
		{"GET", "/doesnotexist", nil, http.StatusNotFound},
		{"GET", "/metrics", nil, http.StatusOK},
		{"GET", "/debug/flags", nil, http.StatusOK},
		{"GET", "/debug/config", nil, http.StatusOK},
		{"POST", "/debug/config", nil, http.StatusMethodNotAllowed},
		{"GET", "/debug/traces", nil, http.StatusOK},

		// /exit is only allowed via POST.
		{"GET", "/exit", nil, http.StatusMethodNotAllowed},

		// Cross-origin requests from browsers must be rejected, to prevent
		// CSRF. They must not reach the handler, which would exit.
		{"POST", "/exit",
			map[string]string{"Sec-Fetch-Site": "cross-site"},
			http.StatusForbidden},
		{"POST", "/exit",
			map[string]string{"Origin": "https://evil.example"},
			http.StatusForbidden},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		for k, v := range c.header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s %s %v: got status %d, expected %d",
				c.method, c.path, c.header, rec.Code, c.want)
		}
	}
}
