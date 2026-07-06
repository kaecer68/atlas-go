package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/constants"
)

// channelState tracks enable/disable status for each data channel.
type channelState struct {
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LoadChannelStates loads channel states from the state file.
func (h *Handlers) LoadChannelStates() {
	path := filepath.Join(h.WorkDir, constants.StateChannels+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		return
	}
	var loaded map[string]channelState
	if err := json.Unmarshal(b, &loaded); err != nil {
		return
	}
	h.channelStatesMu.Lock()
	defer h.channelStatesMu.Unlock()
	h.channelStates = loaded
}

// saveChannelStates persists channel states to the state file.
func (h *Handlers) saveChannelStates() {
	h.channelStatesMu.RLock()
	defer h.channelStatesMu.RUnlock()

	path := filepath.Join(h.WorkDir, constants.StateChannels+".json")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	b, err := json.MarshalIndent(h.channelStates, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

// setChannelEnabled updates the enabled status for a channel.
func (h *Handlers) setChannelEnabled(channelID string, enabled bool) {
	h.channelStatesMu.Lock()
	h.channelStates[channelID] = channelState{
		Enabled:   enabled,
		UpdatedAt: time.Now(),
	}
	h.channelStatesMu.Unlock()
	h.saveChannelStates()
}
