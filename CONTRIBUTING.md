# Contributing

Use a focused branch and keep unrelated working-tree changes out of a pull request. Never commit `.env` files, credentials, broker data, analysis checkpoints, or generated frontend output.

Before opening a pull request, run:

```bash
cd backend && go test -race ./... && go vet ./...
cd ../sidecar && .venv/bin/python -m unittest discover -s . -p 'test_*.py'
cd ../frontend && corepack pnpm run typecheck && corepack pnpm test && corepack pnpm run build
```

Security-sensitive changes require tests for the rejected case as well as the successful case. Changes to API contracts, persisted schemas, encryption, authentication, broker access, or deployment defaults should be called out explicitly in the pull request and reviewed by the code owner.

Report vulnerabilities through the private process in `SECURITY.md`, not through a public issue.
