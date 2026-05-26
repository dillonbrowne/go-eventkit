package middleware

import (
	"bytes"
	"net/http"
	"sync"
	"time"
)

// IdempotencyHeader is the inbound header that identifies a logical
// request. Two requests with the same (method, path, idempotency-key)
// within the configured window are treated as replays of the same
// operation — the second one returns the cached response instead of
// re-executing the handler.
const IdempotencyHeader = "Idempotency-Key"

// IdempotencyReplayedHeader is set on cached replay responses so the
// caller can distinguish "this happened just now" from "this happened
// previously and you're seeing the prior result."
const IdempotencyReplayedHeader = "Idempotency-Replayed"

// Idempotency returns a middleware that caches responses keyed by an
// inbound Idempotency-Key header. Only POST, PATCH, and DELETE are
// cached — GET/HEAD are already idempotent and PUT is rarely used here.
// Responses outside 2xx are not cached: a transient 5xx should not
// poison the cache for subsequent retries.
//
// The cache is in-memory, capped at maxEntries (LRU-evicted), and each
// entry expires after window. Pass window=0 to use a 24h default;
// maxEntries=0 disables the middleware entirely (pass-through).
func Idempotency(window time.Duration, maxEntries int) func(http.Handler) http.Handler {
	if maxEntries <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	c := newIdemCache(maxEntries, window)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPatch, http.MethodDelete:
			default:
				next.ServeHTTP(w, r)
				return
			}
			key := r.Header.Get(IdempotencyHeader)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}
			cacheKey := r.Method + " " + r.URL.Path + " " + key
			if cached, ok := c.get(cacheKey); ok {
				replay(w, cached)
				return
			}
			rec := &recordingWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			if rec.status >= 200 && rec.status < 300 {
				c.put(cacheKey, &idemEntry{
					status:  rec.status,
					body:    append([]byte(nil), rec.body.Bytes()...),
					headers: cloneHeader(rec.Header()),
				})
			}
		})
	}
}

// recordingWriter captures status, body, and headers as the handler
// runs, then proxies them through to the real response writer.
type recordingWriter struct {
	http.ResponseWriter
	status  int
	body    bytes.Buffer
	wrote   bool
	headers http.Header
}

func (r *recordingWriter) WriteHeader(code int) {
	if !r.wrote {
		r.status = code
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recordingWriter) Write(b []byte) (int, error) {
	if !r.wrote {
		r.wrote = true
	}
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func replay(w http.ResponseWriter, e *idemEntry) {
	for k, v := range e.headers {
		// Strip Content-Length — we'll let the response writer recompute
		// it. (Setting CL to a stale value will confuse some clients.)
		if k == "Content-Length" {
			continue
		}
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.Header().Set(IdempotencyReplayedHeader, "true")
	w.WriteHeader(e.status)
	_, _ = w.Write(e.body)
}

func cloneHeader(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, v := range h {
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

// idemEntry is one cached response.
type idemEntry struct {
	status    int
	body      []byte
	headers   http.Header
	expiresAt time.Time
}

// idemCache is a simple capacity-bounded LRU with absolute expiry.
// We don't bother with a heap — at the small sizes we run, a linear
// sweep is fine and the code stays under 50 lines.
type idemCache struct {
	mu      sync.Mutex
	entries map[string]*idemEntry
	order   []string // insertion order, head is oldest
	cap     int
	window  time.Duration
}

func newIdemCache(capacity int, window time.Duration) *idemCache {
	return &idemCache{
		entries: make(map[string]*idemEntry, capacity),
		order:   make([]string, 0, capacity),
		cap:     capacity,
		window:  window,
	}
}

func (c *idemCache) get(key string) (*idemEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expiresAt) {
		c.evict(key)
		return nil, false
	}
	return e, true
}

func (c *idemCache) put(key string, e *idemEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e.expiresAt = time.Now().Add(c.window)
	if _, exists := c.entries[key]; exists {
		c.entries[key] = e
		return
	}
	if len(c.entries) >= c.cap {
		c.evict(c.order[0])
	}
	c.entries[key] = e
	c.order = append(c.order, key)
}

// evict removes a key. Caller must hold c.mu.
func (c *idemCache) evict(key string) {
	delete(c.entries, key)
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}
