# AI Git Workflow Scripts

This directory contains helper scripts for the AI agent to follow proper Git workflow.

## Why These Scripts Exist

To prevent AI from accidentally pushing to `main` branch, these scripts enforce:
1. Feature branch creation from latest `main`
2. Proper branch naming (`feat/`, `fix/`, `refactor/`)
3. Automated PR creation with structured template

## Usage

### Starting a new feature

```bash
bash scripts/git-workflow/ai-feature-branch.sh feat "add channel adapter"
```

This will:
- Pull latest `main`
- Create branch `feat/add-channel-adapter`
- Checkout the new branch

### Creating a PR

```bash
bash scripts/git-workflow/ai-create-pr.sh "feat(apigateway): add channel adapter"
```

This will:
- Verify you're NOT on `main`
- Stage and commit all changes
- Push to origin
- Create PR via `gh CLI`

## Branch Naming Rules

| Type | Prefix | Example |
|------|--------|---------|
| New feature | `feat/` | `feat/geopolitical-adapter` |
| Bug fix | `fix/` | `fix/channel-health-race` |
| Refactor | `refactor/` | `refactor/bootstrap-cleanup` |
| Documentation | `docs/` | `docs/api-examples` |
| Tests | `test/` | `test/adapter-coverage` |

## Commit Message Format

```
type(scope): description

[optional body]

[optional footer]
```

Types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`

## CI Requirements

Before creating PR, ensure:
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] `gofmt -l .` returns empty
- [ ] `staticcheck ./...` passes
- [ ] Coverage ≥ 60%
