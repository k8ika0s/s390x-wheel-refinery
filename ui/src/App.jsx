import { useEffect, useRef, useState } from "react";
import { Routes, Route, Link, useParams } from "react-router-dom";
import { API_BASE, enqueueRetry, fetchDashboard, fetchLog, fetchPackageDetail, fetchRecent, setCookieToken, triggerWorker } from "./api";

function Toasts({ toasts, onDismiss }) {
  return (
    <div className="fixed bottom-4 right-4 z-50 space-y-2 w-full max-w-sm">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={`toast ${t.type === "error" ? "toast-error" : "toast-success"}`}
          onClick={() => onDismiss(t.id)}
          role="status"
        >
          <div className="font-semibold">{t.title || (t.type === "error" ? "Error" : "Success")}</div>
          <div className="text-sm text-slate-200">{t.message}</div>
        </div>
      ))}
    </div>
  );
}

function Skeleton({ className = "" }) {
  return <div className={`skeleton ${className}`} />;
}

function StatCard({ title, children }) {
  return (
    <div className="glass p-4 space-y-3">
      <div className="text-lg font-semibold text-slate-100">{title}</div>
      {children}
    </div>
  );
}

function Summary({ summary }) {
  if (!summary) return null;
  const { status_counts = {}, failures = [] } = summary;
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      <StatCard title="Status counts (recent)">
        <div className="space-y-2">
          {Object.entries(status_counts).map(([k, v]) => (
            <div key={k} className="flex items-center justify-between text-sm text-slate-200">
              <span className="text-slate-300">{k}</span>
              <span className="chip">{v}</span>
            </div>
          ))}
        </div>
      </StatCard>
      <StatCard title="Recent failures">
        <div className="space-y-2">
          {failures.map((f) => (
            <div key={`${f.name}-${f.version}`} className="flex items-center justify-between text-sm text-slate-200">
              <span>{f.name} {f.version}</span>
              <span className="chip">{f.status}</span>
            </div>
          ))}
          {!failures.length && <div className="text-slate-400 text-sm">No recent failures</div>}
        </div>
      </StatCard>
    </div>
  );
}

function EventsTable({ events }) {
  if (!events?.length) return <div className="text-slate-400 text-sm">No events yet.</div>;
  return (
    <div className="glass p-4 space-y-3">
      <div className="text-lg font-semibold">Recent events</div>
      <div className="overflow-x-auto">
        <table className="min-w-full text-sm">
          <thead className="text-slate-400">
            <tr className="border-b border-border">
              <th className="text-left py-2">Status</th>
              <th className="text-left py-2">Package</th>
              <th className="text-left py-2">Python/Platform</th>
              <th className="text-left py-2">Detail</th>
            </tr>
          </thead>
          <tbody>
            {events.map((e) => (
              <tr key={`${e.name}-${e.version}-${e.timestamp}`} className="border-b border-slate-800">
                <td className="py-2"><span className={`status ${e.status}`}>{e.status}</span></td>
                <td className="py-2"><Link className="text-accent hover:underline" to={`/package/${e.name}`}>{e.name} {e.version}</Link></td>
                <td className="py-2 text-slate-400">{e.python_tag}/{e.platform_tag}</td>
                <td className="py-2 text-slate-400">{e.detail || ""}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function TopList({ title, items, render }) {
  return (
    <StatCard title={title}>
      <div className="list">
        {items?.length ? items.map(render) : <div className="muted">No data</div>}
      </div>
    </StatCard>
  );
}

function PackageDetail({ token, pushToast }) {
  const { name } = useParams();
  const [data, setData] = useState(null);
  const [logContent, setLogContent] = useState("");
  const [selectedEvent, setSelectedEvent] = useState(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState("");
  const [autoScroll, setAutoScroll] = useState(true);
  const logRef = useRef(null);

  const load = async () => {
    setLoading(true);
    setError("");
    try {
      const detail = await fetchPackageDetail(name, token, 100);
      setData(detail);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (autoScroll && logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [logContent, autoScroll]);

  useEffect(() => {
    load();
  }, [name, token]);

  const loadLog = async (ev) => {
    setSelectedEvent(ev);
    setLogContent("");
    setMessage("");
    try {
      const resp = await fetchLog(ev.name, ev.version, token);
      const content = typeof resp === "string" ? resp : resp?.content;
      if (content) {
        setLogContent(content);
      } else {
        setMessage("No log content available");
      }
      pushToast?.({ type: "success", title: "Log loaded", message: `${ev.name} ${ev.version}` });
    } catch (e) {
      setError(e.message);
      pushToast?.({ type: "error", title: "Log load failed", message: e.message });
    }
  };

  if (loading) {
    return (
      <div className="max-w-6xl mx-auto px-4 py-6 space-y-4">
        <Skeleton className="h-8 w-48" />
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <StatCard title="Recent failures">
            <div className="space-y-2">
              {[...Array(3)].map((_, i) => <Skeleton key={i} className="h-4 w-1/2" />)}
            </div>
          </StatCard>
          <StatCard title="Variants">
            <div className="space-y-2">
              {[...Array(3)].map((_, i) => <Skeleton key={i} className="h-4 w-1/3" />)}
            </div>
          </StatCard>
        </div>
        <StatCard title="Events">
          <div className="space-y-2">
            {[...Array(4)].map((_, i) => <Skeleton key={i} className="h-4 w-full" />)}
          </div>
        </StatCard>
      </div>
    );
  }
  if (error) return <div className="error">{error}</div>;
  if (!data) return null;

  const { summary, variants, failures, events } = data;
  const logDownloadHref = selectedEvent ? `${API_BASE}/logs/${selectedEvent.name}/${selectedEvent.version}` : null;

  return (
    <div className="max-w-6xl mx-auto px-4 py-6 space-y-4">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-3">
        <div className="space-y-1">
          <h2 className="text-2xl font-semibold text-slate-50">{summary.name}</h2>
          <div className="text-slate-400 text-sm">Status counts: {Object.entries(summary.status_counts || {}).map(([k, v]) => `${k}:${v}`).join("  ")}</div>
          {summary.latest && <div className="text-slate-400 text-sm">Latest: {summary.latest.status} {summary.latest.version} at {summary.latest.timestamp}</div>}
        </div>
        <Link to="/" className="btn btn-secondary">Back</Link>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <StatCard title="Recent failures">
          <div className="space-y-2">
            {failures?.length ? failures.map((f) => (
              <div key={`${f.name}-${f.version}-${f.timestamp}`} className="flex items-center justify-between text-sm text-slate-200">
                <span>{f.name} {f.version}</span>
                <span className="chip">{f.status}</span>
              </div>
            )) : <div className="text-slate-400 text-sm">No failures</div>}
          </div>
        </StatCard>
        <StatCard title="Variants">
          <div className="space-y-2">
            {variants?.length ? variants.map((v, idx) => (
              <div key={idx} className="flex items-center justify-between text-sm text-slate-200">
                <span className="text-slate-400">{v.metadata?.variant || "unknown"}</span>
                <span className="chip">{v.status}</span>
              </div>
            )) : <div className="text-slate-400 text-sm">No variant history</div>}
          </div>
        </StatCard>
      </div>

      <div className="glass p-4 space-y-3">
        <div className="flex items-center justify-between">
          <div className="text-lg font-semibold">Events</div>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full text-sm">
            <thead className="text-slate-400">
              <tr className="border-b border-border">
                <th className="text-left py-2">Status</th>
                <th className="text-left py-2">Version</th>
                <th className="text-left py-2">Detail</th>
                <th className="text-left py-2">Log</th>
              </tr>
            </thead>
            <tbody>
              {events.map((e) => (
                <tr key={`${e.name}-${e.version}-${e.timestamp}`} className="border-b border-slate-800">
                  <td className="py-2"><span className={`status ${e.status}`}>{e.status}</span></td>
                  <td className="py-2 text-slate-200">{e.version}</td>
                  <td className="py-2 text-slate-400">{e.detail || ""}</td>
                  <td className="py-2"><button className="btn btn-secondary" onClick={() => loadLog(e)}>View log</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {message && <div className="text-slate-400 text-sm">{message}</div>}
        {selectedEvent && (
          <div className="glass p-3 space-y-2">
            <div className="flex items-center justify-between">
              <div className="text-base font-semibold">Log: {selectedEvent.name} {selectedEvent.version}</div>
              <div className="flex items-center gap-2 text-xs text-slate-400">
                <span>{selectedEvent.timestamp}</span>
                <button className="btn btn-secondary px-2 py-1 text-xs" onClick={() => setAutoScroll((v) => !v)}>
                  Autoscroll: {autoScroll ? "on" : "off"}
                </button>
                {logDownloadHref && (
                  <a className="btn btn-secondary px-2 py-1 text-xs" href={logDownloadHref} target="_blank" rel="noreferrer">
                    Open log
                  </a>
                )}
              </div>
            </div>
            <pre
              ref={logRef}
              className="bg-slate-900 border border-border rounded-lg p-3 max-h-72 overflow-auto text-xs"
            >
              {logContent || "No content"}
            </pre>
          </div>
        )}
      </div>
    </div>
  );
}

function Dashboard({ token, onTokenChange, pushToast }) {
  const [authToken, setAuthToken] = useState(localStorage.getItem("refinery_token") || token || "");
  const [dashboard, setDashboard] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [retryPkg, setRetryPkg] = useState("");
  const [retryVersion, setRetryVersion] = useState("latest");

  const [pkgFilter, setPkgFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [recentLimit, setRecentLimit] = useState(25);
  const [pollMs, setPollMs] = useState(10000);

  const load = async (opts = {}) => {
    const { packageFilter, statusFilter: status } = opts;
    setLoading(true);
    setError("");
    try {
      const recent = await fetchRecent(
        {
          limit: recentLimit,
          packageFilter: packageFilter ?? pkgFilter,
          status: status ?? statusFilter,
        },
        authToken,
      );
      const data = await fetchDashboard(authToken);
      setDashboard({ ...data, recent });
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load({ packageFilter: pkgFilter, statusFilter });
  }, [authToken, pkgFilter, statusFilter, recentLimit]);

  useEffect(() => {
    if (!pollMs) return;
    const id = setInterval(() => load({ packageFilter: pkgFilter, statusFilter }), pollMs);
    return () => clearInterval(id);
  }, [pollMs, authToken, pkgFilter, statusFilter, recentLimit]);

  const handleTriggerWorker = async () => {
    setMessage("");
    try {
      const resp = await triggerWorker(authToken);
      setMessage(resp.detail || "Worker triggered");
      pushToast?.({ type: "success", title: "Worker", message: resp.detail || "Triggered worker" });
      await load();
    } catch (e) {
      setError(e.message);
      pushToast?.({ type: "error", title: "Worker trigger failed", message: e.message });
    }
  };

  const handleRetry = async () => {
    setMessage("");
    if (!retryPkg) {
      setError("Enter a package name");
      pushToast?.({ type: "error", title: "Retry failed", message: "Enter a package name" });
      return;
    }
    try {
      const resp = await enqueueRetry(retryPkg, retryVersion, authToken);
      setMessage(resp.detail || "Enqueued");
      pushToast?.({ type: "success", title: "Enqueued", message: resp.detail || `${retryPkg} queued` });
      await load();
    } catch (e) {
      setError(e.message);
      pushToast?.({ type: "error", title: "Enqueue failed", message: e.message });
    }
  };

  const handleSaveToken = async () => {
    localStorage.setItem("refinery_token", authToken);
    onTokenChange?.(authToken);
    if (authToken) {
      try {
        await setCookieToken(authToken);
      } catch {
        // ignore
      }
    }
    setMessage("Token saved");
    pushToast?.({ type: "success", title: "Token saved", message: "Worker token stored locally" });
  };

  const queueLength = dashboard?.queue?.length ?? 0;
  const workerMode = dashboard?.queue?.worker_mode || "unknown";
  const queueItems = dashboard?.queue?.items || [];
  const hints = dashboard?.hints || [];
  const metrics = dashboard?.metrics;
  const recent = dashboard?.recent || [];

  const renderLoading = () => (
    <div className="space-y-4">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <StatCard title="Status counts (recent)">
          <div className="space-y-2">
            {[...Array(4)].map((_, i) => (
              <Skeleton key={i} className="h-4 w-32" />
            ))}
          </div>
        </StatCard>
        <StatCard title="Recent failures">
          <div className="space-y-2">
            {[...Array(3)].map((_, i) => (
              <Skeleton key={i} className="h-4 w-48" />
            ))}
          </div>
        </StatCard>
      </div>
      <StatCard title="Recent events">
        <div className="space-y-2">
          {[...Array(5)].map((_, i) => (
            <Skeleton key={i} className="h-4 w-full" />
          ))}
        </div>
      </StatCard>
    </div>
  );

  return (
    <div className="max-w-6xl mx-auto px-4 py-6 space-y-4">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold text-slate-50">s390x Wheel Refinery</h1>
          <div className="text-slate-400 text-sm">Data-driven control plane (React SPA)</div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <input
            className="input max-w-xs"
            placeholder="Worker token (optional)"
            value={authToken}
            onChange={(e) => setAuthToken(e.target.value)}
          />
          <button className="btn btn-secondary" onClick={handleSaveToken}>Save token</button>
          <button className="btn btn-secondary" onClick={() => load({ packageFilter: pkgFilter, statusFilter })} disabled={loading}>Refresh</button>
        </div>
      </div>

      {error && <div className="text-red-400 text-sm">{error}</div>}
      {message && <div className="text-green-400 text-sm">{message}</div>}

      {loading && !dashboard ? (
        renderLoading()
      ) : (
        <Summary summary={dashboard?.summary} />
      )}

      <div className="glass p-4 space-y-3">
        <div className="flex flex-wrap gap-3">
          <input className="input max-w-xs" placeholder="Filter package" value={pkgFilter} onChange={(e) => setPkgFilter(e.target.value)} />
          <input className="input max-w-xs" placeholder="Filter status (built,failed,...)" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} />
          <input className="input max-w-[140px]" placeholder="Recent limit" value={recentLimit} onChange={(e) => setRecentLimit(Number(e.target.value) || 25)} />
          <input className="input max-w-[180px]" placeholder="Poll ms (0=off)" value={pollMs} onChange={(e) => setPollMs(Number(e.target.value) || 0)} />
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <TopList
          title="Top failures"
          items={dashboard?.failures || []}
          render={(f) => (
            <div key={f.name} className="flex items-center justify-between text-sm text-slate-200">
              <span>{f.name}</span>
              <span className="chip">{f.failures} failures</span>
            </div>
          )}
        />
        <TopList
          title="Top slow packages"
          items={dashboard?.slowest || []}
          render={(s) => (
            <div key={s.name} className="flex items-center justify-between text-sm text-slate-200">
              <span>{s.name}</span>
              <span className="chip">{s.avg_duration}s avg</span>
            </div>
          )}
        />
        <StatCard title="Queue & worker">
          <div className="space-y-2 text-sm text-slate-200">
            <div className="flex items-center justify-between">
              <span className="text-slate-400">Queue length</span>
              <span className="chip">{queueLength}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-slate-400">Worker mode</span>
              <span className="chip">{workerMode}</span>
            </div>
            <button className="btn btn-primary" onClick={handleTriggerWorker}>Run worker now</button>
            {queueItems.length > 0 && (
              <div className="overflow-x-auto">
                <table className="min-w-full text-xs border border-border rounded-lg">
                  <thead className="bg-slate-900 text-slate-400">
                    <tr>
                      <th className="text-left px-2 py-2">Package</th>
                      <th className="text-left px-2 py-2">Version</th>
                      <th className="text-left px-2 py-2">Python</th>
                      <th className="text-left px-2 py-2">Platform</th>
                      <th className="text-left px-2 py-2">Recipes</th>
                    </tr>
                  </thead>
                  <tbody>
                    {queueItems.map((q, idx) => (
                      <tr key={`${q.package}-${q.version}-${idx}`} className="border-t border-slate-800">
                        <td className="px-2 py-2">{q.package}</td>
                        <td className="px-2 py-2">{q.version || "latest"}</td>
                        <td className="px-2 py-2 text-slate-400">{q.python_tag || "-"}</td>
                        <td className="px-2 py-2 text-slate-400">{q.platform_tag || "-"}</td>
                        <td className="px-2 py-2 text-slate-400 truncate">{(q.recipes || []).join(", ") || "-"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </StatCard>
        <StatCard title="Hints">
          <div className="space-y-2 max-h-52 overflow-auto text-sm text-slate-200">
            {hints.length ? hints.map((h, idx) => (
              <div key={idx} className="text-slate-300 border border-border rounded-lg p-2">
                <div className="font-semibold">Pattern: {h.pattern}</div>
                <div className="text-slate-400">dnf: {(h.packages?.dnf || []).join(", ") || "-"}</div>
                <div className="text-slate-400">apt: {(h.packages?.apt || []).join(", ") || "-"}</div>
              </div>
            )) : <div className="text-slate-400">No hints loaded</div>}
          </div>
        </StatCard>
      </div>

      {metrics && (
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <StatCard title="Metrics snapshot">
            <div className="space-y-2 text-sm text-slate-200">
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Queue length</span>
                <span className="chip">{metrics.queue_length}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Worker mode</span>
                <span className="chip">{metrics.worker_mode || "unknown"}</span>
              </div>
            </div>
          </StatCard>
        </div>
      )}

      <div className="glass p-4 space-y-3">
        <div className="text-lg font-semibold">Enqueue retry</div>
        <div className="flex flex-col md:flex-row gap-3">
          <input className="input" placeholder="package name" value={retryPkg} onChange={(e) => setRetryPkg(e.target.value)} />
          <input className="input md:max-w-[180px]" placeholder="version (or latest)" value={retryVersion} onChange={(e) => setRetryVersion(e.target.value)} />
          <button className="btn btn-primary" onClick={handleRetry}>Enqueue</button>
        </div>
        <div className="text-slate-400 text-sm">
          Uses API: POST /package/&lt;name&gt;/retry (adds hint-derived recipes automatically).
        </div>
      </div>

      <EventsTable events={recent} />
    </div>
  );
}

export default function App() {
  const [token, setToken] = useState(localStorage.getItem("refinery_token") || "");
  const [toasts, setToasts] = useState([]);

  const dismissToast = (id) => setToasts((ts) => ts.filter((t) => t.id !== id));
  const pushToast = ({ type = "success", title, message }) => {
    const id = `${Date.now()}-${Math.random()}`;
    setToasts((ts) => [...ts, { id, type, title, message }]);
    setTimeout(() => dismissToast(id), 4000);
  };

  return (
    <>
      <Routes>
        <Route path="/" element={<Dashboard token={token} onTokenChange={setToken} pushToast={pushToast} />} />
        <Route path="/package/:name" element={<PackageDetail token={token} pushToast={pushToast} />} />
      </Routes>
      <Toasts toasts={toasts} onDismiss={dismissToast} />
    </>
  );
}
