package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchRepoStarDatesUsesAggregateHistory(t *testing.T) {
	requests := 0
	base := time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/repos/me/tool/stargazers/history" || r.URL.Query().Get("per_page") != "30" || r.Header.Get("Accept") != "application/vnd.github+json" {
			http.Error(w, "unexpected request", http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("page") == "1" {
			weeks := make([]map[string]any, 30)
			for i := range weeks {
				weeks[i] = map[string]any{"week": base.AddDate(0, 0, (30-i)*7).Unix(), "total": 0, "days": []int{0, 0, 0, 0, 0, 0, 0}}
			}
			weeks[0]["days"] = []int{1, 0, 0, 0, 0, 0, 0}
			weeks[0]["total"] = 1
			_ = json.NewEncoder(w).Encode(weeks)
		} else if r.URL.Query().Get("page") == "2" {
			_, _ = fmt.Fprintf(w, `[{"week":%d,"total":3,"days":[0,2,0,0,0,0,1]}]`, base.Unix())
		} else {
			http.Error(w, "unexpected page", http.StatusForbidden)
		}
	}))
	defer srv.Close()
	old := restEndpoint
	restEndpoint = srv.URL
	t.Cleanup(func() { restEndpoint = old })
	dates, err := fetchRepoStarDates(context.Background(), srv.Client(), "metadata-read-token", "me", "tool")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(dates) != 4 {
		t.Fatalf("requests=%d dates=%v", requests, dates)
	}
	want := []time.Time{base.AddDate(0, 0, 210), base.AddDate(0, 0, 1), base.AddDate(0, 0, 1), base.AddDate(0, 0, 6)}
	for i := range want {
		if !dates[i].Equal(want[i]) {
			t.Errorf("date %d = %v, want %v", i, dates[i], want[i])
		}
	}
}
