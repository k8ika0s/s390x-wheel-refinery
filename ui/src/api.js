const inferApiBase = () => {
  const envBase = import.meta.env.VITE_API_BASE;
  if (envBase) return envBase;
  return "";
};

const normalizeBase = (base) => {
  if (!base) return "";
  return base.endsWith("/") ? base.slice(0, -1) : base;
};

const joinBasePath = (base, path) => {
  const b = normalizeBase(base);
  const p = path.startsWith("/") ? path : `/${path}`;
  if (!b) return p;
  if (b.endsWith("/api") && p.startsWith("/api")) {
    return `${b}${p.replace(/^\/api/, "")}`;
  }
  return `${b}${p}`;
};

export const API_BASE = inferApiBase();

const jsonHeaders = (token) => ({
  "Content-Type": "application/json",
  ...(token ? { "X-Worker-Token": token } : {}),
});

async function request(path, options = {}, token) {
  const target = joinBasePath(API_BASE, path);
  const resp = await fetch(target, {
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

export function fetchPendingInputs(token) {
  return request("/api/pending-inputs", {}, token);
}

export function enqueuePlan(id, token) {
  return request(`/api/pending-inputs/${id}/enqueue-plan`, { method: "POST" }, token);
}

export async function uploadRequirements(file, token) {
  const fd = new FormData();
  fd.append("file", file);
  const headers = token ? { "X-Worker-Token": token } : undefined;
  const resp = await fetch(joinBasePath(API_BASE, "/api/requirements/upload"), {
    method: "POST",
    body: fd,
    headers,
  });
  if (!resp.ok) {
    const text = await resp.text();
    const err = new Error(text || resp.statusText);
    err.status = resp.status;
    throw err;
  }
  return resp.json();
}

export function fetchSettings(token) {
  return request("/api/settings", {}, token);
}

export function updateSettings(body, token) {
  return request("/api/settings", { method: "POST", body: JSON.stringify(body) }, token);
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
