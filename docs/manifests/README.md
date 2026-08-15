# Manifests

This directory holds **permanent governance templates** for the invariant tracker manifest mechanism.

**Individual audit/repair manifests are transient artifacts — create them in `.omo/manifests/` instead.**

## Creating a Manifest

Copy `TEMPLATE.md` to `.omo/manifests/` and name it `YYYY-MM-DD-<short-audit-name>.md`.

## Verification

Run the external verifier before declaring a manifest complete:

```bash
./scripts/verify-manifest.sh .omo/manifests/YYYY-MM-DD-<short-audit-name>.md
```

The verifier checks:

- Every invariant row with `Status: done` has non-empty evidence in the `Notes` column.
- Phase D close-out items are populated when any invariant is done.

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
