package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

type ChannelIngestResult struct {
	MacroOK    bool
	GeoOK      bool
	CapFlowOK  bool
	ExportOK   bool
	TsmcOK     bool
	TwGeoOK    bool
	JanusOK    bool
	TejOK      bool
	MacroErr   string
	GeoErr     string
	CapFlowErr string
	ExportErr  string
	TsmcErr    string
	TwGeoErr   string
	JanusErr   string
	TejErr     string
}

type ChannelIngestService struct {
	WorkDir           string
	Pool              *pgxpool.Pool
	MacroIngestor     *narrative.MacroIngestor
	GeoProvider       narrative.GeopoliticalRiskProvider
	TaiwanGeoProvider *narrative.CompositeTaiwanGeopoliticalProvider
	JanusEngine       *janus.Engine
	healthStore       *ChannelHealthStoreAdapter
}

func NewChannelIngestService(workDir string, pool *pgxpool.Pool, macroIngestor *narrative.MacroIngestor, geoProvider narrative.GeopoliticalRiskProvider, taiwanGeoProvider *narrative.CompositeTaiwanGeopoliticalProvider, janusEngine *janus.Engine) *ChannelIngestService {
	return &ChannelIngestService{
		WorkDir:           workDir,
		Pool:              pool,
		MacroIngestor:     macroIngestor,
		GeoProvider:       geoProvider,
		TaiwanGeoProvider: taiwanGeoProvider,
		JanusEngine:       janusEngine,
		healthStore:       NewChannelHealthStoreAdapter(filepath.Join(workDir, "data/state"), pool),
	}
}

func (s *ChannelIngestService) TriggerIngest(ctx context.Context, channel string) error {
	switch channel {
	case "us_yahoo", "jpy_yahoo":
		return s.triggerMacroIngest(ctx)
	case "geopolitical":
		return s.triggerGeoIngest(ctx)
	case "twse_capital_flow":
		return s.triggerCapFlowIngest(ctx)
	case "export_statistics":
		return s.triggerExportIngest(ctx)
	case "twse_margin":
		return s.triggerMarginIngest(ctx)
	case "tsmc_revenue":
		return s.triggerTsmcIngest(ctx)
	case "geopolitical_taiwan":
		return s.triggerTaiwanGeoIngest(ctx)
	case "janus_regime":
		return s.triggerJanusIngest(ctx)
	case "tej":
		return s.triggerTejIngest(ctx)
	default:
		return fmt.Errorf("unknown channel: %s", channel)
	}
}

func (s *ChannelIngestService) TriggerAllIngests(ctx context.Context) ChannelIngestResult {
	stateDir := filepath.Join(s.WorkDir, "data/state")
	var wg sync.WaitGroup
	var macroErr, geoErr, capFlowErr, exportErr, tsmcErr, twGeoErr, janusErr, tejErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		events, snap, err := s.MacroIngestor.Ingest(ctx)
		if err != nil {
			macroErr = err
			s.healthStore.Record("us_yahoo", "error", err.Error())
			s.healthStore.Record("jpy_yahoo", "error", err.Error())
			log.Printf("[ChannelIngest] macro ingest failed: %v", err)
			return
		}
		s.healthStore.Record("us_yahoo", "ok", "")
		s.healthStore.Record("jpy_yahoo", "ok", "")
		log.Printf("[ChannelIngest] macro ingest succeeded: %d events, recorded_at=%d", len(events), snap.RecordedAt)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		score, err := s.GeoProvider.FetchScore(ctx)
		if err != nil {
			geoErr = err
			s.healthStore.Record("geopolitical", "error", err.Error())
			log.Printf("[ChannelIngest] geo ingest failed: %v", err)
			return
		}
		store := narrative.NewGeopoliticalStore(filepath.Join(stateDir, "geopolitical"))
		if err := store.Save(score); err != nil {
			geoErr = err
			s.healthStore.Record("geopolitical", "error", err.Error())
			log.Printf("[ChannelIngest] geo save failed: %v", err)
			return
		}
		s.healthStore.Record("geopolitical", "ok", "")
		log.Printf("[ChannelIngest] geo ingest succeeded: intensity=%.2f", score.Intensity)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		capFlowProvider := marketdata.NewTWSECapitalFlowProvider(filepath.Join(stateDir, "capital_flow"))
		_, err := capFlowProvider.FetchSnapshot(ctx)
		if err != nil {
			capFlowErr = err
			s.healthStore.Record("twse_capital_flow", "error", err.Error())
			log.Printf("[ChannelIngest] capital flow ingest failed: %v", err)
			return
		}
		s.healthStore.Record("twse_capital_flow", "ok", "")
		log.Printf("[ChannelIngest] capital flow ingest succeeded")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		exportProvider := marketdata.NewExportStatisticsProvider(filepath.Join(stateDir, "export"))
		_, err := exportProvider.FetchSnapshot(ctx)
		if err != nil {
			exportErr = err
			s.healthStore.Record("export_statistics", "error", err.Error())
			log.Printf("[ChannelIngest] export statistics ingest failed: %v", err)
			return
		}
		s.healthStore.Record("export_statistics", "ok", "")
		log.Printf("[ChannelIngest] export statistics ingest succeeded")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		marginProvider := marketdata.NewTWSEBalanceProvider(filepath.Join(stateDir, "margin"))
		_, err := marginProvider.FetchSnapshot(ctx)
		if err != nil {
			s.healthStore.Record("twse_margin", "error", err.Error())
			log.Printf("[ChannelIngest] TWSE margin balance ingest failed: %v", err)
			return
		}
		s.healthStore.Record("twse_margin", "ok", "")
		log.Printf("[ChannelIngest] TWSE margin balance ingest succeeded")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		tsmcProvider := marketdata.NewTSMCRevenueProvider(filepath.Join(stateDir, "tsmc_revenue"))
		_, err := tsmcProvider.FetchSnapshot(ctx)
		if err != nil {
			tsmcErr = err
			s.healthStore.Record("tsmc_revenue", "error", err.Error())
			log.Printf("[ChannelIngest] TSMC revenue ingest failed: %v", err)
			return
		}
		s.healthStore.Record("tsmc_revenue", "ok", "")
		log.Printf("[ChannelIngest] TSMC revenue ingest succeeded")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		twGeoScore, err := s.TaiwanGeoProvider.FetchScore(ctx)
		if err != nil {
			twGeoErr = err
			s.healthStore.Record("geopolitical_taiwan", "error", err.Error())
			log.Printf("[ChannelIngest] Taiwan geopolitical ingest failed: %v", err)
			return
		}
		twStore := narrative.NewGeopoliticalStore(filepath.Join(stateDir, "geopolitical", "taiwan"))
		if err := twStore.Save(twGeoScore); err != nil {
			twGeoErr = err
			s.healthStore.Record("geopolitical_taiwan", "error", err.Error())
			log.Printf("[ChannelIngest] Taiwan geopolitical save failed: %v", err)
			return
		}
		s.healthStore.Record("geopolitical_taiwan", "ok", "")
		log.Printf("[ChannelIngest] Taiwan geopolitical ingest succeeded: intensity=%.2f", twGeoScore.Intensity)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if s.JanusEngine == nil {
			janusErr = fmt.Errorf("JANUS engine not initialized")
			s.healthStore.Record("janus_regime", "error", janusErr.Error())
			log.Printf("[ChannelIngest] JANUS regime ingest skipped: engine not initialized")
			return
		}
		s.JanusEngine.Update()
		status := s.JanusEngine.GetStatus()
		if status.LastUpdated.IsZero() {
			janusErr = fmt.Errorf("JANUS engine has no data after update")
			s.healthStore.Record("janus_regime", "error", janusErr.Error())
			log.Printf("[ChannelIngest] JANUS regime ingest failed: %v", janusErr)
			return
		}
		s.healthStore.Record("janus_regime", "ok", "")
		log.Printf("[ChannelIngest] JANUS regime ingest succeeded: class=%s", status.Classification)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		tejKey := os.Getenv("TEJ_API_KEY")
		if tejKey == "" {
			s.healthStore.Record("tej", "inactive", "TEJ_API_KEY not set")
			log.Printf("[ChannelIngest] TEJ ingest skipped: TEJ_API_KEY not set")
			return
		}
		tejClient := marketdata.NewTEJClient(tejKey)
		if err := tejClient.Ping(ctx); err != nil {
			tejErr = err
			s.healthStore.Record("tej", "error", err.Error())
			log.Printf("[ChannelIngest] TEJ ingest failed: %v", err)
			return
		}
		s.healthStore.Record("tej", "ok", "")
		log.Printf("[ChannelIngest] TEJ ingest succeeded")
	}()

	wg.Wait()

	return ChannelIngestResult{
		MacroOK:    macroErr == nil,
		GeoOK:      geoErr == nil,
		CapFlowOK:  capFlowErr == nil,
		ExportOK:   exportErr == nil,
		TsmcOK:     tsmcErr == nil,
		TwGeoOK:    twGeoErr == nil,
		JanusOK:    janusErr == nil,
		TejOK:      tejErr == nil,
		MacroErr:   errStr(macroErr),
		GeoErr:     errStr(geoErr),
		CapFlowErr: errStr(capFlowErr),
		ExportErr:  errStr(exportErr),
		TsmcErr:    errStr(tsmcErr),
		TwGeoErr:   errStr(twGeoErr),
		JanusErr:   errStr(janusErr),
		TejErr:     errStr(tejErr),
	}
}

func (s *ChannelIngestService) triggerMacroIngest(ctx context.Context) error {
	events, snap, err := s.MacroIngestor.Ingest(ctx)
	if err != nil {
		s.healthStore.Record("us_yahoo", "error", err.Error())
		s.healthStore.Record("jpy_yahoo", "error", err.Error())
		return err
	}
	s.healthStore.Record("us_yahoo", "ok", "")
	s.healthStore.Record("jpy_yahoo", "ok", "")
	log.Printf("[ChannelIngest] macro ingest succeeded: %d events, recorded_at=%d", len(events), snap.RecordedAt)
	return nil
}

func (s *ChannelIngestService) triggerGeoIngest(ctx context.Context) error {
	stateDir := filepath.Join(s.WorkDir, "data/state")
	score, err := s.GeoProvider.FetchScore(ctx)
	if err != nil {
		s.healthStore.Record("geopolitical", "error", err.Error())
		return err
	}
	store := narrative.NewGeopoliticalStore(filepath.Join(stateDir, "geopolitical"))
	if err := store.Save(score); err != nil {
		s.healthStore.Record("geopolitical", "error", err.Error())
		return err
	}
	s.healthStore.Record("geopolitical", "ok", "")
	return nil
}

func (s *ChannelIngestService) triggerCapFlowIngest(ctx context.Context) error {
	stateDir := filepath.Join(s.WorkDir, "data/state")
	capFlowProvider := marketdata.NewTWSECapitalFlowProvider(filepath.Join(stateDir, "capital_flow"))
	_, err := capFlowProvider.FetchSnapshot(ctx)
	if err != nil {
		s.healthStore.Record("twse_capital_flow", "error", err.Error())
		return err
	}
	s.healthStore.Record("twse_capital_flow", "ok", "")
	return nil
}

func (s *ChannelIngestService) triggerExportIngest(ctx context.Context) error {
	stateDir := filepath.Join(s.WorkDir, "data/state")
	exportProvider := marketdata.NewExportStatisticsProvider(filepath.Join(stateDir, "export"))
	_, err := exportProvider.FetchSnapshot(ctx)
	if err != nil {
		s.healthStore.Record("export_statistics", "error", err.Error())
		return err
	}
	s.healthStore.Record("export_statistics", "ok", "")
	return nil
}

func (s *ChannelIngestService) triggerMarginIngest(ctx context.Context) error {
	stateDir := filepath.Join(s.WorkDir, "data/state")
	marginProvider := marketdata.NewTWSEBalanceProvider(filepath.Join(stateDir, "margin"))
	_, err := marginProvider.FetchSnapshot(ctx)
	if err != nil {
		s.healthStore.Record("twse_margin", "error", err.Error())
		return err
	}
	s.healthStore.Record("twse_margin", "ok", "")
	return nil
}

func (s *ChannelIngestService) triggerTsmcIngest(ctx context.Context) error {
	stateDir := filepath.Join(s.WorkDir, "data/state")
	tsmcProvider := marketdata.NewTSMCRevenueProvider(filepath.Join(stateDir, "tsmc_revenue"))
	_, err := tsmcProvider.FetchSnapshot(ctx)
	if err != nil {
		s.healthStore.Record("tsmc_revenue", "error", err.Error())
		return err
	}
	s.healthStore.Record("tsmc_revenue", "ok", "")
	return nil
}

func (s *ChannelIngestService) triggerTaiwanGeoIngest(ctx context.Context) error {
	stateDir := filepath.Join(s.WorkDir, "data/state")
	twGeoScore, err := s.TaiwanGeoProvider.FetchScore(ctx)
	if err != nil {
		s.healthStore.Record("geopolitical_taiwan", "error", err.Error())
		return err
	}
	twStore := narrative.NewGeopoliticalStore(filepath.Join(stateDir, "geopolitical", "taiwan"))
	if err := twStore.Save(twGeoScore); err != nil {
		s.healthStore.Record("geopolitical_taiwan", "error", err.Error())
		return err
	}
	s.healthStore.Record("geopolitical_taiwan", "ok", "")
	return nil
}

func (s *ChannelIngestService) triggerJanusIngest(ctx context.Context) error {
	if s.JanusEngine == nil {
		err := fmt.Errorf("JANUS engine not initialized")
		s.healthStore.Record("janus_regime", "error", err.Error())
		return err
	}
	s.JanusEngine.Update()
	status := s.JanusEngine.GetStatus()
	if status.LastUpdated.IsZero() {
		err := fmt.Errorf("JANUS engine has no data after update")
		s.healthStore.Record("janus_regime", "error", err.Error())
		return err
	}
	s.healthStore.Record("janus_regime", "ok", "")
	return nil
}

func (s *ChannelIngestService) triggerTejIngest(ctx context.Context) error {
	tejKey := os.Getenv("TEJ_API_KEY")
	if tejKey == "" {
		s.healthStore.Record("tej", "inactive", "TEJ_API_KEY not set")
		return fmt.Errorf("TEJ_API_KEY not set")
	}
	tejClient := marketdata.NewTEJClient(tejKey)
	if err := tejClient.Ping(ctx); err != nil {
		s.healthStore.Record("tej", "error", err.Error())
		return err
	}
	s.healthStore.Record("tej", "ok", "")
	return nil
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
