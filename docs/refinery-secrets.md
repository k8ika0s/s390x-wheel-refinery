# Refinery secrets

This document lists every secret or credential that can be used by the system today, plus where it is used. Use this as a checklist
when wiring secret management or rotating credentials in production.

Keep in mind:
- Do not commit secret values to git.
- Prefer environment files or a secret manager, and inject at runtime.
- UI tokens are stored in the browser local storage by the UI; treat them as bearer secrets.

## Control-plane secrets

- UI_TOKEN
  - Purpose: authorizes UI-initiated mutations (enqueue, delete, settings updates).
  - Where: control-plane env; UI sends as X-UI-Token header.
  - Notes: if unset, UI write endpoints are open.

- WORKER_TOKEN
  - Purpose: authorizes worker writes (build status, logs, events, heartbeats).
  - Where: control-plane env; worker sends as X-Worker-Token header.

- POSTGRES_DSN
  - Purpose: database connection string (includes user and password).
  - Where: control-plane env.
  - Notes: DSN contains credentials and should be treated as secret.

- POSTGRESQL_USER / POSTGRESQL_PASSWORD / POSTGRESQL_ADMIN_PASSWORD
  - Purpose: containerized postgres user and admin credentials.
  - Where: podman-compose.yml for local/dev; use real secrets in production.

- OBJECT_STORE_ACCESS_KEY / OBJECT_STORE_SECRET_KEY
  - Purpose: object storage credentials for inputs and artifacts.
  - Where: control-plane env.

- MINIO_ROOT_USER / MINIO_ROOT_PASSWORD
  - Purpose: MinIO root credentials (when using built-in MinIO).
  - Where: podman-compose.yml for local/dev.

- WORKER_WEBHOOK_URL
  - Purpose: optional worker trigger webhook.
  - Where: control-plane env.
  - Notes: if the webhook URL embeds a token, treat it as a secret.

## Worker secrets

- WORKER_TOKEN
  - Purpose: authorizes worker writes to the control-plane.
  - Where: worker env; sent as X-Worker-Token header.

- CONTROL_PLANE_TOKEN
  - Purpose: same as WORKER_TOKEN, explicit name for worker -> control-plane.
  - Where: worker env.

- INDEX_USERNAME / INDEX_PASSWORD
  - Purpose: credentials for private package indexes.
  - Where: worker env; used for planning/resolution.

- CAS_REGISTRY_USER / CAS_REGISTRY_PASSWORD
  - Purpose: CAS registry authentication (push/pull).
  - Where: worker env.

- OBJECT_STORE_ACCESS_KEY / OBJECT_STORE_SECRET_KEY
  - Purpose: object storage access for artifacts and inputs.
  - Where: worker env.

## Optional infrastructure credentials

These are not required for local/dev but should be handled as secrets if used:

- KAFKA_BROKERS (may embed credentials or SASL parameters)
- REDIS_URL (may embed passwords)
- CAS_REGISTRY_URL (if auth is embedded in URL)

## Operational guidance

- Rotate UI_TOKEN and WORKER_TOKEN regularly; see docs/token-rotation.md.
- Keep DSN passwords out of logs and error messages.
- Use separate credentials per environment (dev, staging, prod).
- If you enable CORS for a public UI, ensure UI_TOKEN is set and protected.
