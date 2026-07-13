// Package spawning provides agent generation management with SpawningManager
// and spawning cycles for atlas-go's evolution loop.
//
// SpawningManager orchestrates the creation of new agent variants during
// evolution cycles: it reads baseline agent specifications, applies
// candidate mutations, and registers successful variants into the registry.
//
// Lifecycle:
//
//	Propose (mutation brief)
//	  → Spawn candidate (new agent variant)
//	  → Validate against baseline + replay
//	  → Promote or Reject
//	  → If promote: register in internal/orchestrator.AgentRegistry
//	  → If reject: log + archive spawn record to ledger.SpawnRecord
//
// Spawn records are append-only (ledger) and provide the audit trail for
// agent lineage. See docs/evolution-loop.md for the full evolution contract.
//
// Maturity: evolving
package spawning
