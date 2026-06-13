package tools

import (
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dillonbrowne/go-eventkit/mcp/client"
)

// Register installs every eventkit tool onto the given MCP server. Add
// a new tool by writing its definition in the appropriate per-resource
// file and appending a `register*` call here. There is no other place
// where the tool list is enumerated, so this function is the canonical
// answer to "what does eventkit-mcp expose?".
func Register(s *mcp.Server, c *client.Client) {
	// Calendars (5)
	registerListCalendars(s, c)
	registerGetCalendar(s, c)
	registerCreateCalendar(s, c)
	registerUpdateCalendar(s, c)
	registerDeleteCalendar(s, c)

	// Events (6)
	registerListEvents(s, c)
	registerGetEvent(s, c)
	registerCreateEvent(s, c)
	registerUpdateEvent(s, c)
	registerDeleteEvent(s, c)
	registerBatchDeleteEvents(s, c)

	// Reminder lists (5)
	registerListReminderLists(s, c)
	registerGetReminderList(s, c)
	registerCreateReminderList(s, c)
	registerUpdateReminderList(s, c)
	registerDeleteReminderList(s, c)

	// Reminders (8)
	registerListReminders(s, c)
	registerGetReminder(s, c)
	registerCreateReminder(s, c)
	registerUpdateReminder(s, c)
	registerDeleteReminder(s, c)
	registerCompleteReminder(s, c)
	registerUncompleteReminder(s, c)
	registerBatchDeleteReminders(s, c)

	// ChatGPT Deep Research compatibility (2): search + fetch.
	registerSearch(s, c)
	registerFetch(s, c)
}

// addTool wraps mcp.AddTool, defaulting every tool's input schema to the
// strict-compatible form (see strictInput) unless the caller set one
// explicitly. This is the single place input schemas are made compatible
// with both OpenAI (ChatGPT) and Anthropic (Claude) MCP clients.
func addTool[In, Out any](s *mcp.Server, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	if t.InputSchema == nil {
		t.InputSchema = strictInput[In]()
	}
	mcp.AddTool(s, t, h)
}
