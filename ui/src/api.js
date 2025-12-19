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

export const API_BASE_DEFAULT = inferApiBase();

export const getApiBase = () => {
  if (typeof window !== "undefined") {
    const stored = window.localStorage.getItem("refinery_api_base");
    if (stored) return stored;
  }
  return API_BASE_DEFAULT;
};

const jsonHeaders = (token) => ({
  "Content-Type": "application/json",
  ...(token ? { "X-Worker-Token": token } : {}),
});

const parseError = async (resp) => {
  const text = await resp.text();
  let message = text || resp.statusText;
  let details;
  if (text) {
    try {
      const data = JSON.parse(text);
      if (data?.error) {
        message = data.error;
      }
      if (data?.details) {
        details = data.details;
      }
    } catch {
      // ignore non-json error payloads
    }
  }
  const err = new Error(message);
  err.status = resp.status;
  if (details) {
    err.details = details;
  }
  return err;
};

async function request(path, options = {}, token) {
  const target = joinBasePath(getApiBase(), path);
  const resp = await fetch(target, {
    ...options,
    headers: {
      ...jsonHeaders(token),
      ...(options.headers || {}),
    },
  });
  if (!resp.ok) {
    throw await parseError(resp);
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
  const [summary, recent, failures, slowest, queue] = await Promise.all([
    request("/api/summary", {}, token),
    request("/api/recent?limit=25", {}, token),
    request("/api/top-failures?limit=10", {}, token),
    request("/api/top-slowest?limit=10", {}, token),
    request("/api/queue", {}, token),
  ]);
  const metrics = await request("/api/metrics", {}, token).catch(() => null);
  return { summary, recent, failures, slowest, queue, metrics };
}

export function fetchSummary(token) {
  return request("/api/summary", {}, token);
}

export function fetchTopFailures(limit = 10, token) {
  return request(`/api/top-failures?limit=${limit}`, {}, token);
}

export function fetchTopSlowest(limit = 10, token) {
  return request(`/api/top-slowest?limit=${limit}`, {}, token);
}

export function fetchQueue(token) {
  return request("/api/queue", {}, token);
}

export function fetchMetrics(token) {
  return request("/api/metrics", {}, token);
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

export function deletePendingInput(id, token) {
  return request(`/api/pending-inputs/${id}`, { method: "DELETE" }, token);
}

export function restorePendingInput(id, token) {
  return request(`/api/pending-inputs/${id}/restore`, { method: "POST" }, token);
}

export function clearPendingInputs(status = "pending", token) {
  const params = new URLSearchParams();
  if (status) params.set("status", status);
  const qs = params.toString();
  return request(qs ? `/api/pending-inputs/clear?${qs}` : "/api/pending-inputs/clear", { method: "POST" }, token);
}

export async function uploadRequirements(file, token) {
  const fd = new FormData();
  fd.append("file", file);
  const headers = token ? { "X-Worker-Token": token } : undefined;
  const resp = await fetch(joinBasePath(getApiBase(), "/api/requirements/upload"), {
    method: "POST",
    body: fd,
    headers,
  });
  if (!resp.ok) {
    throw await parseError(resp);
  }
  return resp.json();
}

export async function uploadWheel(file, token) {
  const fd = new FormData();
  fd.append("file", file);
  const headers = token ? { "X-Worker-Token": token } : undefined;
  const resp = await fetch(joinBasePath(getApiBase(), "/api/wheels/upload"), {
    method: "POST",
    body: fd,
    headers,
  });
  if (!resp.ok) {
    throw await parseError(resp);
  }
  return resp.json();
}

export function fetchSettings(token) {
  return request("/api/settings", {}, token);
}

export function fetchHints({ limit = 10, offset = 0, query = "" } = {}, token) {
  const params = new URLSearchParams();
  if (limit) params.set("limit", limit);
  if (offset) params.set("offset", offset);
  if (query) params.set("q", query);
  const qs = params.toString();
  return request(qs ? `/api/hints?${qs}` : "/api/hints", {}, token);
}

export function updateSettings(body, token) {
  return request("/api/settings", { method: "POST", body: JSON.stringify(body) }, token);
}

export function enqueueBuildsFromPlan(planId, token) {
  return request(`/api/plan/${planId}/enqueue-builds`, { method: "POST" }, token);
}

export function clearPlanQueue(token) {
  return request("/api/plan-queue/clear", { method: "POST" }, token);
}

export function fetchPlans(limit = 20, token) {
  const params = new URLSearchParams();
  params.set("limit", limit);
  return request(`/api/plans?${params.toString()}`, {}, token);
}

export function deletePlans(id, token) {
  const params = new URLSearchParams();
  if (id) params.set("id", id);
  const qs = params.toString();
  return request(qs ? `/api/plans?${qs}` : "/api/plans", { method: "DELETE" }, token);
}

export function fetchPlan(planId, token) {
  return request(`/api/plan/${planId}`, {}, token);
}

export function createHint(hint, token) {
  return request("/api/hints", { method: "POST", body: JSON.stringify(hint) }, token);
}

export function updateHint(id, hint, token) {
  return request(`/api/hints/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify(hint) }, token);
}

export function deleteHint(id, token) {
  return request(`/api/hints/${encodeURIComponent(id)}`, { method: "DELETE" }, token);
}

export async function bulkUploadHints(file, token) {
  const fd = new FormData();
  fd.append("file", file);
  const headers = token ? { "X-Worker-Token": token } : undefined;
  const resp = await fetch(joinBasePath(getApiBase(), "/api/hints/bulk"), {
    method: "POST",
    body: fd,
    headers,
  });
  if (!resp.ok) {
    throw await parseError(resp);
  }
  return resp.json();
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

export function fetchBuilds({ status, limit = 200 } = {}, token) {
  const params = new URLSearchParams();
  if (status) params.set("status", status);
  params.set("limit", limit);
  return request(`/api/builds?${params.toString()}`, {}, token);
}

export function clearBuilds(status, token) {
  const params = new URLSearchParams();
  if (status) params.set("status", status);
  const qs = params.toString();
  return request(qs ? `/api/builds?${qs}` : "/api/builds", { method: "DELETE" }, token);
}
