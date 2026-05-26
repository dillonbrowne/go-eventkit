package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/mcp"
	"github.com/dillonbrowne/go-eventkit/mcp/client"
	"github.com/dillonbrowne/go-eventkit/mcp/tools"
	"github.com/dillonbrowne/go-eventkit/reminders"
)

// fakeREST stands in for a running eventkit-server. Each handler returns
// a canned JSON response so we can exercise the MCP tools end-to-end
// without TCC, EventKit, or any cgo.
func fakeREST(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/calendars", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(struct {
			Calendars []calendar.Calendar `json:"calendars"`
		}{
			Calendars: []calendar.Calendar{{ID: "CAL-1", Title: "Home", Source: "iCloud"}},
		})
	})
	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(struct {
				Events []calendar.Event `json:"events"`
			}{
				Events: []calendar.Event{{ID: "EV-1", Title: "Stand-up", Calendar: "Home", CalendarID: "CAL-1",
					StartDate: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
					EndDate:   time.Date(2026, 1, 1, 9, 30, 0, 0, time.UTC)}},
			})
		case http.MethodPost:
			// Capture body for assertions.
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("X-Test-StartDate", body["startDate"].(string))
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(calendar.Event{
				ID: "EV-NEW", Title: body["title"].(string),
				Calendar: body["calendar"].(string), CalendarID: "CAL-1",
				StartDate: time.Now(), EndDate: time.Now().Add(time.Hour),
			})
		}
	})
	mux.HandleFunc("/v1/lists", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(struct {
			Lists []reminders.List `json:"lists"`
		}{
			Lists: []reminders.List{{ID: "L-1", Title: "Todo", Source: "iCloud"}},
		})
	})
	mux.HandleFunc("/v1/reminders", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(struct {
			Reminders []reminders.Reminder `json:"reminders"`
		}{
			Reminders: []reminders.Reminder{{ID: "R-1", Title: "Buy milk", List: "Todo", ListID: "L-1"}},
		})
	})
	return httptest.NewServer(mux)
}

// connect spins up the MCP server in-memory and returns a connected
// ClientSession ready for tool calls.
func connect(t *testing.T, apiBase string) *sdkmcp.ClientSession {
	t.Helper()
	srv := mcp.New(mcp.WithAPIBase(apiBase))
	_ = srv // handler not used; we hook the sdkmcp.Server directly via the in-memory transport

	mcpServer := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "eventkit-mcp", Version: "test"}, nil)
	tools.Register(mcpServer, client.New(apiBase, 5*time.Second))

	t1, t2 := sdkmcp.NewInMemoryTransports()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	if _, err := mcpServer.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	cli := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := cli.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestServer_ListToolsCount(t *testing.T) {
	rest := fakeREST(t)
	defer rest.Close()
	cs := connect(t, rest.URL)

	got, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(got.Tools) != 24 {
		t.Errorf("got %d tools, want 24", len(got.Tools))
	}
	// Spot-check a destructive tool and a read-only tool.
	names := map[string]*sdkmcp.Tool{}
	for _, tt := range got.Tools {
		names[tt.Name] = tt
	}
	if names["list_calendars"] == nil || !names["list_calendars"].Annotations.ReadOnlyHint {
		t.Errorf("list_calendars missing or not marked read-only")
	}
	if names["delete_event"] == nil || names["delete_event"].Annotations.DestructiveHint == nil || !*names["delete_event"].Annotations.DestructiveHint {
		t.Errorf("delete_event not marked destructive")
	}
}

func TestServer_ListCalendars_RoundTrip(t *testing.T) {
	rest := fakeREST(t)
	defer rest.Close()
	cs := connect(t, rest.URL)

	res, err := cs.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "list_calendars"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %+v", res.Content)
	}
	// json.Marshal escapes `<` / `>` as `<` / `>`; the SDK
	// passes through the encoded form. Decode-then-introspect lets us
	// match on the unescaped delimiter regardless of encoding choices.
	b, _ := json.Marshal(res.StructuredContent)
	var decoded struct {
		Calendars []struct {
			Title string `json:"title"`
		} `json:"calendars"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("decode: %v (raw: %s)", err, b)
	}
	if len(decoded.Calendars) != 1 {
		t.Fatalf("got %d calendars, want 1", len(decoded.Calendars))
	}
	if got := decoded.Calendars[0].Title; got != "<USER_DATA>Home</USER_DATA>" {
		t.Errorf("calendar title = %q, want <USER_DATA>Home</USER_DATA>", got)
	}
}

func TestServer_CreateEvent_NaturalDate(t *testing.T) {
	rest := fakeREST(t)
	defer rest.Close()
	cs := connect(t, rest.URL)

	res, err := cs.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name: "create_event",
		Arguments: map[string]any{
			"title":     "stand-up",
			"calendar":  "Home",
			"startDate": "tomorrow 9am",
			"endDate":   "tomorrow 9:30am",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %+v", res.Content)
	}
}

func TestServer_GetEvent_PropagatesNotFound(t *testing.T) {
	// REST mux that 404s every /v1/events/* request.
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/events/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "{\"detail\":\"not found\"}", http.StatusNotFound)
	})
	rest := httptest.NewServer(mux)
	defer rest.Close()

	cs := connect(t, rest.URL)
	res, err := cs.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      "get_event",
		Arguments: map[string]any{"id": "missing"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true on 404; got %+v", res.Content)
	}
}
