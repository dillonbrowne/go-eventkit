package middleware

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestAudit_SkipsReads(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	h := Audit(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			buf.Reset()
			r := httptest.NewRequest(method, "/x", nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if got := buf.String(); strings.Contains(got, "audit") {
				t.Errorf("expected no audit log for %s; got %q", method, got)
			}
		})
	}
}

func TestAudit_LogsMutations(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	h := Audit(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	r := httptest.NewRequest(http.MethodPost, "/v1/events?x=1", strings.NewReader(""))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	got := buf.String()
	for _, want := range []string{"audit", "method=POST", "path=/v1/events", "status=201", "elapsed=", `query="x=1"`} {
		if !strings.Contains(got, want) {
			t.Errorf("log missing %q; full: %s", want, got)
		}
	}
}

func TestAudit_IncludesRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	h := RequestID(Audit(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Header.Set(RequestIDHeader, "test-request-id")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !strings.Contains(buf.String(), "request_id=test-request-id") {
		t.Errorf("log missing request id; got %q", buf.String())
	}
}

func TestAudit_StatusWriterIdempotent(t *testing.T) {
	sw := &statusWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	sw.WriteHeader(http.StatusTeapot)
	sw.WriteHeader(http.StatusInternalServerError)
	if sw.status != http.StatusTeapot {
		t.Errorf("status = %d, want %d (only the first WriteHeader counts)", sw.status, http.StatusTeapot)
	}
}

func TestAudit_TrimQueryShort(t *testing.T) {
	out := trimQuery("a=1&b=2")
	if out != "a=1&b=2" {
		t.Errorf("got %q, want unchanged", out)
	}
}

func TestAudit_TrimQueryLong(t *testing.T) {
	long := strings.Repeat("a", 300)
	out := trimQuery(long)
	if len(out) <= 256 {
		t.Errorf("len(out) = %d, want > 256", len(out))
	}
	if !strings.HasSuffix(out, "…") {
		t.Errorf("trimmed query should end with ellipsis; got %q", out[len(out)-5:])
	}
}

func TestAudit_ConcurrentSafe(t *testing.T) {
	var buf bytes.Buffer
	// Wrap buf in a mutex so concurrent slog writes don't corrupt the test
	// log; we are testing the *middleware's* concurrent safety, not the
	// log sink's.
	logger := slog.New(slog.NewTextHandler(&safeBuf{w: &buf}, nil))
	h := Audit(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest(http.MethodPost, "/x", nil).WithContext(context.Background())
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
		}()
	}
	wg.Wait()
	if c := strings.Count(buf.String(), "audit"); c != 50 {
		t.Errorf("expected 50 audit lines, got %d", c)
	}
}

type safeBuf struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
