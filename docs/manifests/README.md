# Manifests

This directory holds audit manifests for debugging, design-review follow-ups, and investigation tasks that lead to implementation.

## Creating a Manifest

Copy `TEMPLATE.md` and name the file `YYYY-MM-DD-<short-audit-name>.md`.

## Verification

Run the external verifier before declaring a manifest complete:

```bash
./scripts/verify-manifest.sh docs/manifests/YYYY-MM-DD-<short-audit-name>.md
```

The verifier checks:

- Every invariant row with `Status: done` has non-empty evidence in the `Notes` column.
- Phase D close-out items are populated when any invariant is done.

## Commit & PR Discipline

- Commit format: `<type>(manifest): #<ID> <short description>`
- PR body must reference the manifest: `See docs/manifests/YYYY-MM-DD-<short-audit-name>.md`
- No direct push to `main`.

## Documentation Governance

See `docs/documentation-standard.md` for `.omo/` lifecycle rules. After merge:

- Delete related `.omo/plans/` files.
- Promote stable `.omo/briefs/` content to `docs/specs/` or `docs/guides/`, then delete the `.omo/` copy.
