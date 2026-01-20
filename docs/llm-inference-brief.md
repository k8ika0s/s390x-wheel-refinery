# LLM inference brief

This brief describes the optional inference layer that proposes build-fix hints and recipes from build logs. The app treats the LLM as a black box: give it a structured prompt and a strict JSON response contract, regardless of the provider.

## Goal

Provide a small, fast, and accurate model that can translate build errors into concrete fix steps (recipes) while staying safe and predictable. The worker only uses inference when it cannot match a known hint or heuristic pattern, and it still applies confidence gating and rate limits. If inference is unavailable, the pipeline continues without it.

## API contract

The worker calls an OpenAI-compatible chat completions endpoint. Any provider that implements the same JSON shape will work.

### Request

- POST to `INFER_URL`
- Optional `Authorization: Bearer <INFER_TOKEN>`
- JSON body:

```json
{
  "model": "<optional model name>",
  "temperature": 0.2,
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."}
  ]
}
```

### Response

The response must contain a single JSON object in `choices[0].message.content` (or in `choices[0].text` for legacy). The worker will parse direct JSON, JSON inside a code fence, or a minimal wrapper.

Required fields:

```json
{
  "pattern": "<string>",
  "confidence": 0.0,
  "reason_code": "<string>",
  "summary": "<string>",
  "recipes": {
    "apt": ["libssl-dev"],
    "dnf": ["openssl-devel"],
    "pip": ["setuptools"],
    "env": ["CFLAGS=-O2"]
  },
  "notes": "<string>",
  "tags": ["openssl", "missing_header"]
}
```

If no useful fix is possible, return an empty `pattern` or an empty `recipes` map. The worker treats either as "no inference" and continues without applying anything.

## Prompt inputs

The worker provides the following fields to the model:

- Package name and version.
- Python version/tag and platform tag.
- Existing recipes already applied.
- A log excerpt (tail of recent stderr/stdout).

The system prompt enforces strict JSON output and safe fixes.

## Reason codes

Standardize reason codes so the UI can display consistent chips and the analytics pipeline can bucket failures. Recommended values:

- `missing_header`
- `missing_library`
- `missing_pkg_config`
- `missing_cmake`
- `missing_python_module`
- `missing_tool`
- `missing_rust_toolchain`
- `linker_error`
- `compiler_error`
- `network_error`
- `other`

## Model suggestions

Pick a small, code-capable model that performs well on build logs and toolchain errors. Suggested candidates (all have strong instruct variants):

- Llama 3.1 8B Instruct (good balance of accuracy vs. cost)
- Mistral 7B Instruct (fast, concise)
- DeepSeek-Coder 6.7B Instruct (strong on error reasoning)
- CodeLlama 7B Instruct (solid baseline)

Quantization:
- 4-bit or 8-bit quantization is typically sufficient for this task.
- Prefer 8-bit if you want higher stability on nuanced logs.

Serving:
- Any OpenAI-compatible server is acceptable (OpenAI API, vLLM, TGI, LM Studio, etc.).
- The worker only needs the URL and optional token.

## Data and evaluation

Training or fine-tuning data should emphasize build logs and packaging workflows. Useful sources:

- PEP 517/518 build logs (pip build isolation)
- manylinux build failures and repair logs
- pkg-config / CMake missing dependency errors
- compiler and linker errors (gcc, clang, ld)
- Rust/maturin failures
- Python import errors

Evaluation metrics:

- Fix accuracy: percent of failures resolved by applied recipe.
- False positives: recipes that add unnecessary packages.
- Time-to-fix: average attempts to first success.
- Safety rate: percent of hints blocked by confidence gating.

## Operational guidance

- Inference is optional and safe to disable (`INFER_ENABLED=false`).
- Set conservative confidence thresholds to avoid noisy fixes.
- Prefer minimal recipes (single package) unless the log clearly indicates toolchain gaps.
- Keep the model small to reduce latency and keep builds fast.

## Security and privacy

- Logs may contain paths, environment hints, or package names. Ensure the inference provider can handle this data.
- Never log or store the inference token in plaintext.
- If you use a hosted API, ensure the data retention policy is acceptable for build logs.
