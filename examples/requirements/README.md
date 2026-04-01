# Example requirements sets

- `easy-common.txt`: first smoke test for common libraries that should exercise upload, planning, build queueing, wheel publication, and log streaming with minimal native build risk.
- `mixed-common-native.txt`: broader first-pass validation set with both easy libraries and common native-extension packages such as `numpy`, `pandas`, and `scikit-learn`.

Suggested first pass:

```bash
API_BASE=http://<control-plane-host>:8080 \
UI_TOKEN=<ui-token> \
REQ_FILE=examples/requirements/easy-common.txt \
./scripts/upload-requirements.sh
```

Suggested second pass:

```bash
API_BASE=http://<control-plane-host>:8080 \
UI_TOKEN=<ui-token> \
REQ_FILE=examples/requirements/mixed-common-native.txt \
./scripts/upload-requirements.sh
```
