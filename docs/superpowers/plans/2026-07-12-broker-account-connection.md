# Broker Account Connection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace browser-entered broker credentials with four broker cards that securely begin OAuth or detect an already logged-in local gateway, then display the bound account name.

**Architecture:** Go remains the public `/api/v1` boundary. It owns OAuth state, callback validation, encrypted credentials and connection metadata. The Python sidecar only discovers Futu/IBKR accounts on loopback and performs the existing read-only sync. React receives state-only DTOs and navigates only to a Go-returned OAuth URL.

**Tech Stack:** Go 1.22, Gin, GORM/Postgres, AES-256-GCM, FastAPI/Python, React 18, TypeScript, Vite, Tailwind, Vitest and React Testing Library.

## Global Constraints

- Keep Go `/api/v1` public and all sidecar traffic loopback-only.
- Never return token, key, OAuth state, client secret or gateway settings to the browser.
- Keep read-only sync, preview and explicit import semantics; no order actions.
- OAuth appears only after server-side developer-app config; never emulate a broker login.
- Futu/IBKR use only a server-configured, authenticated OpenD/TWS/Gateway session.
- Every behavior follows RED, GREEN, then a narrow commit.

---

## File Structure

| File | Responsibility |
| --- | --- |
| `backend/internal/models/broker_connection.go` | Broker metadata and one-time authorization session. |
| `backend/config/config.go` | Server-only OAuth and local-gateway configuration. |
| `backend/internal/services/broker_authorization.go` | OAuth and loopback-gateway clients. |
| `backend/internal/handlers/brokers.go` | Start/callback/detect lifecycle and secure persistence. |
| `sidecar/broker_sync.py`, `sidecar/main.py` | Read-only Futu/IBKR account discovery endpoint. |
| `frontend/src/types/broker.ts`, `frontend/src/lib/api.ts` | Browser-safe DTOs and API calls. |
| `frontend/src/lib/brokerConnectionView.ts` | Pure card state helper. |
| `frontend/src/components/settings/BrokerConnections.tsx` | Four horizontal broker cards plus existing import preview. |

### Task 1: Add connection state metadata and server configuration

**Files:**
- Modify: `backend/internal/models/broker_connection.go`, `backend/config/config.go`, `backend/internal/db/postgres.go`, `backend/schema.sql`
- Create: `backend/internal/models/broker_connection_test.go`

**Produces:**

```go
type BrokerConnectionStatus string
const ( BrokerConnectionStatusUnconnected = "unconnected"; BrokerConnectionStatusPending = "pending_authorization"; BrokerConnectionStatusConnected = "connected"; BrokerConnectionStatusReconnectRequired = "reconnect_required"; BrokerConnectionStatusGatewayUnavailable = "gateway_unavailable" )
type BrokerOAuthConfig struct { ClientID, ClientSecret, AuthorizeURL, TokenURL, AccountURL, RedirectURL string; Scopes []string }
func (c BrokerOAuthConfig) Enabled() bool
```

- [ ] **Step 1: Write the failing tests**

```go
func TestBrokerConnectionMetadataUsesOnlyKnownStates(t *testing.T) {
    if !models.ValidBrokerConnectionStatus(models.BrokerConnectionStatusConnected) { t.Fatal("connected must be valid") }
    if models.ValidBrokerConnectionStatus("browser_supplied_token") { t.Fatal("unknown state accepted") }
}
func TestBrokerOAuthConfigIsDisabledWithoutClientID(t *testing.T) {
    if (config.BrokerOAuthConfig{}).Enabled() { t.Fatal("empty config must be disabled") }
}
```

- [ ] **Step 2: Confirm RED**

Run: `cd backend && go test ./internal/models ./config -run 'TestBroker(ConnectionMetadata|OAuthConfig)' -v`

Expected: FAIL because the status constants and config method do not exist.

- [ ] **Step 3: Implement the smallest model/schema change**

Extend `BrokerConnection` with `AccountName`, `ConnectionMethod`, `ConnectionStatus`, and optional `AuthorizationExpiresAt`. Add `BrokerAuthorizationSession` with broker, SHA-256 state hash, expiry and `ConsumedAt`; never persist raw state. Load `BROKER_LONGPORT_*`, `BROKER_TIGER_*`, and fixed loopback Futu/IBKR settings from environment. Add the session model to `AutoMigrate` and matching SQL columns/indexes.

- [ ] **Step 4: Confirm GREEN and commit**

Run: `cd backend && go test ./internal/models ./config -run 'TestBroker(ConnectionMetadata|OAuthConfig)' -v`

Expected: PASS.

Commit: `git add backend/internal/models/broker_connection.go backend/internal/models/broker_connection_test.go backend/config/config.go backend/internal/db/postgres.go backend/schema.sql && git commit -m "feat: model broker connection states"`

### Task 2: Build the OAuth and local-gateway authorization service

**Files:**
- Create: `backend/internal/services/broker_authorization.go`, `backend/internal/services/broker_authorization_test.go`
- Modify: `backend/internal/services/broker_sync.go`

**Produces:**

```go
type BrokerAuthorizer interface {
    BeginOAuth(context.Context, string) (BrokerOAuthStart, error)
    CompleteOAuth(context.Context, string, string, string) (BrokerAuthorizationIdentity, error)
    DetectGateway(context.Context, string) (BrokerGatewayIdentity, error)
}
type BrokerOAuthStart struct { AuthorizationURL string; StateHash, VerifierCiphertext, VerifierNonce []byte; ExpiresAt time.Time }
type BrokerAuthorizationIdentity struct { Credentials map[string]string; AccountName string; ExpiresAt *time.Time }
type BrokerGatewayIdentity struct { AccountName string; Credentials map[string]string }
```

- [ ] **Step 1: Write the failing service tests**

```go
func TestBeginOAuthAddsStateAndPKCEChallenge(t *testing.T) {
    start, err := newTestAuthorizer(t).BeginOAuth(context.Background(), models.BrokerLongPort)
    if err != nil { t.Fatal(err) }; u, _ := url.Parse(start.AuthorizationURL)
    if u.Query().Get("state") == "" || u.Query().Get("code_challenge") == "" { t.Fatal("OAuth protections missing") }
    if len(start.StateHash) != sha256.Size || len(start.VerifierCiphertext) == 0 || len(start.VerifierNonce) == 0 { t.Fatal("state or PKCE verifier is not safe to persist") }
}
func TestDetectGatewayRejectsRemoteSidecar(t *testing.T) {
    _, err := services.NewHTTPBrokerGatewayClient("https://broker.example.com").DetectGateway(context.Background(), "futu")
    if err == nil { t.Fatal("remote gateway must be rejected") }
}
```

- [ ] **Step 2: Confirm RED**

Run: `cd backend && go test ./internal/services -run 'Test(BeginOAuth|DetectGateway)' -v`

Expected: FAIL because the authorizer/client does not exist.

- [ ] **Step 3: Implement minimal secure behavior**

Generate random state and separate PKCE verifier; persist only the state hash and encrypt the short-lived verifier. Build configured OAuth URLs with `response_type=code`, `client_id`, `redirect_uri`, `state`, `code_challenge`, `code_challenge_method=S256`, and read-only scopes. Exchange the code server-to-server, query the configured account endpoint, and allow-list `account_name`, `account`, or `data.account_name`. Add `HTTPBrokerGatewayClient` posting only server-derived settings to `/broker/connection-status`; call `validateLoopbackBrokerUpstream` before every request and return a typed unavailable error.

- [ ] **Step 4: Confirm GREEN and commit**

Run: `cd backend && go test ./internal/services -run 'Test(BeginOAuth|DetectGateway|BrokerCredentialVault|BrokerSyncUpstream)' -v`

Expected: PASS.

Commit: `git add backend/internal/services/broker_authorization.go backend/internal/services/broker_authorization_test.go backend/internal/services/broker_sync.go && git commit -m "feat: add secure broker authorization service"`

### Task 3: Replace the credential form endpoint with connection lifecycle routes

**Files:**
- Modify: `backend/internal/handlers/brokers.go`, `backend/cmd/server/main.go`
- Create: `backend/internal/handlers/brokers_test.go`

**Produces:**

```text
POST /api/v1/brokers/:broker/connect -> redirect OAuth action or gateway detection action
GET  /api/v1/brokers/oauth/:broker/callback?code=...&state=... -> settings redirect
POST /api/v1/brokers/:broker/detect -> state-only broker connection DTO
```

- [ ] **Step 1: Write the failing handler tests**

```go
func TestConnectOAuthReturnsRedirectWithoutEchoingCredentials(t *testing.T) {
    router := setupBrokerRouter(t, fakeOAuthAuthorizer{})
    req := httptest.NewRequest(http.MethodPost, "/api/v1/brokers/longport/connect", strings.NewReader(`{"credentials":{"access_token":"must-not-be-read"}}`))
    res := httptest.NewRecorder(); router.ServeHTTP(res, req)
    if res.Code != http.StatusOK || strings.Contains(res.Body.String(), "must-not-be-read") { t.Fatal(res.Body.String()) }
    if !strings.Contains(res.Body.String(), `"action":"redirect"`) { t.Fatal("missing redirect action") }
}
func TestOAuthCallbackRejectsExpiredState(t *testing.T) {
    router, database := setupBrokerRouter(t, fakeOAuthAuthorizer{})
    rawState := "expired-state"; digest := sha256.Sum256([]byte(rawState))
    database.Create(&models.BrokerAuthorizationSession{Broker: models.BrokerLongPort, StateHash: digest[:], ExpiresAt: time.Now().Add(-time.Minute)})
    res := httptest.NewRecorder(); router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/brokers/oauth/longport/callback?code=code&state="+rawState, nil))
    if res.Code != http.StatusBadRequest { t.Fatalf("got %d", res.Code) }
}
```

- [ ] **Step 2: Confirm RED**

Run: `cd backend && go test ./internal/handlers -run 'Test(ConnectOAuth|OAuthCallback)' -v`

Expected: FAIL because the handler still parses credentials and lacks a callback.

- [ ] **Step 3: Implement lifecycle handlers**

Remove `connectBrokerRequest`, `cleanBrokerCredentials`, and `validateBrokerCredentials`. OAuth with missing provider configuration returns `409 OAuth 开发者应用尚未配置`; enabled OAuth creates a ten-minute state record and returns a URL. Gateway connect calls `DetectGateway`, stores only verified account metadata plus encrypted server-derived settings, and returns the state DTO. The callback atomically checks broker/state/expiry/consumed status, decrypts the stored PKCE verifier, consumes the state, passes that verifier to `CompleteOAuth`, and upserts encrypted credentials with `connected`; denial/failure clears pending metadata and redirects to `/settings?broker=<broker>&connection_error=<code>`. Extend List with account/method/status/message and map a sync authorization failure to persisted `reconnect_required`. Keep Import/Delete, with Delete removing pending sessions.

- [ ] **Step 4: Verify routes, wire dependencies and commit**

Run: `cd backend && go test ./internal/handlers -run 'Test(ConnectOAuth|OAuthCallback|Gateway|Broker)' -v && go test ./...`

Expected: PASS for disabled OAuth, callback success/failure/replay, gateway detection, and no credential echo.

Commit: `git add backend/internal/handlers/brokers.go backend/internal/handlers/brokers_test.go backend/cmd/server/main.go && git commit -m "feat: add broker authorization lifecycle"`

### Task 4: Discover local Futu and IBKR accounts in the sidecar

**Files:**
- Modify: `sidecar/broker_sync.py`, `sidecar/main.py`, `sidecar/test_main.py`

**Produces:**

```python
def discover_broker_account(broker: str, credentials: dict[str, str]) -> dict[str, object]: ...
# POST /broker/connection-status => {"account_name": str, "credentials": dict[str, str]}
```

- [ ] **Step 1: Write failing tests**

```python
class BrokerGatewayDiscoveryTest(unittest.TestCase):
    def test_gateway_status_returns_futu_account_name(self):
        with patch.object(broker_sync, "_discover_futu_account", return_value="Futu 12345678"):
            result = broker_sync.discover_broker_account("futu", {"host": "127.0.0.1", "port": "11111"})
        self.assertEqual(result["account_name"], "Futu 12345678")
    def test_gateway_status_rejects_non_loopback_host(self):
        with self.assertRaisesRegex(ValueError, "loopback"):
            broker_sync.discover_broker_account("ibkr", {"host": "broker.example.com", "port": "7497"})
```

- [ ] **Step 2: Confirm RED**

Run: `cd sidecar && .venv/bin/python -m unittest test_main.py`

Expected: FAIL because discovery is absent.

- [ ] **Step 3: Implement read-only discovery**

Use `ipaddress.ip_address(host).is_loopback` before creating any adapter. Futu opens `OpenSecTradeContext`, reads the active/default account and closes it. IBKR connects `readonly=True`, reads `managedAccounts()`, and disconnects. Convert failures to actionable Chinese “未检测到已登录…” errors. Do not import/call order methods.

- [ ] **Step 4: Verify sidecar suite and commit**

Run: `cd sidecar && .venv/bin/python -m unittest test_main.py test_ai_runtime.py test_journal_reflection.py test_analysis_runs.py`

Expected: PASS.

Commit: `git add sidecar/broker_sync.py sidecar/main.py sidecar/test_main.py && git commit -m "feat: detect local broker accounts"`

### Task 5: Expose a state-only frontend contract

**Files:**
- Modify: `frontend/package.json`, `frontend/src/types/broker.ts`, `frontend/src/lib/api.ts`
- Create: `frontend/src/lib/brokerConnectionView.ts`, `frontend/src/lib/brokerConnectionView.test.ts`

**Produces:**

```ts
export type BrokerConnectionStart = { action: 'redirect'; authorization_url: string } | BrokerConnection;
export function startBrokerConnection(broker: BrokerID): Promise<BrokerConnectionStart>;
export function brokerConnectionView(connection: BrokerConnection): { statusLabel: string; actionLabel: string; canSync: boolean };
```

- [ ] **Step 1: Write failing view tests**

```ts
it('shows account name and permits sync when bound', () => {
  expect(brokerConnectionView({ broker: 'longport', connected: true, account_name: 'Longbridge · 12345678', connection_status: 'connected' } as BrokerConnection)).toMatchObject({ statusLabel: '已绑定', actionLabel: '重新连接', canSync: true });
});
it('offers redetection when IBKR is unavailable', () => {
  expect(brokerConnectionView({ broker: 'ibkr', connected: false, connection_status: 'gateway_unavailable' } as BrokerConnection).actionLabel).toBe('重新检测');
});
```

- [ ] **Step 2: Confirm RED**

Run: `cd frontend && pnpm test -- brokerConnectionView.test.ts`

Expected: FAIL because the test command/helper is absent.

- [ ] **Step 3: Implement safe DTOs/helpers**

Add account/method/status/message to `BrokerConnection`; replace `connectBroker` with start/detect API calls; remove credential types. Add Vitest, jsdom, Testing Library and the `test` script. Implement a total status switch in the view helper.

- [ ] **Step 4: Verify and commit**

Run: `cd frontend && pnpm test -- brokerConnectionView.test.ts && pnpm typecheck`

Expected: PASS.

Commit: `git add frontend/package.json frontend/pnpm-lock.yaml frontend/src/types/broker.ts frontend/src/lib/api.ts frontend/src/lib/brokerConnectionView.ts frontend/src/lib/brokerConnectionView.test.ts && git commit -m "feat: define broker connection card state"`

### Task 6: Replace the settings form with four horizontal account cards

**Files:**
- Modify: `frontend/src/components/settings/BrokerConnections.tsx`
- Create: `frontend/src/components/settings/BrokerConnections.test.tsx`

**Consumes:** Task 5 state DTOs and helper; preserves current `BrokerSyncResult` preview/import contract.

- [ ] **Step 1: Write failing component tests**

```tsx
it('renders four broker rows and a bound account', async () => {
  mockFetchConnections.mockResolvedValue([connectedLongport, unconnectedTiger, unavailableFutu, unconnectedIBKR]);
  render(<BrokerConnections />);
  expect(await screen.findByText('长桥 LongPort')).toBeInTheDocument();
  expect(screen.getByText('Longbridge · 12345678')).toBeInTheDocument();
});
it('navigates only to the API OAuth URL', async () => {
  mockStartBrokerConnection.mockResolvedValue({ action: 'redirect', authorization_url: 'https://oauth.example/authorize' });
  render(<BrokerConnections />); await userEvent.click(await screen.findByRole('button', { name: '连接账户' }));
  expect(window.location.assign).toHaveBeenCalledWith('https://oauth.example/authorize');
});
```

- [ ] **Step 2: Confirm RED**

Run: `cd frontend && pnpm test -- BrokerConnections.test.tsx`

Expected: FAIL because the credential form is still rendered.

- [ ] **Step 3: Implement cards, no input fields**

Delete selection/label/credential state, field definitions, Input/Label imports and manual-token copy. Render fixed LongPort, Tiger, Futu and IBKR rows with status, verified account name, last sync and right-side connect/reconnect/redetect/sync/disconnect actions. On redirect call `window.location.assign`; on gateway action refresh status. Read only `broker` and `connection_error` after callback. Retain preview/import and disconnect behavior.

- [ ] **Step 4: Verify and commit**

Run: `cd frontend && pnpm test && pnpm build`

Expected: PASS.

Commit: `git add frontend/src/components/settings/BrokerConnections.tsx frontend/src/components/settings/BrokerConnections.test.tsx && git commit -m "feat: show broker account connection cards"`

### Task 7: Document operator setup and validate the full boundary

**Files:**
- Modify: `backend/.env.example`, `README.md`

- [ ] **Step 1: Add a documentation acceptance checklist**

Document AES key setup, developer-app registration, exact callback URL, OAuth server variables and the requirement to run/login OpenD or TWS/Gateway. Explicitly say client secrets/tokens/private keys never belong in the browser.

- [ ] **Step 2: Verify legacy browser fields are absent**

Run: `rg -n -i 'OAuth Access Token|private key|OpenD 主机|TWS / Gateway 主机' frontend/src/components/settings/BrokerConnections.tsx`

Expected: no matches.

- [ ] **Step 3: Add exact environment documentation**

Add empty `BROKER_LONGPORT_*` and `BROKER_TIGER_*` client/secret/authorize/token/account/redirect/scope variables plus fixed Futu/IBKR loopback values. Explain the disabled-card result when no developer application exists.

- [ ] **Step 4: Run final verification and commit**

Run: `cd backend && go test ./... && cd ../sidecar && .venv/bin/python -m unittest test_main.py test_ai_runtime.py test_journal_reflection.py test_analysis_runs.py && cd ../frontend && pnpm test && pnpm build && cd .. && git diff --check`

Expected: all commands exit 0; no browser credential fields remain; sidecar access is loopback-only.

Commit: `git add README.md backend/.env.example && git commit -m "docs: explain broker account authorization setup"`

## Plan Self-Review

- **Spec coverage:** Tasks 1–3 cover data/state, OAuth callback, encrypted persistence and reconnect. Task 4 covers local Futu/IBKR discovery. Tasks 5–6 cover state-only UI/API and four cards. Task 7 covers setup and full regression verification.
- **Completeness scan:** no deferred implementation marker exists; missing developer applications are an explicit disabled runtime state.
- **Type consistency:** authorization interfaces and connection DTOs are introduced before their handlers/UI consumers.
