// Package client is a typed HTTP client for the eventkit REST API.
// The MCP server uses it to translate tool calls into REST requests.
//
// Each REST OperationID has one method here with strongly-typed input
// and output. The client maps REST 4xx/5xx into typed Go errors so the
// MCP layer can render them as JSON-RPC errors with the right shape.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to a running eventkit-server REST API.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a Client pointed at the given REST base URL with the
// supplied per-request timeout. Pass 0 for the timeout to use http.Client's
// default (no timeout).
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: timeout},
	}
}

// Error wraps a non-2xx REST response. The MCP layer can match on Status
// to render appropriate JSON-RPC error codes.
type Error struct {
	Status int
	Body   string
}

func (e *Error) Error() string {
	return fmt.Sprintf("eventkit REST: %d %s: %s", e.Status, http.StatusText(e.Status), e.Body)
}

// ErrNotFound matches REST 404 (including policy denials).
var ErrNotFound = errors.New("not found")

// IsNotFound returns true if err is a wrapped REST 404.
func IsNotFound(err error) bool {
	var re *Error
	if errors.As(err, &re) && re.Status == http.StatusNotFound {
		return true
	}
	return errors.Is(err, ErrNotFound)
}

// do executes an HTTP request, JSON-encoding `in` (if non-nil) and
// JSON-decoding the response into `out` (if non-nil). Returns an *Error
// for non-2xx responses.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Capped read so an unbounded error body can't blow up memory.
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return &Error{Status: resp.StatusCode, Body: strings.TrimSpace(string(raw))}
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
