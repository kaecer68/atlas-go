package marketdata

import (
	"context"
	"errors"
	"sync"
)

// CalendarEventProvider fetches calendar events from an external data source.
//
// Maturity: evolving
type CalendarEventProvider interface {
	Name() string
	FetchEvents(ctx context.Context, year int) ([]CalendarProviderData, error)
}

// CompositeCalendarProvider merges events from multiple providers using
// last-write-wins strategy (later providers in the list take precedence
// over earlier ones for duplicate event type+date combinations).
//
// Maturity: evolving
type CompositeCalendarProvider struct {
	mu        sync.RWMutex
	providers []CalendarEventProvider
}

// NewCompositeCalendarProvider creates a composite from the given providers.
// Providers later in the list override earlier ones for duplicate events.
func NewCompositeCalendarProvider(providers ...CalendarEventProvider) *CompositeCalendarProvider {
	return &CompositeCalendarProvider{providers: providers}
}

// Name returns the provider name.
func (c *CompositeCalendarProvider) Name() string {
	return "composite_calendar"
}

// FetchEvents merges events from all providers (last write wins by type+date).
// Returns partial results when some providers fail; only returns error when
// ALL providers fail.
func (c *CompositeCalendarProvider) FetchEvents(ctx context.Context, year int) ([]CalendarProviderData, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	seen := make(map[string]int) // key → index in merged
	var merged []CalendarProviderData
	var errs []error

	for _, p := range c.providers {
		events, err := p.FetchEvents(ctx, year)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, e := range events {
			key := e.EventType + "|" + e.Date + "|" + e.Symbol
			if idx, exists := seen[key]; exists {
				merged[idx] = e // last write wins
			} else {
				seen[key] = len(merged)
				merged = append(merged, e)
			}
		}
	}

	if len(errs) > 0 && len(errs) == len(c.providers) {
		return merged, errors.Join(errs...)
	}
	return merged, nil
}

// AddProvider appends a provider to the composite. Later providers take
// precedence for duplicate event type+date combinations.
func (c *CompositeCalendarProvider) AddProvider(p CalendarEventProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.providers = append(c.providers, p)
}
