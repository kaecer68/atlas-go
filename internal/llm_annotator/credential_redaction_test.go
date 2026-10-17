package llm_annotator

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// annotatorRedactionFixtureKey is a fake credential shaped like the real thing.
// The value deliberately has NO provider-recognisable prefix (no "sk-"): the
// only rule that can remove it is the caller-supplied literal, so this probe
// fails if the literal is dropped even while the shape rule is intact.
const annotatorRedactionFixtureKey = "FAKEannotator4b7c1d2e3f4a5b6c7d8e"

// TestKimiClient_StatusError_HidesAPIKey is the negative proof for
// FU-20261016-13 on the annotator path: this error used to carry up to 1 MiB of
// provider body verbatim, and a 401 body quotes the key.
func TestKimiClient_StatusError_HidesAPIKey(t *testing.T) {
	// A body far larger than any sane error text, with the credential at the
	// front: both the leak and the unbounded size are asserted.
	bigBody := `{"error":{"message":"Incorrect API key provided: ` + annotatorRedactionFixtureKey + `"}}` +
		strings.Repeat("x", 4096)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); !strings.Contains(got, annotatorRedactionFixtureKey) {
			t.Errorf("fixture: expected the client to send its key, got %q", got)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(bigBody))
	}))
	defer srv.Close()

	client, err := NewKimiClient(Config{APIKey: annotatorRedactionFixtureKey, BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	_, err = client.Annotate(context.Background(), FailureContext{FrameID: "f1", Label: "test"})
	if err == nil {
		t.Fatal("expected an error for HTTP 401")
	}
	got := err.Error()
	t.Logf("annotator 401 -> %s", got)

	if strings.Contains(got, annotatorRedactionFixtureKey) {
		t.Errorf("annotator error leaks the provider credential: %s", got)
	}
	if !strings.Contains(got, "status 401") {
		t.Errorf("annotator error dropped the status: %s", got)
	}
	if !strings.Contains(got, "Incorrect API key provided") {
		t.Errorf("annotator error dropped the provider message skeleton: %s", got)
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("annotator error lost errors.Is(ErrUnavailable): %v", err)
	}
	// 512-byte cap plus the wrapper text: a 4 KiB body must not survive.
	if len(got) > 1024 {
		t.Errorf("annotator error is not size-capped (%d bytes): %s", len(got), got)
	}
}
