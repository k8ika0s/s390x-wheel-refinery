# Diagram Index

Mermaid diagrams live in this directory. They are referenced from docs and can be rendered by any Mermaid-compatible viewer.

## System diagrams
- `overall-system.mmd`: High-level system overview and major services.
- `automatic-build-workflow.mmd`: Build execution flow from queue to repair.
- `auto-fix-decision-flow.mmd`: Auto-fix decision path and retry logic.
- `repair-pipeline.mmd`: Repair stage and artifact handling.
- `automation-ux-visibility.mmd`: Where automation data appears in the UI.
- `queue-status-lifecycle.mmd`: Pending input → plan → build status transitions.
- `log-streaming.mmd`: Live log streaming path (worker → control-plane → UI).

## Component views
- `component-api-control-plane.mmd`: Control-plane API component view.
- `component-builder-worker.mmd`: Worker + builder container roles.
- `component-container-compose.mmd`: Compose runtime layout.
- `component-web-ui.mmd`: UI entry points and data surfaces.
- `component-hint-catalog.mmd`: Hint catalog lifecycle.
- `component-history-manifest.mmd`: Event + manifest storage flow.
- `component-retry-queue.mmd`: Legacy retry queue flow.
- `component-scanner-resolver.mmd`: Resolver/scanner component overview.
- `component-ci-cd.mmd`: CI/CD pipeline summary.

## Workflow views
- `workflow-retry-hint.mmd`: Retry + hint learning loop.
- `deployment-views.mmd`: Deployment/runtime perspectives.

## Notes
- Mermaid lint rejects parentheses in node labels; avoid `()` in bracketed labels.
