package subscription

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// WaitlistStore persists "notify me when Premium launches" emails as JSONL.
// Deliberately separate from Store: waitlist entries are pre-signup leads,
// not user accounts (fix manifest #C05).
type WaitlistStore struct {
	path string
	mu   sync.Mutex
}

// NewWaitlistStore creates a JSONL-backed waitlist store at path.
func NewWaitlistStore(path string) *WaitlistStore {
	return &WaitlistStore{path: path}
}

type waitlistEntry struct {
	Email     string    `json:"email"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

// Add appends email unless already present (case-insensitive).
// Returns already=true when the email was previously registered.
func (s *WaitlistStore) Add(email, source string) (already bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen(email) {
		return true, nil
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return false, err
	}
	defer f.Close()
	rec := waitlistEntry{Email: email, Source: source, CreatedAt: time.Now().UTC()}
	if err := json.NewEncoder(f).Encode(rec); err != nil {
		return false, err
	}
	return false, nil
}

// Count returns the number of registered entries (for observability/tests).
func (s *WaitlistStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	s.scan(func(waitlistEntry) { n++ })
	return n
}

func (s *WaitlistStore) seen(email string) bool {
	found := false
	s.scan(func(e waitlistEntry) {
		if strings.EqualFold(e.Email, email) {
			found = true
		}
	})
	return found
}

func (s *WaitlistStore) scan(fn func(waitlistEntry)) {
	f, err := os.Open(s.path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e waitlistEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err == nil {
			fn(e)
		}
	}
}

// WithWaitlist enables the POST /api/waitlist endpoint backed by a JSONL
// file at path. Chained on NewHandler in cmd/atlas/main.go.
func (h *Handler) WithWaitlist(path string) *Handler {
	h.waitlist = NewWaitlistStore(path)
	return h
}

// handleWaitlist collects pre-launch Premium interest emails. Public
// endpoint (no auth): visitors are guests by definition here.
func (h *Handler) handleWaitlist(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		Email  string `json:"email"`
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(req.Email)
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || len(email) > 254 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email"})
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "premium"
	}
	if len(source) > 32 {
		source = source[:32]
	}
	already, err := h.waitlist.Add(email, source)
	if err != nil {
		logging.Error("subscription", "waitlist_add_failed", "error", err.Error())
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	logging.Info("subscription", "waitlist_added", "email", email, "source", source, "already", already)
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "already_registered": already})
}
