export const API_BASE = import.meta.env.VITE_API_BASE || "";

const jsonHeaders = (token) => ({
  "Content-Type": "application/json",
  ...(token ? { "X-Worker-Token": token } : {}),
});

async function request(path, options = {}, token) {
  const resp = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      ...jsonHeaders(token),
      ...(options.headers || {}),
    },
  });
  if (!resp.ok) {
    const text = await resp.text();
    const err = new Error(text || resp.statusText);
    err.status = resp.status;
    throw err;
  }
  const ct = resp.headers.get("content-type") || "";
  if (ct.includes("application/json")) {
    return resp.json();
  }
  return resp.text();
}

const defaultPyTag = import.meta.env.VITE_DEFAULT_PYTHON_TAG || "cp311";
const defaultPlatform = import.meta.env.VITE_DEFAULT_PLATFORM_TAG || "manylinux2014_s390x";

export async function fetchDashboard(token) {
  const [summary, recent, failures, slowest, queue, hints] = await Promise.all([
    request("/api/summary", {}, token),
    request("/api/recent?limit=25", {}, token),
    request("/api/top-failures?limit=10", {}, token),
    request("/api/top-slowest?limit=10", {}, token),
    request("/api/queue", {}, token),
    request("/api/hints", {}, token),
  ]);
  const metrics = await request("/api/metrics", {}, token).catch(() => null);
  return { summary, recent, failures, slowest, queue, hints, metrics };
}

export function triggerWorker(token) {
  return request("/api/worker/trigger", { method: "POST" }, token);
}

export function enqueueRetry(name, version, token, pythonTag = defaultPyTag, platformTag = defaultPlatform) {
  const body = JSON.stringify({
    package: name,
    version: version || "latest",
    python_tag: pythonTag,
    platform_tag: platformTag,
  });
  return request("/api/queue/enqueue", { method: "POST", body }, token);
}

export function clearQueue(token) {
  return request("/api/queue/clear", { method: "POST" }, token);
}

export function setCookieToken(token) {
  return request(`/api/session/token?token=${encodeURIComponent(token)}`, { method: "POST" }, token);
}

export function fetchPackageDetail(name, token, limit = 50) {
  return Promise.all([
    request(`/api/package/${encodeURIComponent(name)}`, {}, token),
    request(`/api/variants/${encodeURIComponent(name)}?limit=50`, {}, token),
    request(`/api/failures?name=${encodeURIComponent(name)}&limit=${limit}`, {}, token),
    request(`/api/recent?package=${encodeURIComponent(name)}&limit=${limit}`, {}, token),
  ]).then(([summary, variants, failures, events]) => ({ summary, variants, failures, events }));
}

export function fetchLog(name, version, token) {
  return request(`/api/logs/${encodeURIComponent(name)}/${encodeURIComponent(version)}`, {}, token);
}

export function fetchRecent({ limit = 25, packageFilter, status }, token) {
  const params = new URLSearchParams();
  params.set("limit", limit);
  if (packageFilter) params.set("package", packageFilter);
  if (status) params.set("status", status);
  return request(`/api/recent?${params.toString()}`, {}, token);
}
