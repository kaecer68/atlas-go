package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func main() {
	sourceDir := flag.String("source", "data/ledger", "source JSONL directory")
	targetPath := flag.String("target", "data/state/atlas.db", "target SQLite file path")
	dryRun := flag.Bool("dry-run", false, "show what would be migrated without writing")
	storeBackend := flag.String("store-backend", "", "guard: refuse 'postgres' (this tool writes SQLite only)")
	flag.Parse()

	if *storeBackend == "postgres" {
		fmt.Fprintln(os.Stderr, "error: -store-backend=postgres is not supported by this tool (JSONL → SQLite only). Use cmd/migrate-data with -quotes/-outcomes-sqlite/... for the PostgreSQL backend.")
		os.Exit(1)
	}

	sourceAbs, err := filepath.Abs(*sourceDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve source path:", err)
		os.Exit(1)
	}
	targetAbs, err := filepath.Abs(*targetPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve target path:", err)
		os.Exit(1)
	}

	fmt.Println("=== JSONL → SQLite Migration ===")
	fmt.Printf("Source:    %s\n", sourceAbs)
	fmt.Printf("Target:    %s\n", targetAbs)
	fmt.Printf("Dry run:   %v\n", *dryRun)
	fmt.Println()

	if *dryRun {
		fmt.Println("[DRY RUN] No data will be written.")
		fmt.Println()
	}

	var outcomeStore *ledger.SQLiteOutcomeStore
	if !*dryRun {
		db, err := ledger.OpenSQLiteDB(*targetPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open sqlite db:", err)
			os.Exit(1)
		}
		defer func() { _ = db.Close() }()

		if err := ledger.InitSchema(db); err != nil {
			fmt.Fprintln(os.Stderr, "init schema:", err)
			os.Exit(1)
		}
		fmt.Printf("Opened SQLite DB: %s\n", *targetPath)
		fmt.Println()

		outcomeStore = ledger.NewSQLiteOutcomeStore(db)
	}

	jsonStore := ledger.NewStore(sourceAbs)
	fullStore, ok := jsonStore.(*ledger.Store)
	if !ok {
		fmt.Fprintln(os.Stderr, "json store does not support LoadExperiments")
		os.Exit(1)
	}

	migrated := struct {
		outcomes           int
		sessionOutcomes    int
		screeningRejects   int
		sessionSummaries   int
		humanInterventions int
		experiments        int
	}{}
	errors := 0

	outcomes, err := jsonStore.LoadOutcomes()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load outcomes:", err)
		errors++
	} else {
		migrated.outcomes = len(outcomes)
		if *dryRun {
			fmt.Printf("  outcomes: %d (dry-run)\n", len(outcomes))
		} else {
			if err := outcomeStore.RecordOutcomes(outcomes); err != nil {
				fmt.Fprintf(os.Stderr, "record outcomes: %v\n", err)
				errors++
			} else {
				fmt.Printf("  outcomes: %d ✓\n", len(outcomes))
			}
		}
	}

	sessions, err := os.ReadDir(filepath.Join(sourceAbs, "sessions"))
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "read sessions dir:", err)
			errors++
		}
	} else {
		for _, entry := range sessions {
			if !entry.IsDir() {
				continue
			}
			sessionID := entry.Name()

			sessionOutcomes, err := jsonStore.LoadSessionOutcomes(sessionID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "load session %s outcomes: %v\n", sessionID, err)
				errors++
				continue
			}
			if len(sessionOutcomes) > 0 {
				migrated.sessionOutcomes += len(sessionOutcomes)
				if *dryRun {
					fmt.Printf("  session %s outcomes: %d (dry-run)\n", sessionID, len(sessionOutcomes))
				} else {
					session := domain.ReplaySession{ID: sessionID}
					if err := outcomeStore.RecordSessionOutcomes(session, sessionOutcomes); err != nil {
						fmt.Fprintf(os.Stderr, "record session %s outcomes: %v\n", sessionID, err)
						errors++
					} else {
						fmt.Printf("  session %s outcomes: %d ✓\n", sessionID, len(sessionOutcomes))
					}
				}
			}

			rejects, err := jsonStore.LoadSessionScreeningRejects(sessionID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "load session %s screening rejects: %v\n", sessionID, err)
				errors++
				continue
			}
			if len(rejects) > 0 {
				migrated.screeningRejects += len(rejects)
				if *dryRun {
					fmt.Printf("  session %s screening_rejects: %d (dry-run)\n", sessionID, len(rejects))
				} else {
					if err := outcomeStore.RecordSessionScreeningRejects(sessionID, rejects); err != nil {
						fmt.Fprintf(os.Stderr, "record session %s screening rejects: %v\n", sessionID, err)
						errors++
					} else {
						fmt.Printf("  session %s screening_rejects: %d ✓\n", sessionID, len(rejects))
					}
				}
			}
		}
	}

	sessionSummaries, err := jsonStore.LoadSessionSummaries()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load session summaries:", err)
		errors++
	} else {
		migrated.sessionSummaries = len(sessionSummaries)
		if *dryRun {
			fmt.Printf("  session_summaries: %d (dry-run)\n", len(sessionSummaries))
		} else {
			for _, summary := range sessionSummaries {
				session := domain.ReplaySession{ID: summary.SessionID}
				if err := outcomeStore.RecordSessionSummary(session, summary); err != nil {
					fmt.Fprintf(os.Stderr, "record session summary %s: %v\n", summary.SessionID, err)
					errors++
				}
			}
			fmt.Printf("  session_summaries: %d ✓\n", len(sessionSummaries))
		}
	}

	interventions, err := jsonStore.LoadHumanInterventions()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load human interventions:", err)
		errors++
	} else {
		migrated.humanInterventions = len(interventions)
		if *dryRun {
			fmt.Printf("  human_interventions: %d (dry-run)\n", len(interventions))
		} else {
			for _, intervention := range interventions {
				if err := outcomeStore.RecordHumanIntervention(intervention); err != nil {
					fmt.Fprintf(os.Stderr, "record human intervention: %v\n", err)
					errors++
				}
			}
			fmt.Printf("  human_interventions: %d ✓\n", len(interventions))
		}
	}

	experiments, err := fullStore.LoadExperiments()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load experiments:", err)
		errors++
	} else {
		migrated.experiments = len(experiments)
		if *dryRun {
			fmt.Printf("  experiments: %d (dry-run)\n", len(experiments))
		} else {
			for _, experiment := range experiments {
				if err := outcomeStore.RecordExperiment(experiment); err != nil {
					fmt.Fprintf(os.Stderr, "record experiment %s: %v\n", experiment.ID, err)
					errors++
				}
			}
			fmt.Printf("  experiments: %d ✓\n", len(experiments))
		}
	}

	fmt.Println()
	fmt.Println("=== Migration Summary ===")
	fmt.Printf("Global outcomes:       %d\n", migrated.outcomes)
	fmt.Printf("Session outcomes:       %d\n", migrated.sessionOutcomes)
	fmt.Printf("Screening rejects:      %d\n", migrated.screeningRejects)
	fmt.Printf("Session summaries:      %d\n", migrated.sessionSummaries)
	fmt.Printf("Human interventions:    %d\n", migrated.humanInterventions)
	fmt.Printf("Experiments:            %d\n", migrated.experiments)
	fmt.Println()

	total := migrated.outcomes + migrated.sessionOutcomes + migrated.screeningRejects +
		migrated.sessionSummaries + migrated.humanInterventions + migrated.experiments
	fmt.Printf("Total records:          %d\n", total)

	if errors > 0 {
		fmt.Printf("\nErrors encountered: %d (continuing...)\n", errors)
	}

	if *dryRun {
		fmt.Println("\n[DRY RUN] No data was written.")
	} else {
		fmt.Printf("\nMigration complete. DB: %s\n", *targetPath)
	}
}
