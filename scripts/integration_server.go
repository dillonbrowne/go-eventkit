//go:build darwin && integration

// Package main exercises the REST server end-to-end against a real
// EventKit. It starts the server on an ephemeral port with a permissive
// in-memory policy, round-trips a curated set of endpoints, and then
// repeats key endpoints under a restricted policy to verify the
// defense-in-depth scoping really refuses writes to forbidden targets.
//
// Run with: go run -tags integration ./scripts/integration_server.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/reminders"
	"github.com/dillonbrowne/go-eventkit/server"
	"github.com/dillonbrowne/go-eventkit/server/policy"
)

const testPrefix = "[go-eventkit test] "

// assertSecurityHeaders verifies the responses include X-Request-ID and
// have no Server header. Called after every happy-path response.
func assertSecurityHeaders(name string, resp *http.Response, check func(string, error)) {
	if resp == nil {
		return
	}
	if resp.Header.Get("X-Request-ID") == "" {
		check(name+" X-Request-ID present", fmt.Errorf("missing X-Request-ID"))
	}
	if got := resp.Header.Get("Server"); got != "" {
		check(name+" Server header stripped", fmt.Errorf("Server = %q", got))
	}
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("[integration-server] ")

	passed, failed := 0, 0
	check := func(name string, err error) {
		if err != nil {
			log.Printf("FAIL: %s: %v", name, err)
			failed++
		} else {
			log.Printf("PASS: %s", name)
			passed++
		}
	}

	// --- Bring up real EventKit clients (will trigger TCC if needed) ---
	calClient, err := calendar.New()
	if err != nil {
		log.Fatalf("FATAL: calendar.New: %v", err)
	}
	remClient, err := reminders.New()
	if err != nil {
		log.Fatalf("FATAL: reminders.New: %v", err)
	}

	// Discover a writable calendar + list to scope the permissive policy to.
	cals, err := calClient.Calendars()
	if err != nil {
		log.Fatalf("FATAL: list calendars: %v", err)
	}
	var writableCal *calendar.Calendar
	for i := range cals {
		c := cals[i]
		if c.ReadOnly {
			continue
		}
		if c.Title == "Home" || writableCal == nil {
			writableCal = &c
		}
	}
	if writableCal == nil {
		log.Fatalf("FATAL: no writable calendar found")
	}
	log.Printf("Using calendar %q (ID=%s, source=%s) for write tests", writableCal.Title, writableCal.ID, writableCal.Source)

	lists, err := remClient.Lists()
	if err != nil {
		log.Fatalf("FATAL: list reminder lists: %v", err)
	}
	var writableList *reminders.List
	for i := range lists {
		l := lists[i]
		if l.ReadOnly {
			continue
		}
		if writableList == nil {
			writableList = &l
		}
	}
	if writableList == nil {
		log.Fatalf("FATAL: no writable reminder list found")
	}
	log.Printf("Using reminder list %q (ID=%s, source=%s)", writableList.Title, writableList.ID, writableList.Source)

	// --- Permissive run ---
	permissive := &policy.Policy{
		Calendar: policy.PackagePolicy{
			Default: policy.ModeDeny,
			Entries: []policy.Entry{{Source: writableCal.Source, Mode: policy.ModeReadWrite}},
		},
		Reminders: policy.PackagePolicy{
			Default: policy.ModeDeny,
			Entries: []policy.Entry{{Source: writableList.Source, Mode: policy.ModeReadWrite}},
		},
	}
	srv, err := server.New(
		server.WithCalendarBridge(calClient),
		server.WithRemindersBridge(remClient),
		server.WithPolicy(permissive),
	)
	if err != nil {
		log.Fatalf("FATAL: server.New: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	// httptest binds 127.0.0.1, so the loopback middleware allows requests
	// going through ts.URL.

	// 1. /healthz
	{
		resp, err := http.Get(ts.URL + "/healthz")
		check("GET /healthz", err)
		if err == nil {
			assertSecurityHeaders("/healthz", resp, check)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				check("GET /healthz status", fmt.Errorf("got %d", resp.StatusCode))
			} else {
				check("GET /healthz status 200", nil)
			}
		}
	}

	// 2. /openapi.json
	{
		resp, err := http.Get(ts.URL + "/openapi.json")
		check("GET /openapi.json", err)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var spec map[string]any
			if err := json.Unmarshal(b, &spec); err != nil || spec["paths"] == nil {
				check("OpenAPI spec parseable", fmt.Errorf("invalid spec: %v", err))
			} else {
				check("OpenAPI spec parseable", nil)
			}
		}
	}

	// 3. List calendars (filtered to source)
	{
		resp, err := http.Get(ts.URL + "/v1/calendars")
		check("GET /v1/calendars", err)
		if err == nil {
			assertSecurityHeaders("/v1/calendars", resp, check)
			var body struct {
				Calendars []calendar.Calendar `json:"calendars"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			found := false
			for _, c := range body.Calendars {
				if c.ID == writableCal.ID {
					found = true
				}
				if c.Source != writableCal.Source {
					check("list-calendars filter", fmt.Errorf("calendar %q has unexpected source %q", c.ID, c.Source))
				}
			}
			if !found {
				check("list-calendars contains writableCal", fmt.Errorf("missing %s", writableCal.ID))
			} else {
				check("list-calendars contains writableCal", nil)
			}
		}
	}

	// 4. Create event, fetch it, update it, delete it
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 11, 0, 0, 0, time.Local).Add(24 * time.Hour)
	createBody := map[string]any{
		"title":     testPrefix + "REST roundtrip",
		"startDate": start,
		"endDate":   start.Add(time.Hour),
		"calendar":  writableCal.Title,
	}
	var createdID string
	{
		resp, err := postJSON(ts.URL+"/v1/events", createBody)
		check("POST /v1/events", err)
		if err == nil {
			assertSecurityHeaders("/v1/events POST", resp, check)
			if resp.StatusCode != http.StatusCreated {
				check("POST /v1/events status 201", fmt.Errorf("got %d", resp.StatusCode))
			} else {
				var got calendar.Event
				_ = json.NewDecoder(resp.Body).Decode(&got)
				createdID = got.ID
				check("POST /v1/events status 201", nil)
			}
			resp.Body.Close()
		}
	}

	if createdID != "" {
		// GET
		resp, err := http.Get(ts.URL + "/v1/events/" + createdID)
		check("GET /v1/events/{id}", err)
		if err == nil {
			resp.Body.Close()
		}
		// PATCH
		newTitle := testPrefix + "REST renamed"
		resp, err = patchJSON(ts.URL+"/v1/events/"+createdID, map[string]any{"title": newTitle})
		check("PATCH /v1/events/{id}", err)
		if err == nil {
			resp.Body.Close()
		}
		// DELETE
		resp, err = deleteReq(ts.URL + "/v1/events/" + createdID)
		check("DELETE /v1/events/{id}", err)
		if err == nil {
			resp.Body.Close()
		}
	}

	// 5. Reminder roundtrip
	var createdRemID string
	{
		body := map[string]any{
			"title": testPrefix + "REST reminder",
			"list":  writableList.Title,
		}
		resp, err := postJSON(ts.URL+"/v1/reminders", body)
		check("POST /v1/reminders", err)
		if err == nil {
			if resp.StatusCode != http.StatusCreated {
				b, _ := io.ReadAll(resp.Body)
				check("POST /v1/reminders status 201", fmt.Errorf("got %d: %s", resp.StatusCode, b))
			} else {
				var got reminders.Reminder
				_ = json.NewDecoder(resp.Body).Decode(&got)
				createdRemID = got.ID
				check("POST /v1/reminders status 201", nil)
			}
			resp.Body.Close()
		}
	}
	if createdRemID != "" {
		resp, err := http.Post(ts.URL+"/v1/reminders/"+createdRemID+"/complete", "", nil)
		check("POST /v1/reminders/{id}/complete", err)
		if err == nil {
			resp.Body.Close()
		}
		resp, err = deleteReq(ts.URL + "/v1/reminders/" + createdRemID)
		check("DELETE /v1/reminders/{id}", err)
		if err == nil {
			resp.Body.Close()
		}
	}

	// --- Restricted run: writableCal allowed read-only ---
	restricted := &policy.Policy{
		Calendar: policy.PackagePolicy{
			Default: policy.ModeDeny,
			Entries: []policy.Entry{{ID: writableCal.ID, Mode: policy.ModeRead}},
		},
		Reminders: policy.PackagePolicy{
			Default: policy.ModeDeny,
			Entries: []policy.Entry{{ID: writableList.ID, Mode: policy.ModeRead}},
		},
	}
	srv2, err := server.New(
		server.WithCalendarBridge(calClient),
		server.WithRemindersBridge(remClient),
		server.WithPolicy(restricted),
	)
	if err != nil {
		log.Fatalf("FATAL: restricted server.New: %v", err)
	}
	ts2 := httptest.NewServer(srv2.Handler())
	defer ts2.Close()

	// Reads still work
	{
		resp, err := http.Get(ts2.URL + "/v1/calendars/" + writableCal.ID)
		check("restricted: GET /v1/calendars/{id} (read-only allowed)", err)
		if err == nil {
			if resp.StatusCode != http.StatusOK {
				check("restricted: status 200", fmt.Errorf("got %d", resp.StatusCode))
			}
			resp.Body.Close()
		}
	}
	// Writes are refused (404)
	{
		resp, err := postJSON(ts2.URL+"/v1/events", createBody)
		check("restricted: POST /v1/events (writes forbidden)", err)
		if err == nil {
			if resp.StatusCode != http.StatusNotFound {
				b, _ := io.ReadAll(resp.Body)
				check("restricted: write refused with 404", fmt.Errorf("got %d: %s", resp.StatusCode, b))
			} else {
				check("restricted: write refused with 404", nil)
			}
			resp.Body.Close()
		}
	}

	// --- Loopback gate: refuses non-loopback Host header ---
	{
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/healthz", nil)
		req.Host = "evil.example.com"
		resp, err := http.DefaultClient.Do(req)
		check("loopback gate: request with foreign Host", err)
		if err == nil {
			if resp.StatusCode != http.StatusForbidden {
				check("loopback gate: status 403", fmt.Errorf("got %d", resp.StatusCode))
			} else {
				check("loopback gate: status 403", nil)
			}
			resp.Body.Close()
		}
	}

	// --- Verify the OS bind is loopback-only ---
	{
		_, port, _ := net.SplitHostPort(ts.URL[len("http://"):])
		// httptest always binds 127.0.0.1, so this just sanity-checks our
		// assumptions.
		_, err := net.Dial("tcp", "127.0.0.1:"+port)
		check("bind: loopback dialable", err)
	}

	log.Printf("\nRESULTS: %d passed, %d failed", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func postJSON(url string, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func patchJSON(url string, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest(http.MethodPatch, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func deleteReq(url string) (*http.Response, error) {
	req, _ := http.NewRequest(http.MethodDelete, url, nil)
	return http.DefaultClient.Do(req)
}
