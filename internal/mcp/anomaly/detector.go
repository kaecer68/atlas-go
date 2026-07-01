package anomaly

import (
	"math"
	"sync"
	"time"
)

// Config tunes the anomaly detector thresholds. Zero values use conservative
// defaults.
type Config struct {
	ShortWindow          time.Duration // rolling short window (default 5m)
	LongWindow           time.Duration // rolling long window (default 24h)
	BurstZScoreThreshold float64       // z-score threshold for burst detection (default 3.0)
	ErrorRateThreshold   float64       // error rate threshold for tool/tenant anomalies (default 0.5)
	MinObservations      int           // minimum observations before error-rate anomaly (default 5)
	MaxStoreEvents       int           // anomaly store capacity (default 1000)
}

// Detector consumes audit entries and emits AnomalyEvent values when a
// baseline is breached.
type Detector struct {
	cfg         Config
	metrics     ScoreRecorder
	store       *Store
	now         func() time.Time
	mu          sync.Mutex
	tenantObs   map[string]*obsWindow
	toolStats   map[string]*statusWindow
	tenantStats map[string]*statusWindow
	emitted     map[string]time.Time
}

type obsWindow struct {
	short []time.Time
	long  []time.Time
}

type statusWindow struct {
	entries []statusRecord
}

type statusRecord struct {
	ts time.Time
	ok bool
}

// NewDetector builds a Detector with defaults for zero-valued Config fields.
func NewDetector(cfg Config, metrics ScoreRecorder, store *Store) *Detector {
	if cfg.ShortWindow <= 0 {
		cfg.ShortWindow = 5 * time.Minute
	}
	if cfg.LongWindow <= 0 {
		cfg.LongWindow = 24 * time.Hour
	}
	if cfg.BurstZScoreThreshold <= 0 {
		cfg.BurstZScoreThreshold = 3.0
	}
	if cfg.ErrorRateThreshold <= 0 {
		cfg.ErrorRateThreshold = 0.5
	}
	if cfg.MinObservations <= 0 {
		cfg.MinObservations = 5
	}
	if cfg.MaxStoreEvents <= 0 {
		cfg.MaxStoreEvents = 1000
	}
	if store == nil {
		store = NewStore(cfg.MaxStoreEvents)
	}
	return &Detector{
		cfg:         cfg,
		metrics:     metrics,
		store:       store,
		now:         time.Now,
		tenantObs:   make(map[string]*obsWindow),
		toolStats:   make(map[string]*statusWindow),
		tenantStats: make(map[string]*statusWindow),
		emitted:     make(map[string]time.Time),
	}
}

// Observe ingests one audit entry. V1 entries (Version() < 2) are ignored.
func (d *Detector) Observe(e Entry) {
	if e.Version() < 2 {
		return
	}
	now := d.now()
	ts := e.ObservedAt()
	if ts.IsZero() {
		ts = now
	}
	tenant := e.GetTenantID()
	if tenant == "" {
		tenant = "anonymous"
	}
	tool := e.GetTool()

	d.mu.Lock()
	ow := d.tenantObs[tenant]
	if ow == nil {
		ow = &obsWindow{}
		d.tenantObs[tenant] = ow
	}
	ow.short = append(ow.short, ts)
	ow.long = append(ow.long, ts)

	tw := d.toolStats[tool]
	if tw == nil {
		tw = &statusWindow{}
		d.toolStats[tool] = tw
	}
	tw.entries = append(tw.entries, statusRecord{ts: ts, ok: e.GetStatus() == "ok"})

	tnw := d.tenantStats[tenant]
	if tnw == nil {
		tnw = &statusWindow{}
		d.tenantStats[tenant] = tnw
	}
	tnw.entries = append(tnw.entries, statusRecord{ts: ts, ok: e.GetStatus() == "ok"})

	d.evict(now)
	d.mu.Unlock()

	d.evaluate(now, tenant, tool)
}

// Store returns the underlying anomaly event store.
func (d *Detector) Store() *Store {
	return d.store
}

func (d *Detector) evict(now time.Time) {
	shortCutoff := now.Add(-d.cfg.ShortWindow)
	longCutoff := now.Add(-d.cfg.LongWindow)

	for _, ow := range d.tenantObs {
		ow.short = filterTimes(ow.short, shortCutoff)
		ow.long = filterTimes(ow.long, longCutoff)
	}
	for _, w := range d.toolStats {
		w.entries = filterRecords(w.entries, shortCutoff)
	}
	for _, w := range d.tenantStats {
		w.entries = filterRecords(w.entries, shortCutoff)
	}
}

func (d *Detector) evaluate(now time.Time, tenant, tool string) {
	d.evalBurst(now, tenant)
	d.evalToolError(now, tenant, tool)
	d.evalTenantError(now, tenant)
}

func (d *Detector) evalBurst(now time.Time, tenant string) {
	w, ok := d.tenantObs[tenant]
	if !ok || len(w.short) == 0 {
		return
	}
	shortCount := len(w.short)
	bucketSize := d.cfg.ShortWindow
	numBuckets := int(d.cfg.LongWindow / bucketSize)
	if numBuckets <= 0 {
		numBuckets = 1
	}
	cutoff := now.Add(-d.cfg.LongWindow)
	counts := bucketCounts(w.long, cutoff, bucketSize, numBuckets)
	mean, std := meanStd(counts)

	var z float64
	if std > 0 {
		z = (float64(shortCount) - mean) / std
	} else if float64(shortCount) > mean {
		z = d.cfg.BurstZScoreThreshold + 1
	}

	if z > d.cfg.BurstZScoreThreshold {
		d.emit(now, tenant, "burst", z, "")
	}
}

func (d *Detector) evalToolError(now time.Time, tenant, tool string) {
	w, ok := d.toolStats[tool]
	if !ok {
		return
	}
	total, errors := countErrors(w.entries, now.Add(-d.cfg.ShortWindow))
	if total < d.cfg.MinObservations {
		return
	}
	rate := float64(errors) / float64(total)
	if rate > d.cfg.ErrorRateThreshold {
		d.emit(now, tenant, "tool_error_spike", rate, tool)
	}
}

func (d *Detector) evalTenantError(now time.Time, tenant string) {
	w, ok := d.tenantStats[tenant]
	if !ok {
		return
	}
	total, errors := countErrors(w.entries, now.Add(-d.cfg.ShortWindow))
	if total < d.cfg.MinObservations {
		return
	}
	rate := float64(errors) / float64(total)
	if rate > d.cfg.ErrorRateThreshold {
		d.emit(now, tenant, "tenant_error_anomaly", rate, "")
	}
}

func (d *Detector) emit(now time.Time, tenant, anomalyType string, score float64, tool string) {
	var key string
	switch anomalyType {
	case "tool_error_spike":
		key = anomalyType + "|" + tool
	case "tenant_error_anomaly":
		key = anomalyType + "|" + tenant
	default:
		key = anomalyType + "|" + tenant
	}
	if last, ok := d.emitted[key]; ok && now.Sub(last) < d.cfg.ShortWindow {
		return
	}
	d.emitted[key] = now

	event := AnomalyEvent{
		TenantID:    tenant,
		AnomalyType: anomalyType,
		Score:       score,
		TS:          now.UTC().Format(time.RFC3339),
		Tool:        tool,
	}
	d.store.Add(event)
	if d.metrics != nil {
		d.metrics.SetAnomalyScore(tenant, anomalyType, score)
	}
}

func filterTimes(ts []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for _, t := range ts {
		if !t.Before(cutoff) {
			ts[i] = t
			i++
		}
	}
	return ts[:i]
}

func filterRecords(recs []statusRecord, cutoff time.Time) []statusRecord {
	i := 0
	for _, r := range recs {
		if !r.ts.Before(cutoff) {
			recs[i] = r
			i++
		}
	}
	return recs[:i]
}

func bucketCounts(obs []time.Time, cutoff time.Time, bucketSize time.Duration, numBuckets int) []float64 {
	counts := make([]float64, numBuckets)
	for _, t := range obs {
		if t.Before(cutoff) {
			continue
		}
		idx := int(t.Sub(cutoff) / bucketSize)
		if idx < 0 {
			continue
		}
		if idx >= numBuckets {
			idx = numBuckets - 1
		}
		counts[idx]++
	}
	return counts
}

func meanStd(vals []float64) (float64, float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean := sum / float64(len(vals))
	var sq float64
	for _, v := range vals {
		diff := v - mean
		sq += diff * diff
	}
	std := math.Sqrt(sq / float64(len(vals)))
	return mean, std
}

func countErrors(recs []statusRecord, cutoff time.Time) (total, errors int) {
	for _, r := range recs {
		if r.ts.Before(cutoff) {
			continue
		}
		total++
		if !r.ok {
			errors++
		}
	}
	return
}
