// Package marketdata: captcha_cooldown.go
//
// CaptchaCooldown is the per-channel, in-memory CAPTCHA backoff used by
// upstream fetches that may hit a CAPTCHA gate. The first use case is the
// TWSE bsr endpoint (GovernmentBrokerAggregator / channel
// "government_broker"), which serves a CAPTCHA page when queried too
// aggressively.
//
// Policy (fix/20260731-govflow-cadence):
//   - Default cooldown duration: 24h. Override via CaptchaCooldownWith.
//   - ShouldSkip returns true if the most recent RecordCaptcha for the
//     channel was within Cooldown. The check is wall-clock-based: process
//     restart = full reset (acceptable, since the upstream's view of the
//     cooldown is what we are protecting).
//   - RecordCaptcha sets the cooldown start time for the channel to now.
//   - RecordSuccess clears the cooldown. Any non-CAPTCHA success (e.g. a
//     successful fetch after a CAPTCHA error) must clear so the next
//     CAPTCHA does not inherit a stale timer.
//   - IsCaptchaErr matches errors.Is(err, ErrCaptchaRequired). Callers
//     that have a different error type (string match, custom wrapper)
//     can drive RecordCaptcha directly without going through IsCaptchaErr.
//   - All state is in-memory. No persistence. Matches the prompt's
//     "process 內記憶體即可" requirement — restart resets, which is
//     intentional (do not persist CAPTCHA-on-upstream state across
//     deploys, since the upstream's posture may have changed).
//
// Thread-safety: uses a sync.RWMutex so ShouldSkip can be a fast read
// in the hot path (1h tick that gates a 24h fetch).
package marketdata

import (
	"errors"
	"sync"
	"time"
)

// DefaultCaptchaCooldownDuration is the default backoff window after a
// CAPTCHA response. 24h matches the prompt's "跳過後續嘗試一段時間
// （如 24h）" example. Tunable per-instance via CaptchaCooldownWith.
const DefaultCaptchaCooldownDuration = 24 * time.Hour

// CaptchaCooldown tracks per-channel CAPTCHA backoff state in memory.
type CaptchaCooldown struct {
	cooldown  time.Duration
	mu        sync.RWMutex
	cooldowns map[string]time.Time
	now       func() time.Time // injectable for tests
}

// NewCaptchaCooldown returns a CaptchaCooldown using
// DefaultCaptchaCooldownDuration and time.Now as the clock.
func NewCaptchaCooldown() *CaptchaCooldown {
	return &CaptchaCooldown{
		cooldown:  DefaultCaptchaCooldownDuration,
		cooldowns: make(map[string]time.Time),
		now:       time.Now,
	}
}

// CaptchaCooldownWith is a constructor that allows overriding the
// cooldown duration and the clock (for deterministic tests). Returns
// nil if d is non-positive (callers that hold *CaptchaCooldown should
// always nil-check before calling).
func CaptchaCooldownWith(d time.Duration, clock func() time.Time) *CaptchaCooldown {
	if d <= 0 {
		return nil
	}
	if clock == nil {
		clock = time.Now
	}
	return &CaptchaCooldown{
		cooldown:  d,
		cooldowns: make(map[string]time.Time),
		now:       clock,
	}
}

// ShouldSkip reports whether the given channel is currently in a
// CAPTCHA cooldown window. Returns false if the channel is unknown
// (i.e. has never hit a CAPTCHA), or if the cooldown has expired.
func (c *CaptchaCooldown) ShouldSkip(channelID string) bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	start, ok := c.cooldowns[channelID]
	c.mu.RUnlock()
	if !ok {
		return false
	}
	return c.now().Sub(start) < c.cooldown
}

// Until returns the wall-clock time at which the cooldown for the
// given channel will expire. Returns the zero time if the channel is
// not in cooldown (callers can compare with .IsZero() to distinguish
// "not in cooldown" from "cooldown already expired").
func (c *CaptchaCooldown) Until(channelID string) time.Time {
	if c == nil {
		return time.Time{}
	}
	c.mu.RLock()
	start, ok := c.cooldowns[channelID]
	c.mu.RUnlock()
	if !ok {
		return time.Time{}
	}
	return start.Add(c.cooldown)
}

// RecordCaptcha marks the channel as having hit a CAPTCHA response
// right now. Subsequent ShouldSkip calls return true for the next
// Cooldown window.
func (c *CaptchaCooldown) RecordCaptcha(channelID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.cooldowns[channelID] = c.now()
	c.mu.Unlock()
}

// RecordSuccess clears the cooldown for the channel, so the next
// CAPTCHA will start a fresh window. No-op if the channel has no
// active cooldown.
func (c *CaptchaCooldown) RecordSuccess(channelID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.cooldowns, channelID)
	c.mu.Unlock()
}

// IsCaptchaErr reports whether err is (or wraps) ErrCaptchaRequired.
// The standard pattern is:
//
//	if cd.IsCaptchaErr(err) {
//	    cd.RecordCaptcha(channelID)
//	}
func (c *CaptchaCooldown) IsCaptchaErr(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrCaptchaRequired)
}
