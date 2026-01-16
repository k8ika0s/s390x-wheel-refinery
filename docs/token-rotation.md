# Token scoping and rotation

This project uses two distinct tokens:

- **UI token** (`UI_TOKEN`): protects user-initiated mutations such as
  enqueueing, clearing queues, updating hints, and changing settings.
  The UI sends this token as `X-UI-Token`.
- **Worker token** (`WORKER_TOKEN`): protects worker posts such as build status
  updates, log uploads, heartbeats, and manifest writes. The worker sends this
  token as `X-Worker-Token`.

If a token is unset, the associated endpoints are open. For production, set
both tokens and rotate them regularly.

## Rotation procedure

1) Generate new tokens for UI and worker.
2) Update control-plane environment:
   - `UI_TOKEN=<new-ui-token>`
   - `WORKER_TOKEN=<new-worker-token>`
3) Update worker environment:
   - `CONTROL_PLANE_TOKEN=<new-worker-token>`
4) Restart control-plane and worker containers.
5) Update the UI settings panel with the new UI token.
6) Verify:
   - UI actions succeed (enqueue, clear queue, update settings).
   - Worker heartbeats and log streams continue.

## Notes

- UI token and worker token are intentionally separate to follow
  least-privilege.
- Consider time-boxed tokens and regular rotation as part of on-call runbooks.
