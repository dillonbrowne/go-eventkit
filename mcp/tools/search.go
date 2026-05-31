package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dillonbrowne/go-eventkit/calendar"
	"github.com/dillonbrowne/go-eventkit/mcp/client"
	"github.com/dillonbrowne/go-eventkit/reminders"
)

// search and fetch implement the two tools ChatGPT requires for its
// standard connector / Deep Research mode (it rejects MCP servers that
// lack them). The schemas below match OpenAI's reference implementation
// exactly:
//
//	search(query) -> { "results": [ {id, title, text, url} ] }
//	fetch(id)     -> { id, title, text, url, metadata }
//
// The go-sdk's typed AddTool fills both the JSON-RPC `structuredContent`
// (from the Out struct) and a mirrored `content` text block, which is
// the dual shape OpenAI expects — so we only define the Out structs.
//
// Unlike the other tools, search/fetch output is NOT wrapped in
// <USER_DATA> delimiters: ChatGPT treats it as citable document text.
// Content is still length-capped. See docs/prd/mcp-threats.md.

// searchWindow bounds the event query (events require a time range; the
// search tool has no date parameter). A year either side of "now" covers
// the overwhelming majority of "search my calendar" intents.
const searchWindow = 365 * 24 * time.Hour

// searchMaxResults caps the merged events+reminders result set.
const searchMaxResults = 50

// searchSnippetLen / fetchTextLen cap per-field text returned to the LLM.
const (
	searchSnippetLen = 280
	fetchTextLen     = 4000
)

// ---- search ----

type SearchInput struct {
	Query string `json:"query" jsonschema:"Text to match against your calendar events and reminders (title, location, notes)"`
}

// SearchResult is one hit. Field names (id,title,text,url) are mandated
// by OpenAI's search-tool contract.
type SearchResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
	URL   string `json:"url"`
}

type SearchOutput struct {
	Results []SearchResult `json:"results"`
}

func registerSearch(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:  "search",
		Title: "Search calendar & reminders",
		Description: "Search the user's calendar events and reminders by text. Returns a list of " +
			"results, each with an opaque id usable with the `fetch` tool to retrieve full details. " +
			"Required for ChatGPT Deep Research.",
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
		query := strings.TrimSpace(in.Query)
		var results []SearchResult

		now := time.Now()
		events, err := c.ListEvents(ctx, client.ListEventsInput{
			Start:  now.Add(-searchWindow),
			End:    now.Add(searchWindow),
			Search: query,
		})
		if err != nil {
			return nil, SearchOutput{}, err
		}
		for _, ev := range events {
			results = append(results, SearchResult{
				ID:    eventRefID(ev.ID),
				Title: ev.Title,
				Text:  truncate(eventSnippet(ev), searchSnippetLen),
				URL:   eventRefURL(ev.ID),
			})
			if len(results) >= searchMaxResults {
				break
			}
		}

		if len(results) < searchMaxResults {
			rems, err := c.ListReminders(ctx, client.ListRemindersInput{Search: query})
			if err != nil {
				return nil, SearchOutput{}, err
			}
			for _, r := range rems {
				results = append(results, SearchResult{
					ID:    reminderRefID(r.ID),
					Title: r.Title,
					Text:  truncate(reminderSnippet(r), searchSnippetLen),
					URL:   reminderRefURL(r.ID),
				})
				if len(results) >= searchMaxResults {
					break
				}
			}
		}

		// Always return a non-nil slice so structuredContent is {"results":[]}.
		if results == nil {
			results = []SearchResult{}
		}
		return nil, SearchOutput{Results: results}, nil
	})
}

// ---- fetch ----

type FetchInput struct {
	ID string `json:"id" jsonschema:"An id returned by the search tool (e.g. event:<id> or reminder:<id>)"`
}

// FetchOutput field names (id,title,text,url,metadata) are mandated by
// OpenAI's fetch-tool contract.
type FetchOutput struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Text     string         `json:"text"`
	URL      string         `json:"url"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func registerFetch(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:  "fetch",
		Title: "Fetch a calendar event or reminder",
		Description: "Retrieve the full details of a single event or reminder by the opaque id " +
			"returned from `search`. Required for ChatGPT Deep Research.",
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in FetchInput) (*mcp.CallToolResult, FetchOutput, error) {
		// Split on the FIRST colon only — event identifiers themselves
		// contain colons (e.g. "ABC123:DEF456").
		kind, raw, ok := strings.Cut(in.ID, ":")
		if !ok || raw == "" {
			return nil, FetchOutput{}, fmt.Errorf("invalid id %q: expected event:<id> or reminder:<id>", in.ID)
		}
		switch kind {
		case "event":
			ev, err := c.GetEvent(ctx, raw)
			if err != nil {
				return nil, FetchOutput{}, err
			}
			return nil, eventFetch(in.ID, *ev), nil
		case "reminder":
			r, err := c.GetReminder(ctx, raw)
			if err != nil {
				return nil, FetchOutput{}, err
			}
			return nil, reminderFetch(in.ID, *r), nil
		default:
			return nil, FetchOutput{}, fmt.Errorf("unknown id kind %q (want event or reminder)", kind)
		}
	})
}

// ---- id / url helpers ----

func eventRefID(id string) string    { return "event:" + id }
func reminderRefID(id string) string { return "reminder:" + id }

// eventkit has no web URL; the OpenAI contract requires a url for
// citation. We return an app-scheme URI that identifies the object. If a
// client demands http(s), this is the single place to change it.
func eventRefURL(id string) string    { return "eventkit://event/" + id }
func reminderRefURL(id string) string { return "eventkit://reminder/" + id }

// ---- text builders ----

func eventSnippet(e calendar.Event) string {
	var b strings.Builder
	b.WriteString(formatRange(e.StartDate, e.EndDate, e.AllDay))
	if e.Calendar != "" {
		fmt.Fprintf(&b, " · %s", e.Calendar)
	}
	if e.Location != "" {
		fmt.Fprintf(&b, " · %s", e.Location)
	}
	if e.Notes != "" {
		fmt.Fprintf(&b, " · %s", firstLine(e.Notes))
	}
	return b.String()
}

func eventFetch(refID string, e calendar.Event) FetchOutput {
	var b strings.Builder
	fmt.Fprintf(&b, "Event: %s\n", e.Title)
	fmt.Fprintf(&b, "When: %s\n", formatRange(e.StartDate, e.EndDate, e.AllDay))
	fmt.Fprintf(&b, "Calendar: %s\n", e.Calendar)
	if e.Location != "" {
		fmt.Fprintf(&b, "Location: %s\n", e.Location)
	}
	if e.URL != "" {
		fmt.Fprintf(&b, "URL: %s\n", e.URL)
	}
	if e.Recurring {
		b.WriteString("Recurring: yes\n")
	}
	if e.Notes != "" {
		fmt.Fprintf(&b, "Notes:\n%s\n", e.Notes)
	}
	md := map[string]any{
		"kind":       "event",
		"calendar":   e.Calendar,
		"calendarID": e.CalendarID,
		"allDay":     e.AllDay,
		"status":     e.Status.String(),
		"recurring":  e.Recurring,
	}
	return FetchOutput{
		ID:       refID,
		Title:    e.Title,
		Text:     truncate(strings.TrimRight(b.String(), "\n"), fetchTextLen),
		URL:      eventRefURL(e.ID),
		Metadata: md,
	}
}

func reminderSnippet(r reminders.Reminder) string {
	var b strings.Builder
	if r.Completed {
		b.WriteString("[done] ")
	}
	if r.DueDate != nil {
		fmt.Fprintf(&b, "due %s", r.DueDate.Format("Mon Jan 2 2006 15:04"))
	} else {
		b.WriteString("no due date")
	}
	if r.List != "" {
		fmt.Fprintf(&b, " · %s", r.List)
	}
	if r.Priority != reminders.PriorityNone {
		fmt.Fprintf(&b, " · %s priority", r.Priority.String())
	}
	if r.Notes != "" {
		fmt.Fprintf(&b, " · %s", firstLine(r.Notes))
	}
	return b.String()
}

func reminderFetch(refID string, r reminders.Reminder) FetchOutput {
	var b strings.Builder
	fmt.Fprintf(&b, "Reminder: %s\n", r.Title)
	fmt.Fprintf(&b, "List: %s\n", r.List)
	fmt.Fprintf(&b, "Completed: %t\n", r.Completed)
	if r.DueDate != nil {
		fmt.Fprintf(&b, "Due: %s\n", r.DueDate.Format(time.RFC1123))
	}
	if r.Priority != reminders.PriorityNone {
		fmt.Fprintf(&b, "Priority: %s\n", r.Priority.String())
	}
	if r.Flagged {
		b.WriteString("Flagged: yes\n")
	}
	if r.URL != "" {
		fmt.Fprintf(&b, "URL: %s\n", r.URL)
	}
	if r.Notes != "" {
		fmt.Fprintf(&b, "Notes:\n%s\n", r.Notes)
	}
	md := map[string]any{
		"kind":      "reminder",
		"list":      r.List,
		"listID":    r.ListID,
		"completed": r.Completed,
		"priority":  r.Priority.String(),
		"flagged":   r.Flagged,
	}
	return FetchOutput{
		ID:       refID,
		Title:    r.Title,
		Text:     truncate(strings.TrimRight(b.String(), "\n"), fetchTextLen),
		URL:      reminderRefURL(r.ID),
		Metadata: md,
	}
}

// formatRange renders a start/end pair compactly.
func formatRange(start, end time.Time, allDay bool) string {
	if start.IsZero() {
		return "(no date)"
	}
	if allDay {
		return start.Format("Mon Jan 2 2006") + " (all day)"
	}
	if end.IsZero() || end.Equal(start) {
		return start.Format("Mon Jan 2 2006 15:04")
	}
	if sameDay(start, end) {
		return fmt.Sprintf("%s–%s", start.Format("Mon Jan 2 2006 15:04"), end.Format("15:04"))
	}
	return fmt.Sprintf("%s – %s", start.Format("Mon Jan 2 2006 15:04"), end.Format("Mon Jan 2 2006 15:04"))
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// firstLine returns the first non-empty line of s, trimmed.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}
