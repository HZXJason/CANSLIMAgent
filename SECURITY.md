# Security Policy

## Reporting a vulnerability

Do not open a public issue for suspected vulnerabilities or exposed secrets. Use GitHub's private vulnerability reporting for this repository under **Security → Advisories → Report a vulnerability**. Include affected versions, reproduction steps, impact, and any suggested mitigation.

Do not access data that does not belong to you, disrupt a running deployment, or retain credentials discovered during research. We will acknowledge a complete report as soon as practical and coordinate disclosure after a fix is available.

## Supported versions

Security fixes are applied to the latest commit on `main`. Deployments should track tagged releases built by CI and should not use an older commit after a security update has shipped.

## Deployment boundary

The browser must reach the Go API through a trusted identity-aware reverse proxy. The proxy owns TLS, user authentication, rate limiting, and injection of `X-Internal-API-Token`; the token must never be embedded in frontend assets. Go and the Python sidecar use a separate `SIDECAR_SHARED_SECRET`. LLM and broker credentials are server-side secrets only.
