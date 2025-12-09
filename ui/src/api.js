const API_BASE = import.meta.env.VITE_API_BASE || "";

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
    throw new Error(text || resp.statusText);
  }
  const ct = resp.headers.get("content-type") || "";
  if (ct.includes("application/json")) {
    return resp.json();
  }
  return resp.text();
}

export async function fetchDashboard(token) {
  const [summary, recent, failures, slowest, queue] = await Promise.all([
    request("/api/summary", {}, token),
    request("/api/recent?limit=25", {}, token),
    request("/api/top-failures?limit=10", {}, token),
    request("/api/top-slowest?limit=10", {}, token),
    request("/api/queue", {}, token),
  ]);
  return { summary, recent, failures, slowest, queue };
}

export function triggerWorker(token) {
  return request("/api/worker/trigger", { method: "POST" }, token);
}

export function enqueueRetry(name, version, token) {
  const body = JSON.stringify({ version });
  return request(`/package/${encodeURIComponent(name)}/retry`, { method: "POST", body }, token);
}

export function setCookieToken(token) {
  return request(`/api/session/token?token=${encodeURIComponent(token)}`, { method: "POST" }, token);
}
