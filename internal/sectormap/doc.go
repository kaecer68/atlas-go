// Package sectormap is the explicit namespace bridge between the sector
// vocabularies that coexist in atlas-go.
//
// Background (issue #1943): atlas-go accumulated six mutually incompatible
// sector vocabularies ("namespaces"). Every consumer (monitoring tree mapper,
// marketdata sector-index providers, sectorallocation weight engines, the
// narrative model, the front-end display map) picked its own key space, so a
// symbol could resolve to different "industries" depending on the code path and
// most consumers could not consume each other's output at all.
//
// This package freezes the canonical namespace (internal/industry's SectorID:
// 20 L1 + 18 L2) and declares, per foreign namespace, an explicit key-by-key
// mapping table. Design rules:
//
//   - No string similarity, no fuzzy matching, no silent aliasing. A key either
//     has a declared mapping or it is reported as StatusUnmapped.
//   - Unmapped keys are first-class data (status + reason + candidate canonical
//     IDs), never a silent drop. Callers must decide what to do with them.
//   - Keys that are not declared in a namespace resolve to StatusUnknown, which
//     signals drift: the upstream config/code changed without updating the
//     table, and the drift tests in this package will fail.
//
// Why a separate leaf package: internal/industry imports internal/marketdata,
// so internal/marketdata cannot import internal/industry. This package imports
// nothing from the project, so both sides can share one table. The typed
// authority stays in internal/industry (SectorID, IsL1, IsL2); the tests in that
// package assert that this package's canonical lists agree with
// industry.L1Sectors()/AllSectors().
package sectormap
