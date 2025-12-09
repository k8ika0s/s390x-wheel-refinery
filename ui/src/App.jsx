import { useEffect, useRef, useState } from "react";
import { Routes, Route, Link, useParams, useLocation } from "react-router-dom";
import { API_BASE, clearQueue, enqueueRetry, fetchDashboard, fetchLog, fetchPackageDetail, fetchRecent, setCookieToken, triggerWorker } from "./api";

const ENV_LABEL = import.meta.env.VITE_ENV_LABEL || "Local";

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

function EmptyState({ title = "Nothing here", detail, actionLabel, onAction }) {
  return (
    <div className="glass p-4 text-slate-300 text-sm space-y-2 border-dashed border border-border">
      <div className="font-semibold">{title}</div>
      {detail && <div className="text-slate-500">{detail}</div>}
      {actionLabel && onAction && (
        <button className="btn btn-secondary px-2 py-1 text-xs" onClick={onAction}>{actionLabel}</button>
      )}
    </div>
  );
}

function Layout({ children, tokenActive }) {
  const location = useLocation();
  const isActive = (path) => location.pathname === path || (path !== "/" && location.pathname.startsWith(path));
  return (
    <div className="min-h-screen bg-bg text-slate-100">
      <header className="glass sticky top-0 z-40 backdrop-blur-xs border-b border-border">
        <div className="max-w-6xl mx-auto px-4 py-3 flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <Link to="/" className="text-xl font-bold text-accent">s390x Wheel Refinery</Link>
            <span className="chip bg-slate-800 border-border text-xs">Env: {ENV_LABEL}</span>
            <span className="chip bg-slate-800 border-border text-xs">API: {API_BASE || "same-origin"}</span>
            {tokenActive ? (
              <span className="chip bg-emerald-900 border border-emerald-600 text-xs text-emerald-100">Token active</span>
            ) : (
              <span className="chip bg-slate-800 border-border text-xs text-slate-300">Token: none</span>
            )}
          </div>
          <nav className="flex items-center gap-3 text-sm text-slate-200">
            <Link to="/" className={`hover:text-accent ${isActive("/") ? "text-accent font-semibold" : ""}`}>Dashboard</Link>
            <span className="text-xs text-slate-500">|</span>
            <span className="text-xs text-slate-400">Status filters: tap chips below</span>
          </nav>
        </div>
      </header>
      <main>{children}</main>
    </div>
  );
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

function EventsTable({ events, title = "Recent events", pageSize = 10 }) {
  const [page, setPage] = useState(1);
  const [sortKey, setSortKey] = useState("timestamp");
  const [sortDir, setSortDir] = useState("desc");

  const sorted = (events || []).slice().sort((a, b) => {
    const dir = sortDir === "asc" ? 1 : -1;
    if (sortKey === "status") {
      return dir * (a.status || "").localeCompare(b.status || "");
    }
    if (sortKey === "package") {
      return dir * (`${a.name || ""}${a.version || ""}`.localeCompare(`${b.name || ""}${b.version || ""}`));
    }
    // default timestamp
    if (a.timestamp && b.timestamp) {
      return dir * (new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());
    }
    return dir * (b.name || "").localeCompare(a.name || "");
  });
  const totalPages = Math.max(1, Math.ceil(sorted.length / pageSize));
  const pageItems = sorted.slice((page - 1) * pageSize, page * pageSize);

  useEffect(() => {
    setPage(1);
  }, [events, pageSize, sortKey, sortDir]);

  const toggleSort = (key) => {
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir("desc");
    }
  };

  if (!events?.length) return <EmptyState title="No events" detail="Try clearing filters or increasing the recent limit." />;
  return (
    <div className="glass p-4 space-y-3">
      <div className="flex items-center justify-between">
        <div className="text-lg font-semibold">{title}</div>
        <div className="flex items-center gap-2 text-xs text-slate-400">
          <span>Sort: {sortKey} ({sortDir})</span>
          <span>Page {page} / {totalPages}</span>
          <button className="btn btn-secondary px-2 py-1 text-xs" disabled={page === 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>Prev</button>
          <button className="btn btn-secondary px-2 py-1 text-xs" disabled={page === totalPages} onClick={() => setPage((p) => Math.min(totalPages, p + 1))}>Next</button>
        </div>
      </div>
      <div className="overflow-x-auto">
        <table className="min-w-full text-sm">
          <thead className="text-slate-400">
            <tr className="border-b border-border sticky top-0 bg-slate-900">
              <th className="text-left py-2 cursor-pointer" onClick={() => toggleSort("status")}>Status</th>
              <th className="text-left py-2 cursor-pointer" onClick={() => toggleSort("package")}>Package</th>
              <th className="text-left py-2">Python/Platform</th>
              <th className="text-left py-2">Detail</th>
            </tr>
          </thead>
          <tbody>
            {pageItems.map((e) => (
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
  const [tab, setTab] = useState("overview");
  const [variantPage, setVariantPage] = useState(1);
  const [failurePage, setFailurePage] = useState(1);
  const [hintsPage, setHintsPage] = useState(1);
  const pageSize = 10;

  const load = async () => {
    setLoading(true);
    setError("");
    try {
      const detail = await fetchPackageDetail(name, token, 100);
      setData(detail);
    } catch (e) {
      const msg = e.status === 403 ? "Forbidden: set a worker token" : e.message;
      setError(msg);
      pushToast?.({ type: "error", title: "Load failed", message: msg || "Unknown error" });
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

  const paged = (items, page) => {
    const arr = Array.isArray(items) ? items : [];
    const total = Math.max(1, Math.ceil(arr.length / pageSize));
    const slice = arr.slice((page - 1) * pageSize, page * pageSize);
    return { total, slice };
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

  const { summary, variants, failures, events, hints = [] } = data;
  const logDownloadHref = selectedEvent ? `${API_BASE}/logs/${selectedEvent.name}/${selectedEvent.version}` : null;

  const variantsPaged = paged(variants, variantPage);
  const failuresPaged = paged(failures, failurePage);
  const hintsPaged = paged(hints, hintsPage);

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
      <div className="flex gap-2">
        {["overview", "events", "hints"].map((t) => (
          <button
            key={t}
            className={`btn ${tab === t ? "btn-primary" : "btn-secondary"}`}
            onClick={() => setTab(t)}
          >
            {t === "overview" ? "Overview" : t === "events" ? "Events & Logs" : "Hints"}
          </button>
        ))}
      </div>

      {tab === "overview" && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <StatCard title="Recent failures">
            <div className="space-y-2">
              {failuresPaged.slice.length ? failuresPaged.slice.map((f) => (
                <div key={`${f.name}-${f.version}-${f.timestamp}`} className="flex items-center justify-between text-sm text-slate-200">
                  <span>{f.name} {f.version}</span>
                  <span className="chip">{f.status}</span>
                </div>
              )) : <EmptyState title="No failures" detail="Great! No recent failures logged for this package." />}
              {failures?.length > pageSize && (
                <div className="flex items-center gap-2 text-xs text-slate-400">
                  <span>Page {failurePage} / {failuresPaged.total}</span>
                  <button className="btn btn-secondary px-2 py-1 text-xs" disabled={failurePage === 1} onClick={() => setFailurePage((p) => Math.max(1, p - 1))}>Prev</button>
                  <button className="btn btn-secondary px-2 py-1 text-xs" disabled={failurePage === failuresPaged.total} onClick={() => setFailurePage((p) => Math.min(failuresPaged.total, p + 1))}>Next</button>
                </div>
              )}
            </div>
          </StatCard>
          <StatCard title="Variants">
            <div className="space-y-2">
              {variantsPaged.slice.length ? variantsPaged.slice.map((v, idx) => (
                <div key={idx} className="flex items-center justify-between text-sm text-slate-200">
                  <span className="text-slate-400">{v.metadata?.variant || "unknown"}</span>
                  <span className="chip">{v.status}</span>
                </div>
              )) : <EmptyState title="No variant history" detail="No variant attempts recorded yet." />}
              {variants?.length > pageSize && (
                <div className="flex items-center gap-2 text-xs text-slate-400">
                  <span>Page {variantPage} / {variantsPaged.total}</span>
                  <button className="btn btn-secondary px-2 py-1 text-xs" disabled={variantPage === 1} onClick={() => setVariantPage((p) => Math.max(1, p - 1))}>Prev</button>
                  <button className="btn btn-secondary px-2 py-1 text-xs" disabled={variantPage === variantsPaged.total} onClick={() => setVariantPage((p) => Math.min(variantsPaged.total, p + 1))}>Next</button>
                </div>
              )}
            </div>
          </StatCard>
        </div>
      )}

      {tab === "events" && (
        <div className="space-y-3">
          <div className="glass p-4 space-y-3">
            <div className="flex items-center justify-between">
              <div className="text-lg font-semibold">Events</div>
            </div>
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead className="text-slate-400 sticky top-0 bg-slate-900">
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
          </div>
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
      )}

      {tab === "hints" && (
        <div className="glass p-4 space-y-3">
          <div className="text-lg font-semibold">Hints matched</div>
          <div className="space-y-2">
            {hintsPaged.slice.length ? hintsPaged.slice.map((h, idx) => (
              <div key={idx} className="border border-border rounded-lg p-3 space-y-1 text-sm text-slate-200">
                <div className="font-semibold text-slate-100">Pattern: {h.pattern}</div>
                <div className="text-slate-400">dnf: {(h.packages?.dnf || []).join(", ") || "-"}</div>
                <div className="text-slate-400">apt: {(h.packages?.apt || []).join(", ") || "-"}</div>
                {h.note && <div className="text-slate-400">note: {h.note}</div>}
              </div>
            )) : <EmptyState title="No hints" detail="No hint recipes recorded for this package." />}
            {hints?.length > pageSize && (
              <div className="flex items-center gap-2 text-xs text-slate-400">
                <span>Page {hintsPage} / {hintsPaged.total}</span>
                <button className="btn btn-secondary px-2 py-1 text-xs" disabled={hintsPage === 1} onClick={() => setHintsPage((p) => Math.max(1, p - 1))}>Prev</button>
                <button className="btn btn-secondary px-2 py-1 text-xs" disabled={hintsPage === hintsPaged.total} onClick={() => setHintsPage((p) => Math.min(hintsPaged.total, p + 1))}>Next</button>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

const STATUS_CHIPS = ["built", "failed", "reused", "cached", "missing", "skipped_known_failure"];

function Dashboard({ token, onTokenChange, pushToast }) {
  const [authToken, setAuthToken] = useState(localStorage.getItem("refinery_token") || token || "");
  const [dashboard, setDashboard] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [retryPkg, setRetryPkg] = useState("");
  const [retryVersion, setRetryVersion] = useState("latest");
  const [selectedQueue, setSelectedQueue] = useState({});

  const [pkgFilter, setPkgFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [recentLimit, setRecentLimit] = useState(25);
  const [pollMs, setPollMs] = useState(10000);
  const [search, setSearch] = useState("");

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
      const msg = e.status === 403 ? "Forbidden: set a worker token" : e.message;
      setError(msg);
      pushToast?.({ type: "error", title: "Load failed", message: msg || "Unknown error" });
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

  const handleBulkRetry = async () => {
    const items = Object.values(selectedQueue);
    if (!items.length) {
      pushToast?.({ type: "error", title: "No selection", message: "Select queue items first" });
      return;
    }
    setMessage("");
    try {
      for (const item of items) {
        await enqueueRetry(item.package, item.version, authToken);
      }
      pushToast?.({ type: "success", title: "Enqueued retries", message: `${items.length} item(s) queued` });
      clearSelected();
      await load();
    } catch (e) {
      setError(e.message);
      pushToast?.({ type: "error", title: "Bulk retry failed", message: e.message });
    }
  };

  const handleClearQueue = async () => {
    if (!window.confirm("Clear all items from the queue?")) return;
    try {
      const resp = await clearQueue(authToken);
      pushToast?.({ type: "success", title: "Queue cleared", message: resp.detail || "Cleared" });
      clearSelected();
      await load();
    } catch (e) {
      setError(e.message);
      pushToast?.({ type: "error", title: "Clear queue failed", message: e.message });
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
  const queueItemsSorted = queueItems.slice().sort((a, b) => (a.package || "").localeCompare(b.package || ""));
  const hints = dashboard?.hints || [];
  const metrics = dashboard?.metrics;
  const recent = dashboard?.recent || [];
  const filteredRecent = recent.filter((e) => {
    const matchPkg = search ? `${e.name} ${e.version}`.toLowerCase().includes(search.toLowerCase()) : true;
    return matchPkg;
  });

  const toggleSelectQueue = (item) => {
    const key = `${item.package}@${item.version || "latest"}`;
    setSelectedQueue((prev) => {
      const next = { ...prev };
      if (next[key]) {
        delete next[key];
      } else {
        next[key] = item;
      }
      return next;
    });
  };

  const clearSelected = () => setSelectedQueue({});

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

      {error && (
        <div className="glass p-3 border border-red-500/40 text-sm text-red-200 flex items-center justify-between">
          <span>{error}</span>
          <button className="btn btn-secondary px-2 py-1 text-xs" onClick={() => load({ packageFilter: pkgFilter, statusFilter })}>Retry</button>
        </div>
      )}
      {message && <div className="text-green-400 text-sm">{message}</div>}

      <div className="grid lg:grid-cols-[320px,1fr] gap-4 items-start">
        <div className="space-y-4">
          <div className="glass p-4 space-y-3">
            <div className="text-lg font-semibold flex items-center gap-2">
              <span>Filters</span>
              <span className="chip text-xs">🎯</span>
            </div>
            <div className="space-y-2">
              <div className="space-y-1">
                <div className="text-xs text-slate-400">Filter package</div>
                <input className="input w-full" placeholder="Filter package" value={pkgFilter} onChange={(e) => setPkgFilter(e.target.value)} />
              </div>
              <input className="input w-full" placeholder="Search recent (name/version)" value={search} onChange={(e) => setSearch(e.target.value)} />
              <div className="flex gap-2">
                <input className="input w-1/2" placeholder="Recent limit" value={recentLimit} onChange={(e) => setRecentLimit(Number(e.target.value) || 25)} />
                <input className="input w-1/2" placeholder="Poll ms (0=off)" value={pollMs} onChange={(e) => setPollMs(Number(e.target.value) || 0)} />
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              {STATUS_CHIPS.map((s) => {
                const active = statusFilter === s;
                return (
                  <button
                    key={s}
                    className={`chip cursor-pointer ${active ? "bg-accent text-slate-900" : "hover:bg-slate-800"}`}
                    onClick={() => setStatusFilter(active ? "" : s)}
                  >
                    {s}
                  </button>
                );
              })}
              {statusFilter && (
                <button className="btn btn-secondary px-2 py-1 text-xs" onClick={() => setStatusFilter("")}>
                  Clear status filter
                </button>
              )}
              <button className="btn btn-secondary px-2 py-1 text-xs" onClick={() => { clearFilters(); load({ packageFilter: "", statusFilter: "" }); }}>
                Clear all
              </button>
            </div>
          </div>

          <div className="glass p-4 space-y-3">
            <div className="text-lg font-semibold flex items-center gap-2">
              <span>Queue controls</span>
              <span className="chip text-xs">🧰</span>
            </div>
            <div className="space-y-2 text-sm text-slate-200">
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Queue length</span>
                <span className="chip">{queueLength}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Worker mode</span>
                <span className="chip">{workerMode}</span>
              </div>
              <button className="btn btn-primary w-full" onClick={handleTriggerWorker}>Run worker now</button>
              <div className="flex flex-wrap gap-2">
                <button className="btn btn-secondary px-2 py-1 text-xs" onClick={handleBulkRetry} disabled={!Object.keys(selectedQueue).length}>
                  Retry selected
                </button>
                <button className="btn btn-secondary px-2 py-1 text-xs" onClick={handleClearQueue} disabled={!queueItemsSorted.length}>
                  Clear queue
                </button>
              </div>
            </div>
            {queueItemsSorted.length > 0 ? (
              <div className="overflow-x-auto">
                <table className="min-w-full text-xs border border-border rounded-lg">
                  <thead className="bg-slate-900 text-slate-400 sticky top-0">
                    <tr>
                      <th className="px-2 py-2"></th>
                      <th className="text-left px-2 py-2">Package</th>
                      <th className="text-left px-2 py-2">Version</th>
                      <th className="text-left px-2 py-2">Python</th>
                      <th className="text-left px-2 py-2">Platform</th>
                      <th className="text-left px-2 py-2">Recipes</th>
                    </tr>
                  </thead>
                  <tbody>
                    {queueItemsSorted.map((q, idx) => {
                      const key = `${q.package}@${q.version || "latest"}`;
                      const checked = Boolean(selectedQueue[key]);
                      return (
                      <tr key={`${q.package}-${q.version}-${idx}`} className="border-t border-slate-800">
                        <td className="px-2 py-2">
                          <input type="checkbox" checked={checked} onChange={() => toggleSelectQueue(q)} />
                        </td>
                        <td className="px-2 py-2">{q.package}</td>
                        <td className="px-2 py-2">{q.version || "latest"}</td>
                        <td className="px-2 py-2 text-slate-400">{q.python_tag || "-"}</td>
                        <td className="px-2 py-2 text-slate-400">{q.platform_tag || "-"}</td>
                        <td className="px-2 py-2 text-slate-400 truncate">{(q.recipes || []).join(", ") || "-"}</td>
                      </tr>
                    )})}
                  </tbody>
                </table>
              </div>
            ) : (
              <EmptyState title="Queue is empty" detail="No retry requests pending." actionLabel="Refresh" onAction={() => load({ packageFilter: pkgFilter, statusFilter })} />
            )}
          </div>

          <div className="glass p-4 space-y-3">
            <div className="text-lg font-semibold flex items-center gap-2">
              <span>Enqueue retry</span>
              <span className="chip text-xs">⏩</span>
            </div>
            <div className="flex flex-col gap-3">
              <input className="input" placeholder="package name" value={retryPkg} onChange={(e) => setRetryPkg(e.target.value)} />
              <input className="input" placeholder="version (or latest)" value={retryVersion} onChange={(e) => setRetryVersion(e.target.value)} />
              <button className="btn btn-primary" onClick={handleRetry}>Enqueue</button>
            </div>
            <div className="text-slate-400 text-sm">
              Uses API: POST /package/&lt;name&gt;/retry (adds hint-derived recipes automatically).
            </div>
          </div>
        </div>

        <div className="space-y-4">
          {loading && !dashboard ? (
            renderLoading()
          ) : (
            <Summary summary={dashboard?.summary} />
          )}

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
            {metrics && (
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
            )}
          </div>

          <EventsTable events={filteredRecent} />
        </div>
      </div>
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
    <Layout tokenActive={Boolean(token)}>
      <Routes>
        <Route path="/" element={<Dashboard token={token} onTokenChange={setToken} pushToast={pushToast} />} />
        <Route path="/package/:name" element={<PackageDetail token={token} pushToast={pushToast} />} />
      </Routes>
      <Toasts toasts={toasts} onDismiss={dismissToast} />
    </Layout>
  );
}
  const clearFilters = () => {
    setPkgFilter("");
    setStatusFilter("");
    setSearch("");
    setRecentLimit(25);
    setPollMs(10000);
  };
