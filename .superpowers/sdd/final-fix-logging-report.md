# OAuth callback access-log fix

## Scope

- Replaced `gin.Default()` with explicit Gin middleware registration.
- Retained Gin access logging for ordinary routes and Gin recovery handling.
- Skipped only access-log entries whose path is the OAuth callback route, so callback `code` and `state` query values cannot be emitted by Gin's default formatter.
- Did not change OAuth handler or authorization business logic.

## Regression coverage

`TestSafeAccessLoggerOmitsOAuthCallbackQueryAndLogsOtherRoutes` captures Gin access-log output and verifies that a callback URL containing `code` and `state` produces no secret-bearing log data, while `/api/v1/health?probe=normal` is still logged.

## Verification

- Red: the focused test initially failed to build because `safeAccessLogger` did not exist.
- Green: `cd backend && go test ./cmd/server -run TestSafeAccessLoggerOmitsOAuthCallbackQueryAndLogsOtherRoutes -count=1` passed.
- Full backend: `cd backend && go test ./...` passed.
