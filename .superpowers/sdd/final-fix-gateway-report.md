# Local gateway contract fixes

## Scope

- The Go HTTP gateway client now treats the sidecar discovery result as account metadata only. It always persists the already validated, server-derived loopback settings and ignores any sidecar-returned credential fields.
- Gateway connections use the public `local_gateway` method value in Go persistence/DTOs and the frontend contract; the response DTO normalizes legacy stored `gateway` rows during the transition.
- `BROKER_FUTU_ACCOUNT` and `BROKER_IBKR_ACCOUNT` are server-only configuration. The Go service forwards a configured account to the local sidecar.
- IBKR now chooses an explicitly configured discovered account, accepts an unconfigured session only when exactly one account exists, and rejects both ambiguity and configured-account mismatches. Its adapter remains `readonly=True`.

## Regression coverage

- `TestDetectGatewayPostsOnlyConfiguredSettings` simulates a malicious sidecar response containing a remote host and asserts the identity retains the original loopback configuration.
- Gateway configuration tests cover both broker account environment variables; the service test verifies the IBKR account is sent to the sidecar.
- Sidecar tests cover IBKR configured selection, sole-account fallback, ambiguous discovery, and mismatched configuration.
- Backend handler and frontend component tests assert `connection_method: local_gateway`, including response normalization for legacy stored rows.

## TDD evidence

- Red: the adversarial Go test returned `sidecar.example.com:443`; the configuration tests did not compile because `BrokerGatewayConfig.Account` did not exist; the handler persisted `gateway`; and the sidecar selection tests failed because `_select_ibkr_account` was absent.
- Green: focused Go, sidecar, and frontend tests passed after the narrowly scoped changes.

## Full verification

- `cd backend && go test ./... -count=1` passed.
- `cd sidecar && ./.venv/bin/python -m unittest` passed (53 tests; the expected mocked stream-failure traceback was emitted by its test).
- `cd frontend && node_modules/.bin/vitest run && node_modules/.bin/tsc --noEmit -p tsconfig.json --pretty` passed (8 Vitest tests and TypeScript validation).
