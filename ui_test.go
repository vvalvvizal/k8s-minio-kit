package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndex(t *testing.T) {
	srv := &server{}
	recorder := httptest.NewRecorder()
	srv.index(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if !strings.Contains(recorder.Body.String(), "MinIO API Tester") {
		t.Fatal("response does not contain the UI")
	}
}

func TestIndexRejectsUnknownPathAndMethod(t *testing.T) {
	srv := &server{}

	unknown := httptest.NewRecorder()
	srv.index(unknown, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown path status = %d, want %d", unknown.Code, http.StatusNotFound)
	}

	post := httptest.NewRecorder()
	srv.index(post, httptest.NewRequest(http.MethodPost, "/", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want %d", post.Code, http.StatusMethodNotAllowed)
	}
}
