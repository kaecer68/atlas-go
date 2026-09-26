# Manifests

This directory holds **permanent governance templates** for the invariant tracker manifest mechanism.

**Individual audit/repair manifests are transient artifacts — create them in `.omo/manifests/` instead.**

## Creating a Manifest

Copy `TEMPLATE.md` to `.omo/manifests/` and name it `YYYY-MM-DD-<short-audit-name>.md`.

## Verification

**There is no automated verifier.** `scripts/verify-manifest.sh` was deleted on 2026-09-27 because it could not fail:

- Its `Status == done` → non-empty `Notes` check never fired. The awk trim used a two-argument `gsub(/^[ \t]+|[ \t]+$/, "")`, which rewrites `$0`, not the field, so the extracted status kept its surrounding spaces (`" done "`), the `!= "done"` test was always true, and **every row was skipped**. A manifest row with `Status: done` and an empty `Notes` column still returned `OK` with exit 0.
- Its input lives in gitignored `.omo/`, so a clean clone has nothing to verify in the first place.

Completion is therefore reviewed by the owner, against the version-controlled records in `docs/operations/remediation-manifest.md` and `docs/operations/FOLLOWUPS.md`. The lifecycle is described in `docs/documentation-standard.md` §Manifest 生命週期.

## Commit & PR Discipline

- Commit format: `<type>(manifest): #<ID> <short description>`
- PR body must reference the manifest path
- No direct push to `main`.

## Manifest Lifecycle & Promotion

See `docs/documentation-standard.md` §`docs/manifests/` 治理 for the full lifecycle. After completion:

1. Run `./scripts/cleanup-manifests.sh --stale-days 7` to check for stale manifests
2. **Archive** (→ `.omo/audit/`, harness-private): if the manifest documents a significant bug with teaching value
3. **Promote** (→ `docs/specs/<topic>-spec.md`): if the manifest contains stable spec-level invariants
4. **Delete**: if it's a simple repair tracker with no long-term value

## Documentation Governance

See `docs/documentation-standard.md` for `.omo/` lifecycle rules. After merge:

- Delete related `.omo/manifests/` completed manifests (if no archive/promotion needed)
- Delete related `.omo/plans/` files.
- Promote stable content to `docs/specs/` or `docs/guides/`, then delete the `.omo/` copy.
