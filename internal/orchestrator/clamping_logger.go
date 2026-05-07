package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

type clampingLogger struct {
	mu   sync.Mutex
	path string
}

func newClampingLogger(path string) *clampingLogger {
	return &clampingLogger{path: path}
}

func (l *clampingLogger) Append(payload eventbus.ClampingEventPayload) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Printf("[clampingLogger] failed to open file: %v\n", err)
		return
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	if err := enc.Encode(payload); err != nil {
		fmt.Printf("[clampingLogger] failed to encode payload: %v\n", err)
	}
}

func (l *clampingLogger) AppendConvictionEvents(events []portfolio.ConvictionClampingEvent) {
	if l == nil || len(events) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Printf("[clampingLogger] failed to open file: %v\n", err)
		return
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			fmt.Printf("[clampingLogger] failed to encode conviction event: %v\n", err)
		}
	}
}
