# Changelog

All notable changes are recorded here. This project follows Semantic Versioning once the first release is tagged.

## Unreleased

### Security

- Added exact-origin CORS, loopback-by-default binding, reverse-proxy API authentication, sidecar authentication, request and concurrency limits, secret redaction, and scoped broker-credential encryption.
- Removed browser-managed LLM keys and the arbitrary model-discovery proxy; custom AI gateways now use a server-side HTTPS allowlist.
- Added dependency vulnerability checks, pinned Python dependency hashes, hardened containers, and production deployment guidance.

### Reliability

- Made trade-plan execution and broker import idempotent under concurrent requests.
- Added a one-shot production migration job with version records and advisory-lock serialization, plus readiness checks, bounded list responses, checkpoint retention, and reconnectable analysis capacity limits.
