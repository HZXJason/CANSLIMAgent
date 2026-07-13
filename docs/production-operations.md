# Production operations baseline

## Required boundary

Run the stack behind an HTTPS identity-aware reverse proxy. Keep the compose frontend port on loopback, overwrite client-supplied identity/internal-token headers, enforce user-level rate limits, and restrict operator access to the host and database. The application-level API token is a proxy-to-backend credential, not end-user authentication.

Use separate secrets for `POSTGRES_PASSWORD`, `API_AUTH_TOKEN`, `SIDECAR_SHARED_SECRET`, broker credential encryption, and every external provider. Store them in a managed secret store rather than source control or image layers.

## Deploy and rollback

1. Build from a reviewed commit after CI is green; record the commit and image digests.
2. Back up PostgreSQL before a schema-changing release. The compose `migrate` job must complete successfully before any backend replica starts; API replicas run with migrations disabled.
3. Start the internal services, verify sidecar `/health`, then backend `/api/v1/health`, then the proxy path.
4. Exercise one read-only stock request and a non-billable or low-cost AI smoke test.
5. Roll back to the prior image digests if readiness fails. Restore the database only when a migration is proven incompatible; do not mix an old binary with an unknown newer schema.

## Backup and recovery

- Take encrypted PostgreSQL backups at least daily and before releases; keep copies outside the deployment host.
- Test restoration into an isolated environment on a schedule. A backup that has not been restored is not considered verified.
- Treat the broker encryption key as recovery-critical. Back it up separately under access control; database backups are unusable for broker reconnection without it.
- AI checkpoints are disposable and expire by policy; do not treat them as the system of record.

## Rotation and incident response

- Rotate proxy/sidecar shared secrets by deploying coordinated configuration changes during a maintenance window. Rotate external API keys immediately after suspected exposure.
- Broker encryption-key rotation requires a controlled decrypt-and-reencrypt migration; do not replace the key in place while encrypted rows still exist.
- On suspected compromise, isolate the service, preserve redacted logs and image/commit digests, revoke external and broker tokens, rotate secrets, assess database access, and notify affected operators before restoring service.

## Monitoring

Alert on readiness failures, HTTP 401/413/429/5xx spikes, analysis-capacity exhaustion, database pool saturation, disk/volume growth, provider quota errors, and repeated broker authorization failures. Forward the structured Go access log to a retention-controlled log system and correlate by `X-Request-ID`.
