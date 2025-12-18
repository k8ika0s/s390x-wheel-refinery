import { useEffect, useRef, useState } from "react";
import { Routes, Route, Link, Navigate, useParams, useLocation, useNavigate } from "react-router-dom";
import {
  getApiBase,
  clearQueue,
  clearBuilds,
  clearPendingInputs,
  clearPlanQueue,
  enqueueRetry,
  createHint,
  updateHint,
  deleteHint,
  bulkUploadHints,
  fetchMetrics,
  fetchQueue,
  fetchSummary,
  fetchTopFailures,
  fetchTopSlowest,
  fetchLog,
  fetchPendingInputs,
  fetchPackageDetail,
  fetchRecent,
  fetchBuilds,
  fetchSettings,
  fetchHints,
  setCookieToken,
  triggerWorker,
  updateSettings,
  uploadRequirements,
  uploadWheel,
  deletePendingInput,
  enqueuePlan,
  enqueueBuildsFromPlan,
  fetchPlans,
  deletePlans,
  fetchPlan,
} from "./api";

const ENV_LABEL = import.meta.env.VITE_ENV_LABEL || "Local";
const LOGO_SRC = "/s390x-wheel-refinery-logo.png";

const toArray = (value) => (Array.isArray(value) ? value : []);
const formatEpoch = (value) => {
  if (!value) return "";
  const date = new Date(Number(value) * 1000);
  return Number.isFinite(date.getTime()) ? date.toLocaleString() : "";
};
const formatBytes = (value) => {
  if (value === undefined || value === null) return "-";
  const n = Number(value);
  if (!Number.isFinite(n)) return "-";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
};

function ArtifactBadges({ meta }) {
  if (!meta) return null;
  const items = [];
  const add = (label, url, digest) => {
    if (url) items.push({ label, url });
    else if (digest) items.push({ label: `${label}:${digest.slice(0, 12)}…` });
  };
  add("wheel", meta.wheel_url, meta.wheel_digest);
  add("repair", meta.repair_url, meta.repair_digest);
  add("runtime", meta.runtime_url, meta.runtime_digest);
  const packs = meta.pack_urls || meta.pack_digests || [];
  packs.forEach((p, idx) => add(`pack${idx + 1}`, typeof p === "string" ? p : "", typeof p === "string" ? p : ""));
  if (!items.length) return null;
  return (
    <div className="flex flex-wrap gap-1">
      {items.map((it, idx) =>
        it.url ? (
          <a key={idx} href={it.url} target="_blank" rel="noreferrer" className="chip chip-link">
            {it.label}
          </a>
        ) : (
          <span key={idx} className="chip chip-muted">{it.label}</span>
        ),
      )}
    </div>
  );
}

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

function EmptyState({ title = "Nothing here", detail, actionLabel, onAction, icon = "🫗" }) {
  return (
    <div className="glass p-4 text-slate-300 text-sm space-y-2 border-dashed border border-border">
      <div className="flex items-center gap-2 font-semibold">
        <span>{icon}</span>
        <span>{title}</span>
      </div>
      {detail && <div className="text-slate-500">{detail}</div>}
      {actionLabel && onAction && (
        <button className="btn btn-secondary px-2 py-1 text-xs" onClick={onAction}>{actionLabel}</button>
      )}
    </div>
  );
}

function Layout({ children, tokenActive, theme, onToggleTheme, metrics, apiBase, apiStatus }) {
  const location = useLocation();
  const isActive = (path, aliases = []) =>
    [path, ...aliases].some((p) => location.pathname === p || (p !== "/" && location.pathname.startsWith(p)));
  const systemStatus = metrics?.db?.status || "unknown";
  const systemTone = systemStatus === "ok" ? "text-emerald-300" : "text-amber-300";
  const apiLabel = apiStatus === "ok" ? "connected" : apiStatus === "error" ? "offline" : "unknown";
  const apiTone = apiStatus === "ok" ? "bg-emerald-400" : apiStatus === "error" ? "bg-rose-400" : "bg-amber-400";
  const totalQueue =
    (metrics?.queue?.length ?? 0) + (metrics?.pending?.plan_queue ?? 0) + (metrics?.build?.length ?? 0);
  const queueLevel = totalQueue === 0 ? 0 : totalQueue < 5 ? 1 : totalQueue < 20 ? 2 : totalQueue < 50 ? 3 : totalQueue < 100 ? 4 : 5;
  const navItems = [
    { to: "/", label: "Overview", aliases: ["/overview"] },
    { to: "/inputs", label: "Inputs" },
    { to: "/plans", label: "Plans" },
    { to: "/builds", label: "Builds" },
    { to: "/queues", label: "Queues" },
    { to: "/hints", label: "Hints" },
    { to: "/settings", label: "Settings" },
  ];

  return (
    <div className={`min-h-screen app-shell ${theme === "light" ? "theme-light" : "bg-bg text-slate-100"}`}>
      <header className="glass sticky top-0 z-40 backdrop-blur-sm border-b border-border/70">
        <div className="max-w-6xl mx-auto px-4 py-3 space-y-3">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <Link to="/" className="brand">
              <img src={LOGO_SRC} alt="Wheel Refinery logo" className="h-12 w-12 rounded-xl shadow-lg object-contain" />
              <div className="flex flex-col leading-tight">
                <span className="text-sm text-slate-400">s390x</span>
                <span className="text-lg font-bold text-accent">Wheel Refinery</span>
              </div>
            </Link>
            <div className="flex flex-wrap items-center gap-3 text-sm text-slate-200 lg:justify-end">
              <nav className="flex items-center gap-3 flex-wrap">
                {navItems.map((item) => (
                  <Link key={item.to} to={item.to} className={`nav-link ${isActive(item.to, item.aliases) ? "nav-link-active" : ""}`}>
                    {item.label}
                  </Link>
                ))}
              </nav>
              <button className="btn btn-ghost px-2 py-1 text-xs" onClick={onToggleTheme} title="Toggle theme">
                {theme === "light" ? "🌤️" : "🌙"}
              </button>
            </div>
          </div>
          <div className="status-rail">
            <span className="chip status-chip bg-slate-800 border-border text-xs">
              <span className="text-slate-400">Env</span>
              <span className="text-slate-200">{ENV_LABEL}</span>
            </span>
            <span className="chip status-chip bg-slate-800 border-border text-xs" title={apiBase || "same-origin"}>
              <span className="text-slate-400">API</span>
              <span className="inline-flex items-center gap-2">
                <span className={`inline-block h-2 w-2 rounded-full ${apiTone}`} />
                <span className="text-slate-300">{apiLabel}</span>
              </span>
            </span>
            <span className="chip status-chip bg-slate-800 border-border text-xs">
              <span className="text-slate-400">Queue</span>
              <span className="inline-flex items-center gap-2">
                <span className="inline-flex items-center gap-1">
                  {Array.from({ length: 5 }).map((_, idx) => (
                    <span
                      key={idx}
                      className={`inline-block h-2 w-2 rounded-full ${idx < queueLevel ? "bg-cyan-300" : "bg-slate-700"}`}
                    />
                  ))}
                </span>
                <span className="text-slate-300">{totalQueue}</span>
              </span>
            </span>
            <span className="chip status-chip bg-slate-800 border-border text-xs">
              <span className="text-slate-400">System</span>
              <span className={systemTone}>{systemStatus}</span>
            </span>
            {tokenActive && (
              <span className="chip status-chip bg-emerald-900 border border-emerald-600 text-xs text-emerald-100">
                <span className="text-emerald-100/80">Token</span>
                <span>active</span>
              </span>
            )}
          </div>
        </div>
      </header>
      <main>{children}</main>
    </div>
  );
}

function StatCard({ title, children }) {
  return (
    <div className="glass surface p-4 space-y-3">
      <div className="text-lg font-semibold text-slate-100 flex items-center gap-2">
        <span className="w-1 h-6 rounded-full bg-accent/80" aria-hidden />
        <span>{title}</span>
      </div>
      {children}
    </div>
  );
}

function StatTile({ icon, label, value, hint }) {
  return (
    <div className="stat-tile glass">
      <div className="flex items-center gap-2 text-sm text-slate-400">
        <span className="text-xl">{icon}</span>
        <div>{label}</div>
      </div>
      <div className="text-3xl font-semibold text-slate-50 text-center py-2">{value}</div>
      {hint && <div className="text-xs text-slate-500 mt-1">{hint}</div>}
    </div>
  );
}

function PageHeader({ title, subtitle, badge }) {
  return (
    <div className="glass p-4 flex flex-col md:flex-row md:items-center md:justify-between gap-3">
      <div>
        <div className="flex items-center gap-2">
          <h2 className="text-2xl font-semibold text-slate-50">{title}</h2>
          {badge && <span className="chip text-xs">{badge}</span>}
        </div>
        {subtitle && <p className="text-slate-400 text-sm mt-1 max-w-2xl">{subtitle}</p>}
      </div>
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
              <th className="text-left py-2">Artifacts</th>
              <th className="text-left py-2">Detail</th>
            </tr>
          </thead>
          <tbody>
            {pageItems.map((e) => (
              <tr key={`${e.name}-${e.version}-${e.timestamp}`} className="border-b border-slate-800">
                <td className="py-2"><span className={`status ${e.status}`}>{e.status}</span></td>
                <td className="py-2"><Link className="text-accent hover:underline" to={`/package/${e.name}`}>{e.name} {e.version}</Link></td>
                <td className="py-2 text-slate-400">{e.python_tag}/{e.platform_tag}</td>
                <td className="py-2 text-slate-300"><ArtifactBadges meta={e.metadata} /></td>
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

function PackageDetail({ token, pushToast, apiBase }) {
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
  const variantsArr = toArray(variants);
  const failuresArr = toArray(failures);
  const eventsArr = toArray(events);
  const hintsArr = toArray(hints);
  const logDownloadHref = selectedEvent ? `${apiBase || ""}/api/logs/${selectedEvent.name}/${selectedEvent.version}` : null;

  const variantsPaged = paged(variantsArr, variantPage);
  const failuresPaged = paged(failuresArr, failurePage);
  const hintsPaged = paged(hintsArr, hintsPage);

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
          <div className="glass p-4 space-y-3 min-w-0">
            <div className="flex items-center justify-between">
              <div className="text-lg font-semibold">Events</div>
            </div>
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead className="text-slate-400 sticky top-0 bg-slate-900">
                  <tr className="border-b border-border">
                    <th className="text-left py-2">Status</th>
                    <th className="text-left py-2">Version</th>
                    <th className="text-left py-2">Artifacts</th>
                    <th className="text-left py-2">Detail</th>
                    <th className="text-left py-2">Log</th>
                  </tr>
                </thead>
                <tbody>
                  {eventsArr.map((e) => (
                    <tr key={`${e.name}-${e.version}-${e.timestamp}`} className="border-b border-slate-800">
                      <td className="py-2"><span className={`status ${e.status}`}>{e.status}</span></td>
                      <td className="py-2 text-slate-200">{e.version}</td>
                      <td className="py-2 text-slate-300"><ArtifactBadges meta={e.metadata} /></td>
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
                  {logDownloadHref && (
                    <button
                      className="btn btn-secondary px-2 py-1 text-xs"
                      onClick={async () => {
                        try {
                          await navigator.clipboard?.writeText(logDownloadHref);
                          pushToast?.({ type: "success", title: "Copied log URL", message: logDownloadHref });
                        } catch {
                          pushToast?.({ type: "error", title: "Copy failed", message: "Could not copy log URL" });
                        }
                      }}
                    >
                      Copy URL
                    </button>
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
                <div className="text-slate-400">dnf: {(h.recipes?.dnf || h.packages?.dnf || []).join(", ") || "-"}</div>
                <div className="text-slate-400">apt: {(h.recipes?.apt || h.packages?.apt || []).join(", ") || "-"}</div>
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

const STATUS_CHIPS = ["built", "failed", "retry", "reused", "cached", "missing", "skipped_known_failure"];

function Dashboard({ token, onTokenChange, pushToast, onMetrics, onApiStatus, apiBase, onApiBaseChange, view = "overview" }) {
  const navigate = useNavigate();
  const location = useLocation();
  const [authToken, setAuthToken] = useState(localStorage.getItem("refinery_token") || token || "");
  const [dashboard, setDashboard] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [retryPkg, setRetryPkg] = useState("");
  const [retryVersion, setRetryVersion] = useState("latest");
  const [selectedQueue, setSelectedQueue] = useState({});
  const [reqFile, setReqFile] = useState(null);
  const [reqError, setReqError] = useState("");
  const [wheelFile, setWheelFile] = useState(null);
  const [wheelError, setWheelError] = useState("");

  const [pkgFilter, setPkgFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [recentLimit, setRecentLimit] = useState(25);
  const [pollMs, setPollMs] = useState(10000);
  const [search, setSearch] = useState("");
  const [settingsData, setSettingsData] = useState(null);
  const [settingsDirty, setSettingsDirty] = useState(false);
  const [settingsSaving, setSettingsSaving] = useState(false);
  const [apiBaseInput, setApiBaseInput] = useState(apiBase || "");
  const [apiBlocked, setApiBlocked] = useState(false);
  const [pendingInputs, setPendingInputs] = useState([]);
  const [builds, setBuilds] = useState([]);
  const [buildsLoading, setBuildsLoading] = useState(false);
  const [buildStatusFilter, setBuildStatusFilter] = useState("");
  const [clearingBuilds, setClearingBuilds] = useState(false);
  const [clearingPendingInputs, setClearingPendingInputs] = useState(false);
  const [clearingPlanQueue, setClearingPlanQueue] = useState(false);
  const [pendingActions, setPendingActions] = useState({});
  const [pendingRefreshing, setPendingRefreshing] = useState(false);
  const [planList, setPlanList] = useState([]);
  const [planListLoading, setPlanListLoading] = useState(false);
  const [planListError, setPlanListError] = useState("");
  const [clearingPlans, setClearingPlans] = useState(false);
  const [selectedPlanId, setSelectedPlanId] = useState(null);
  const [selectedPlan, setSelectedPlan] = useState(null);
  const [planDetailsLoading, setPlanDetailsLoading] = useState(false);
  const [planDetailsError, setPlanDetailsError] = useState("");
  const [enqueueingBuilds, setEnqueueingBuilds] = useState(false);
  const [hintSearch, setHintSearch] = useState("");
  const [hintPage, setHintPage] = useState(1);
  const [hintQuery, setHintQuery] = useState("");
  const [selectedHintId, setSelectedHintId] = useState("");
  const [hintForm, setHintForm] = useState(null);
  const [hintFormError, setHintFormError] = useState("");
  const [hintSaving, setHintSaving] = useState(false);
  const [hintsState, setHintsState] = useState([]);
  const [hintsLoading, setHintsLoading] = useState(false);
  const [hintsError, setHintsError] = useState("");
  const [bulkFile, setBulkFile] = useState(null);
  const [bulkStatus, setBulkStatus] = useState(null);
  const [bulkUploading, setBulkUploading] = useState(false);
  const apiToastShown = useRef(false);
  const viewKey = view || "overview";
  const hintPageSize = 200;
  const [isVisible, setIsVisible] = useState(
    () => (typeof document !== "undefined" ? document.visibilityState === "visible" : true),
  );

  const isValidDashboard = (data) => {
    if (!data || typeof data !== "object") return false;
    const isObject = (v) => v && typeof v === "object" && !Array.isArray(v);
    if (data.summary !== null && data.summary !== undefined && !isObject(data.summary)) return false;
    if (data.recent !== null && data.recent !== undefined && !Array.isArray(data.recent)) return false;
    if (data.failures !== null && data.failures !== undefined && !Array.isArray(data.failures)) return false;
    if (data.slowest !== null && data.slowest !== undefined && !Array.isArray(data.slowest)) return false;
    if (data.queue !== null && data.queue !== undefined && !(isObject(data.queue) || Array.isArray(data.queue))) return false;
    return true;
  };

  const load = async (opts = {}) => {
    if ((apiBlocked || !isVisible) && !opts.force) {
      return;
    }
    const { packageFilter, statusFilter: status, buildStatusFilter: buildStatusOverride } = opts;
    const buildStatus = buildStatusOverride ?? buildStatusFilter;
    setLoading(true);
    const activeView = opts.view || viewKey;
    const wantsOverview = activeView === "overview";
    const wantsQueues = activeView === "queues";
    const wantsInputs = activeView === "inputs";
    const wantsBuilds = activeView === "builds";
    const wantsRecent = wantsOverview || wantsBuilds;
    if (wantsBuilds) {
      setBuildsLoading(true);
    }
    try {
      const recentPromise = wantsRecent
        ? fetchRecent(
            {
              limit: recentLimit,
              packageFilter: packageFilter ?? pkgFilter,
              status: status ?? statusFilter,
            },
            authToken,
          )
        : Promise.resolve(null);
      const summaryPromise = wantsOverview ? fetchSummary(authToken) : Promise.resolve(null);
      const failuresPromise = wantsOverview ? fetchTopFailures(10, authToken) : Promise.resolve(null);
      const slowestPromise = wantsOverview ? fetchTopSlowest(10, authToken) : Promise.resolve(null);
      const queuePromise = wantsOverview || wantsQueues ? fetchQueue(authToken) : Promise.resolve(null);
      const metricsPromise = fetchMetrics(authToken).catch(() => null);

      const [recent, summary, failures, slowest, queue, metrics] = await Promise.all([
        recentPromise,
        summaryPromise,
        failuresPromise,
        slowestPromise,
        queuePromise,
        metricsPromise,
      ]);

      const data = {
        summary: summary ?? null,
        recent: Array.isArray(recent) ? recent : [],
        failures: Array.isArray(failures) ? failures : [],
        slowest: Array.isArray(slowest) ? slowest : [],
        queue: queue ?? null,
        metrics: metrics || null,
      };
      if (!isValidDashboard(data)) {
        throw new Error("API not connected: unexpected response. Check API base or proxy.");
      }
      if (wantsInputs || wantsOverview) {
        const pending = await fetchPendingInputs(authToken).catch(() => []);
        setPendingInputs(Array.isArray(pending) ? pending : []);
      }
      if (wantsBuilds) {
        const buildsList = await fetchBuilds({ status: buildStatus || undefined }, authToken).catch(() => []);
        setBuilds(Array.isArray(buildsList) ? buildsList : []);
      }
      setDashboard((prev) => ({
        summary: data.summary ?? prev?.summary ?? null,
        recent: data.recent ?? prev?.recent ?? [],
        failures: data.failures ?? prev?.failures ?? [],
        slowest: data.slowest ?? prev?.slowest ?? [],
        queue: data.queue ?? prev?.queue ?? null,
        metrics: data.metrics ?? prev?.metrics ?? null,
      }));
      onMetrics?.(data.metrics);
      onApiStatus?.("ok");
      apiToastShown.current = false;
      setError("");
      setApiBlocked(false);
    } catch (e) {
      const msg = e.status === 403 ? "Forbidden: set a worker token" : e.message;
      const isApiOffline = msg?.toLowerCase().includes("api not connected");
      const isHttpError = Number.isFinite(e.status);
      setError(msg);
      if (!isApiOffline || !apiToastShown.current) {
        pushToast?.({ type: "error", title: "Load failed", message: msg || "Unknown error" });
        if (isApiOffline) apiToastShown.current = true;
      }
      onApiStatus?.(isApiOffline || !isHttpError ? "error" : "ok");
      if (isApiOffline) {
        setApiBlocked(true);
      }
    } finally {
      setLoading(false);
      if (wantsBuilds) {
        setBuildsLoading(false);
      }
    }
  };

  const loadHints = async (opts = {}) => {
    if ((apiBlocked || !isVisible) && !opts.force) {
      return;
    }
    const nextPage = Math.max(1, opts.page ?? hintPage);
    const nextQuery = (opts.query ?? hintQuery).trim();
    setHintsLoading(true);
    setHintsError("");
    try {
      const list = await fetchHints(
        {
          limit: hintPageSize,
          offset: (nextPage - 1) * hintPageSize,
          query: nextQuery,
        },
        authToken,
      );
      setHintsState(Array.isArray(list) ? list : []);
      setHintPage(nextPage);
      setHintQuery(nextQuery);
      setHintsError("");
    } catch (e) {
      const msg = e.message || "Failed to load hints.";
      setHintsError(msg);
      pushToast?.({ type: "error", title: "Hints load failed", message: msg });
    } finally {
      setHintsLoading(false);
    }
  };

  useEffect(() => {
    if (apiBlocked) return;
    if (!isVisible) return;
    load({ packageFilter: pkgFilter, statusFilter, view: viewKey });
  }, [authToken, pkgFilter, statusFilter, recentLimit, apiBlocked, isVisible, viewKey]);

  useEffect(() => {
    if (viewKey !== "hints") return;
    const handle = setTimeout(() => {
      loadHints({ force: true, page: 1, query: hintSearch });
    }, 300);
    return () => clearTimeout(handle);
  }, [viewKey, authToken, hintSearch, isVisible]);

  useEffect(() => {
    const loadSettings = async () => {
      try {
        const s = await fetchSettings(authToken);
        setSettingsData(s);
        if (s.recent_limit) setRecentLimit(s.recent_limit);
        if (s.poll_ms !== undefined) setPollMs(s.poll_ms || 0);
        if (s.auto_plan !== undefined) {
          setSettingsData((prev) => ({ ...(prev || s), auto_plan: !!s.auto_plan, auto_build: s.auto_build !== undefined ? !!s.auto_build : false }));
        }
        if (s.plan_pool_size !== undefined) {
          setSettingsData((prev) => ({ ...(prev || s), plan_pool_size: s.plan_pool_size }));
        }
        if (s.build_pool_size !== undefined) {
          setSettingsData((prev) => ({ ...(prev || s), build_pool_size: s.build_pool_size }));
        }
        setApiBaseInput(apiBase || getApiBase());
      } catch {
        // ignore settings load failures silently
      }
    };
    loadSettings();
  }, [authToken]);

  useEffect(() => {
    setApiBaseInput(apiBase || getApiBase());
  }, [apiBase]);

  const loadPlanList = async () => {
    setPlanListLoading(true);
    setPlanListError("");
    try {
      const list = await fetchPlans(20, authToken);
      const items = Array.isArray(list) ? list : [];
      setPlanList(items);
      if (items.length > 0) {
        const ids = new Set(items.map((p) => p.id));
        if (!selectedPlanId || !ids.has(selectedPlanId)) {
          setSelectedPlanId(items[0].id);
        }
      } else {
        setSelectedPlanId(null);
        setSelectedPlan(null);
      }
    } catch (e) {
      setPlanListError(e.message || "Failed to load plans.");
    } finally {
      setPlanListLoading(false);
    }
  };

  const loadPlanDetails = async (planId) => {
    if (!planId) return;
    setPlanDetailsLoading(true);
    setPlanDetailsError("");
    try {
      const plan = await fetchPlan(planId, authToken);
      setSelectedPlan(plan);
    } catch (e) {
      setPlanDetailsError(e.message || "Failed to load plan details.");
      setSelectedPlan(null);
    } finally {
      setPlanDetailsLoading(false);
    }
  };

  const parseLines = (value) =>
    (value || "")
      .split(/\r?\n/)
      .map((v) => v.trim())
      .filter(Boolean);

  const parseCSV = (value) =>
    (value || "")
      .split(",")
      .map((v) => v.trim())
      .filter(Boolean);

  const normalizeHintForm = (h) => ({
    id: h?.id || "",
    pattern: h?.pattern || "",
    note: h?.note || "",
    severity: h?.severity || "",
    confidence: h?.confidence || "",
    tags: (h?.tags || []).join(", "),
    examples: (h?.examples || []).join("\n"),
    appliesTo: h?.applies_to ? JSON.stringify(h.applies_to, null, 2) : "",
    recipes: {
      dnf: (h?.recipes?.dnf || []).join("\n"),
      apt: (h?.recipes?.apt || []).join("\n"),
      apk: (h?.recipes?.apk || []).join("\n"),
      brew: (h?.recipes?.brew || []).join("\n"),
    },
  });

  useEffect(() => {
    if (!pollMs || apiBlocked || !isVisible) return;
    const id = setInterval(() => load({ packageFilter: pkgFilter, statusFilter, view: viewKey }), pollMs);
    return () => clearInterval(id);
  }, [pollMs, authToken, pkgFilter, statusFilter, recentLimit, apiBlocked, isVisible, viewKey]);

  useEffect(() => {
    const handleVisibility = () => {
      if (typeof document === "undefined") return;
      setIsVisible(document.visibilityState === "visible");
    };
    if (typeof document !== "undefined") {
      document.addEventListener("visibilitychange", handleVisibility);
    }
    return () => {
      if (typeof document !== "undefined") {
        document.removeEventListener("visibilitychange", handleVisibility);
      }
    };
  }, []);

  useEffect(() => {
    if (view === "plans") {
      loadPlanList();
    }
  }, [view, authToken]);

  useEffect(() => {
    if (view === "plans" && selectedPlanId) {
      loadPlanDetails(selectedPlanId);
    }
  }, [view, selectedPlanId, authToken]);

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

  const handleClearPendingInputs = async () => {
    const pendingCount = pendingInputs.filter((pi) => pi.status === "pending").length;
    if (!pendingCount) {
      pushToast?.({ type: "info", title: "No pending inputs", message: "Nothing to clear." });
      return;
    }
    if (!window.confirm(`Clear ${pendingCount} pending input${pendingCount === 1 ? "" : "s"}?`)) return;
    setClearingPendingInputs(true);
    try {
      const resp = await clearPendingInputs("pending", authToken);
      pushToast?.({ type: "success", title: "Pending inputs cleared", message: `${resp.count ?? 0} removed` });
      await load();
    } catch (e) {
      setError(e.message);
      pushToast?.({ type: "error", title: "Clear pending inputs failed", message: e.message });
    } finally {
      setClearingPendingInputs(false);
    }
  };

const handleDeletePendingInput = async (pi) => {
    if (!pi?.id) return;
    if (!window.confirm(`Delete pending input ${pi.filename || pi.id}?`)) return;
    setPendingActions((m) => ({ ...m, [pi.id]: "delete" }));
    try {
      await deletePendingInput(pi.id, authToken);
      pushToast?.({ type: "success", title: "Pending input deleted", message: pi.filename || `ID ${pi.id}` });
      await load();
    } catch (e) {
      setError(e.message);
      pushToast?.({ type: "error", title: "Delete pending input failed", message: e.message });
    } finally {
      setPendingActions((m) => {
        const next = { ...m };
        delete next[pi.id];
        return next;
      });
    }
  };

  const handleClearPlanQueue = async () => {
    if (!window.confirm("Clear all pending plan queue items? This resets their status to pending.")) return;
    setClearingPlanQueue(true);
    try {
      const resp = await clearPlanQueue(authToken);
      pushToast?.({ type: "success", title: "Plan queue cleared", message: `${resp.cleared ?? 0} item(s) removed` });
      await load();
    } catch (e) {
      setError(e.message);
      pushToast?.({ type: "error", title: "Clear plan queue failed", message: e.message });
    } finally {
      setClearingPlanQueue(false);
    }
  };

  const handleClearBuilds = async () => {
    const clearStatus = buildStatusFilter || "pending";
    if (!window.confirm(`Clear ${clearStatus} builds from the queue?`)) return;
    setClearingBuilds(true);
    try {
      const resp = await clearBuilds(clearStatus, authToken);
      pushToast?.({ type: "success", title: "Builds cleared", message: `${resp.count ?? 0} removed (${clearStatus})` });
      await load({ packageFilter: pkgFilter, statusFilter, buildStatusFilter });
    } catch (e) {
      setError(e.message);
      pushToast?.({ type: "error", title: "Clear builds failed", message: e.message });
    } finally {
      setClearingBuilds(false);
    }
  };

  const handleClearPlans = async (planId) => {
    const label = planId ? `plan ${planId}` : "all plans";
    if (!window.confirm(`Clear ${label}?`)) return;
    setClearingPlans(true);
    try {
      const resp = await deletePlans(planId, authToken);
      pushToast?.({ type: "success", title: "Plans cleared", message: `${resp.count ?? 0} removed` });
      if (planId && selectedPlanId === planId) {
        setSelectedPlanId(null);
        setSelectedPlan(null);
      }
      await loadPlanList();
    } catch (e) {
      setError(e.message);
      pushToast?.({ type: "error", title: "Clear plans failed", message: e.message });
    } finally {
      setClearingPlans(false);
    }
  };

const enqueuePlanForInput = async (pi, verb) => {
    if (!pi?.id) return;
    setPendingActions((m) => ({ ...m, [pi.id]: "enqueue" }));
    try {
      await enqueuePlan(pi.id, authToken);
      pushToast?.({ type: "success", title: verb, message: pi.filename });
      await load();
    } catch (e) {
      pushToast?.({ type: "error", title: "Enqueue failed", message: e.message });
    } finally {
      setPendingActions((m) => {
        const next = { ...m };
        delete next[pi.id];
        return next;
      });
    }
  };

  const lintReqFile = async (file) => {
    if (!file) return "Pick a requirements.txt file";
    if (file.size === 0) return "File is empty";
    if (file.size > 128 * 1024) return "File too large (>128KB)";
    const text = await file.text();
    if (!text.trim()) return "File has no content";
    const lines = text.split(/\r?\n/);
    if (lines.length > 2000) return "Too many lines (>2000)";
    for (let i = 0; i < lines.length; i++) {
      if (lines[i].length > 800) return `Line ${i + 1} too long`;
    }
    if (text.indexOf("\u0000") !== -1) return "Invalid character (null byte)";
    return "";
  };

  const handleUploadReqs = async (trigger = false) => {
    setReqError("");
    const lintErr = await lintReqFile(reqFile);
    if (lintErr) {
      setReqError(lintErr);
      pushToast?.({ type: "error", title: "Upload failed", message: lintErr });
      return;
    }
    try {
      const resp = await uploadRequirements(reqFile, authToken);
      pushToast?.({ type: "success", title: "Uploaded", message: resp.detail || "requirements uploaded" });
      if (trigger) {
        await handleTriggerWorker();
      } else {
        await load();
      }
    } catch (e) {
      const msg = e.message || "upload failed";
      setReqError(msg);
      pushToast?.({ type: "error", title: "Upload failed", message: msg });
    }
  };

  const handleUploadWheel = async (trigger = false) => {
    setWheelError("");
    if (!wheelFile) {
      const msg = "Pick a wheel (.whl) file";
      setWheelError(msg);
      pushToast?.({ type: "error", title: "Upload failed", message: msg });
      return;
    }
    if (!wheelFile.name.toLowerCase().endsWith(".whl")) {
      const msg = "Wheel file (.whl) required";
      setWheelError(msg);
      pushToast?.({ type: "error", title: "Upload failed", message: msg });
      return;
    }
    try {
      const resp = await uploadWheel(wheelFile, authToken);
      pushToast?.({ type: "success", title: "Uploaded", message: resp.detail || "wheel uploaded" });
      if (trigger) {
        await handleTriggerWorker();
      } else {
        await load();
      }
    } catch (e) {
      const msg = e.message || "upload failed";
      setWheelError(msg);
      pushToast?.({ type: "error", title: "Upload failed", message: msg });
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

  const queueObj = dashboard?.queue && typeof dashboard.queue === "object" && !Array.isArray(dashboard.queue) ? dashboard.queue : null;
  const queueItems = toArray(queueObj?.items);
  const queueLength = Number.isFinite(queueObj?.length) ? queueObj.length : queueItems.length;
  const queueInvalid = Boolean(dashboard?.queue) && !queueObj;
  const workerMode = queueObj?.worker_mode || dashboard?.metrics?.queue?.backend || "unknown";
  const planQueueLength = dashboard?.metrics?.pending?.plan_queue ?? 0;
  const buildQueueLength = dashboard?.metrics?.build?.length ?? 0;
  const buildQueueOldest = dashboard?.metrics?.build?.oldest_age_seconds ?? "-";
  const clearBuildsLabel = buildStatusFilter ? `Clear ${buildStatusFilter} builds` : "Clear pending builds";
  const queueItemsSorted = queueItems.slice().sort((a, b) => (a.package || "").localeCompare(b.package || ""));
  const hints = toArray(hintsState);
  const metrics = dashboard?.metrics;
  const hintsCount = Number.isFinite(metrics?.hints?.count) ? metrics.hints.count : hints.length;
  const hintTotalPages = hintQuery ? null : Math.max(1, Math.ceil(hintsCount / hintPageSize));
  const hintNextDisabled = hintQuery ? hints.length < hintPageSize : hintPage >= hintTotalPages;
  const hintSuggestionValues = (() => {
    const term = hintSearch.trim().toLowerCase();
    const pool = [];
    for (const h of hints) {
      if (h?.id) pool.push(h.id);
      if (h?.pattern) pool.push(h.pattern);
      if (Array.isArray(h?.tags)) {
        pool.push(...h.tags);
      }
    }
    const seen = new Set();
    const out = [];
    for (const value of pool) {
      const val = String(value || "").trim();
      if (!val) continue;
      if (term && !val.toLowerCase().includes(term)) continue;
      if (seen.has(val)) continue;
      seen.add(val);
      out.push(val);
      if (out.length >= 8) break;
    }
    return out;
  })();
  const recent = toArray(dashboard?.recent);
  const selectedPlanNodes = toArray(selectedPlan?.plan);
  const selectedPlanBuilds = selectedPlanNodes.filter((n) => (n?.action || "").toLowerCase() === "build");
  const planPythonVersion =
    selectedPlanNodes.find((n) => n?.python_version)?.python_version ||
    selectedPlanNodes.find((n) => n?.python_tag)?.python_tag ||
    settingsData?.python_version ||
    "-";
  const planPlatformTag =
    selectedPlanNodes.find((n) => n?.platform_tag)?.platform_tag ||
    settingsData?.platform_tag ||
    "-";
  const pendingByStatus = pendingInputs.reduce(
    (acc, cur) => {
      acc[cur.status] = (acc[cur.status] || 0) + 1;
      return acc;
    },
    {},
  );
  const visiblePendingInputs = pendingInputs.filter((pi) => ["pending", "planning", "failed"].includes(pi.status));
  const pendingTotal = visiblePendingInputs.length;
  const filteredRecent = recent.filter((e) => {
    const matchPkg = search ? `${e.name} ${e.version}`.toLowerCase().includes(search.toLowerCase()) : true;
    return matchPkg;
  });
  const filteredHints = hints.filter((h) => {
    if (!hintSearch) return true;
    const haystack = [
      h.id,
      h.pattern,
      h.note,
      ...(h.tags || []),
      ...(h.recipes ? Object.values(h.recipes).flat() : []),
    ]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
    return haystack.includes(hintSearch.toLowerCase());
  });
  const bulkErrors = toArray(bulkStatus?.errors);
  const bulkErrorLines = bulkErrors.map((entry, idx) => {
    if (typeof entry === "string") return entry;
    if (entry?.errors && Array.isArray(entry.errors)) {
      const label = entry.id || entry.index || `item ${idx + 1}`;
      return `${label}: ${entry.errors.join("; ")}`;
    }
    if (entry?.error) {
      const label = entry.id || entry.index || `item ${idx + 1}`;
      return `${label}: ${entry.error}`;
    }
    return JSON.stringify(entry);
  });

  useEffect(() => {
    if (viewKey !== "hints") return;
    const params = new URLSearchParams(location.search);
    const target = params.get("hint");
    if (!target) return;
    const match = filteredHints.find((h) => h.id === target) || hints.find((h) => h.id === target);
    if (!match || selectedHintId === match.id) return;
    setSelectedHintId(match.id);
    setHintForm(normalizeHintForm(match));
    setHintFormError("");
  }, [viewKey, location.search, filteredHints, hints, selectedHintId]);

  useEffect(() => {
    if (!selectedHintId && filteredHints.length) {
      setSelectedHintId(filteredHints[0].id);
      setHintForm(normalizeHintForm(filteredHints[0]));
    }
  }, [filteredHints, selectedHintId]);

  const alerts = [];
  if (metrics?.db?.status && metrics.db.status !== "ok") {
    alerts.push(`Database is ${metrics.db.status}`);
  }
  if (metrics?.queue?.consumer_state) {
    alerts.push(`Queue note: ${metrics.queue.consumer_state}`);
  }
  if (queueInvalid) {
    alerts.push("Retry queue response is not valid JSON. Check API base or /api proxy configuration.");
  }
  if (settingsData?.auto_plan === false) {
    alerts.push("Auto-plan is off; uploads require manual plan enqueue.");
  }
  if (settingsData?.auto_build === false) {
    alerts.push("Auto-build is off; planned builds will wait for manual start.");
  }
  if (!authToken) {
    alerts.push("No worker token set; worker actions may be rejected.");
  }
  const failuresTop = toArray(dashboard?.failures);
  const planListBadge = planListLoading ? "Loading..." : `${planList.length} plans`;
  const slowestTop = toArray(dashboard?.slowest);

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

  const handleSaveSettings = async () => {
    if (!settingsData) return;
    setSettingsSaving(true);
    try {
      const trimmedBase = apiBaseInput.trim();
      if (trimmedBase) {
        localStorage.setItem("refinery_api_base", trimmedBase);
      } else {
        localStorage.removeItem("refinery_api_base");
      }
      onApiBaseChange?.(trimmedBase || getApiBase());
      const body = {
        python_version: settingsData.python_version,
        platform_tag: settingsData.platform_tag,
        poll_ms: pollMs,
        recent_limit: recentLimit,
        auto_plan: settingsData.auto_plan,
        auto_build: settingsData.auto_build,
        plan_pool_size: settingsData.plan_pool_size,
        build_pool_size: settingsData.build_pool_size,
      };
      const resp = await updateSettings(body, authToken);
      setSettingsData(resp);
      setSettingsDirty(false);
      setApiBlocked(false);
      load({ packageFilter: pkgFilter, statusFilter, force: true });
      pushToast?.({ type: "success", title: "Settings saved", message: "Defaults updated" });
    } catch (e) {
      pushToast?.({ type: "error", title: "Settings save failed", message: e.message });
    } finally {
      setSettingsSaving(false);
    }
  };

  const handleHintSave = async () => {
    if (!hintForm) return;
    setHintFormError("");
    setHintSaving(true);
    let applies = {};
    if (hintForm.appliesTo?.trim()) {
      try {
        applies = JSON.parse(hintForm.appliesTo);
      } catch (e) {
        setHintFormError("Applies-to must be valid JSON.");
        setHintSaving(false);
        return;
      }
    }
    const payload = {
      id: hintForm.id?.trim(),
      pattern: hintForm.pattern?.trim(),
      note: hintForm.note?.trim(),
      severity: hintForm.severity?.trim(),
      confidence: hintForm.confidence?.trim(),
      tags: parseCSV(hintForm.tags),
      examples: parseLines(hintForm.examples),
      applies_to: applies,
      recipes: {
        dnf: parseLines(hintForm.recipes?.dnf),
        apt: parseLines(hintForm.recipes?.apt),
        apk: parseLines(hintForm.recipes?.apk),
        brew: parseLines(hintForm.recipes?.brew),
      },
    };
    const hasRecipes = Object.values(payload.recipes).some((items) => items.length > 0);
    if (!payload.id || !payload.pattern || !payload.note) {
      setHintFormError("ID, pattern, and note are required.");
      setHintSaving(false);
      return;
    }
    if (!hasRecipes) {
      setHintFormError("At least one recipe entry is required.");
      setHintSaving(false);
      return;
    }
    try {
      const exists = hints.some((h) => h.id === payload.id);
      if (exists) {
        await updateHint(payload.id, payload, authToken);
      } else {
        await createHint(payload, authToken);
      }
      pushToast?.({ type: "success", title: "Hint saved", message: payload.id });
      await loadHints({ force: true });
      await load({ packageFilter: pkgFilter, statusFilter, force: true });
    } catch (e) {
      if (Array.isArray(e.details) && e.details.length) {
        setHintFormError(e.details.join(" "));
      }
      pushToast?.({ type: "error", title: "Hint save failed", message: e.message });
    } finally {
      setHintSaving(false);
    }
  };

  const handleHintDelete = async () => {
    if (!selectedHintId) return;
    if (!window.confirm(`Delete hint ${selectedHintId}?`)) return;
    try {
      await deleteHint(selectedHintId, authToken);
      pushToast?.({ type: "success", title: "Hint deleted", message: selectedHintId });
      setSelectedHintId("");
      setHintForm(null);
      await loadHints({ force: true });
      await load({ packageFilter: pkgFilter, statusFilter, force: true });
    } catch (e) {
      pushToast?.({ type: "error", title: "Delete failed", message: e.message });
    }
  };

  const handleBulkUpload = async () => {
    if (!bulkFile) return;
    setBulkUploading(true);
    setBulkStatus(null);
    try {
      const resp = await bulkUploadHints(bulkFile, authToken);
      setBulkStatus(resp);
      setBulkFile(null);
      pushToast?.({ type: "success", title: "Hints imported", message: `Loaded ${resp.loaded || 0}` });
      await loadHints({ force: true });
      await load({ packageFilter: pkgFilter, statusFilter, force: true });
    } catch (e) {
      pushToast?.({ type: "error", title: "Import failed", message: e.message });
    } finally {
      setBulkUploading(false);
    }
  };

  const handleEnqueueBuilds = async () => {
    if (!selectedPlanId) {
      setPlanDetailsError("Select a plan to enqueue.");
      return;
    }
    setEnqueueingBuilds(true);
    try {
      const resp = await enqueueBuildsFromPlan(selectedPlanId, authToken);
      pushToast?.({
        type: "success",
        title: "Builds enqueued",
        message: `${resp.enqueued ?? 0} builds queued`,
      });
      await load({ packageFilter: pkgFilter, statusFilter, force: true });
    } catch (e) {
      pushToast?.({ type: "error", title: "Enqueue failed", message: e.message });
    } finally {
      setEnqueueingBuilds(false);
    }
  };

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

  const renderOverview = () => (
    <>
      <div className="hero glass">
        <div className="space-y-3">
          <div>
            <p className="text-xs tracking-widest text-slate-400 uppercase">Control Plane</p>
            <h1 className="text-3xl font-extrabold text-slate-50">s390x Wheel Refinery</h1>
          </div>
          <p className="text-slate-400 text-sm max-w-2xl">
            High-level health, queue posture, and build momentum. Drill into inputs, queues, and builds from the tabs.
          </p>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3 pt-2">
            <StatTile icon="📦" label="Retry queue" value={queueLength} hint="Legacy retry backlog" />
            <StatTile icon="📥" label="Pending inputs" value={pendingTotal} hint={`waiting: ${pendingByStatus.pending || 0}, planning: ${pendingByStatus.planning || 0}`} />
            <StatTile icon="🗂️" label="Plan queue" value={planQueueLength} hint="Awaiting plan pop" />
            <StatTile icon="🚀" label="Build queue" value={buildQueueLength} hint={`Oldest: ${buildQueueOldest === "-" ? "—" : `${buildQueueOldest}s`}`} />
            <StatTile icon="🧭" label="Recent events" value={filteredRecent.length} hint="Last fetch" />
            <StatTile icon="🧠" label="Hints loaded" value={hintsCount} hint="Recipe guidance" />
          </div>
        </div>
        <div className="flex flex-col gap-3 min-w-[260px]">
          <div className="glass subtle p-4 space-y-3">
            <div className="text-xs text-slate-400 uppercase tracking-widest">System pulse</div>
            <div className="space-y-2 text-sm text-slate-200">
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Environment</span>
                <span className="chip">{ENV_LABEL}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">API</span>
                <span className="chip">{apiBase || "same-origin"}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">DB status</span>
                <span className={`chip ${metrics?.db?.status === "ok" ? "bg-emerald-900" : "bg-slate-800"}`}>{metrics?.db?.status || "unknown"}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Queue backend</span>
                <span className="chip">{metrics?.queue?.backend || "unknown"}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Auto-plan</span>
                <span className={`chip ${settingsData?.auto_plan ? "bg-emerald-900" : "bg-slate-800"}`}>
                  {settingsData?.auto_plan ? "on" : "off"}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Auto-build</span>
                <span className={`chip ${settingsData?.auto_build ? "bg-emerald-900" : "bg-slate-800"}`}>
                  {settingsData?.auto_build ? "on" : "off"}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Plan pool</span>
                <span className="chip">{settingsData?.plan_pool_size ?? "-"}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Build pool</span>
                <span className="chip">{settingsData?.build_pool_size ?? "-"}</span>
              </div>
            </div>
            <div className="space-y-1 text-xs">
              {alerts.length ? (
                alerts.map((a, idx) => <div key={idx} className="text-amber-200">• {a}</div>)
              ) : (
                <div className="text-emerald-300">All systems nominal.</div>
              )}
            </div>
          </div>
        </div>
      </div>

      {loading && !dashboard ? renderLoading() : <Summary summary={dashboard?.summary} />}

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <TopList
          title="Top failures"
          items={failuresTop}
          render={(f) => (
            <div key={f?.name || `fail-${Math.random()}`} className="flex items-center justify-between text-sm text-slate-200">
              <span>{f?.name || "unknown"}</span>
              <span className="chip">{f?.failures ?? 0} failures</span>
            </div>
          )}
        />
        <TopList
          title="Top slow packages"
          items={slowestTop}
          render={(s) => (
            <div key={s?.name || `slow-${Math.random()}`} className="flex items-center justify-between text-sm text-slate-200">
              <span>{s?.name || "unknown"}</span>
              <span className="chip">{s?.avg_duration ?? "?"}s avg</span>
            </div>
          )}
        />
        <StatCard title="Hints">
          <div className="space-y-2 max-h-52 overflow-auto text-sm text-slate-200">
            {hintsLoading ? (
              <div className="text-slate-400">Loading hints…</div>
            ) : hints.length ? (
              hints.map((h, idx) => (
                <button
                  key={idx}
                  type="button"
                  className="text-left w-full text-slate-300 border border-border rounded-lg p-2 hover:bg-slate-800/40"
                  onClick={() => {
                    if (!h?.id) return;
                    navigate(`/hints?hint=${encodeURIComponent(h.id)}`);
                  }}
                >
                  <div className="font-semibold">Pattern: {h?.pattern || "n/a"}</div>
                  <div className="text-slate-400">dnf: {(h?.recipes?.dnf || h?.packages?.dnf || []).join(", ") || "-"}</div>
                  <div className="text-slate-400">apt: {(h?.recipes?.apt || h?.packages?.apt || []).join(", ") || "-"}</div>
                </button>
              ))
            ) : (
              <div className="text-slate-400">Hints load on demand in the Hints tab.</div>
            )}
            {hintsError && <div className="text-amber-200 text-xs">{hintsError}</div>}
            {!hints.length && viewKey !== "hints" && (
              <button
                className="btn btn-secondary px-2 py-1 text-xs"
                onClick={() => loadHints({ force: true, page: 1, query: hintSearch })}
              >
                Load hints
              </button>
            )}
          </div>
        </StatCard>
        {metrics && (
          <StatCard title="Metrics snapshot">
            <div className="space-y-3 text-sm text-slate-200">
              <div className="text-slate-400 text-xs">{metrics.summary?.description || "Queue and DB health at a glance."}</div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Queue length</span>
                <span className="chip">{metrics.queue?.length ?? "?"}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Oldest age (s)</span>
                <span className="chip">{metrics.queue?.oldest_age_seconds ?? "-"}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Build queue</span>
                <span className="chip">{metrics.build?.length ?? 0}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Pending inputs</span>
                <span className="chip">{metrics.pending?.count ?? 0}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Plan queue</span>
                <span className="chip">{metrics.pending?.plan_queue ?? 0}</span>
              </div>
            </div>
          </StatCard>
        )}
      </div>

      <EventsTable events={filteredRecent} pageSize={8} />
    </>
  );

  const renderInputs = () => (
    <div className="space-y-4">
      <PageHeader
        title="Inputs & planning"
        subtitle="Manage requirements uploads and watch them advance into planning."
        badge={`${pendingTotal} pending`}
      />
      <div className="grid lg:grid-cols-3 gap-4">
        <div className="glass p-4 space-y-3">
          <div className="text-lg font-semibold flex items-center gap-2">
            <span>Upload requirements.txt</span>
            <span className="chip text-xs">📄</span>
          </div>
          <div className="space-y-2 text-sm text-slate-200">
            <input
              type="file"
              accept=".txt"
              className="input"
              onChange={(e) => {
                setReqFile(e.target.files?.[0] || null);
                setReqError("");
              }}
            />
            {reqError && <div className="text-red-300 text-xs">{reqError}</div>}
            <div className="flex flex-wrap gap-2">
              <button className="btn btn-primary" onClick={() => handleUploadReqs(false)} disabled={!reqFile}>Upload only</button>
              <button className="btn btn-secondary" onClick={() => handleUploadReqs(true)} disabled={!reqFile}>Upload & Trigger worker</button>
            </div>
            <div className="text-slate-400 text-xs">
              Lints basic text (&lt;128KB, no nulls, ≤2000 lines, ≤800 chars/line) then stores it in object storage and registers the pending input.
            </div>
          </div>
        </div>
        <div className="glass p-4 space-y-3">
          <div className="text-lg font-semibold flex items-center gap-2">
            <span>Upload wheel (.whl)</span>
            <span className="chip text-xs">🧪</span>
          </div>
          <div className="space-y-2 text-sm text-slate-200">
            <input
              type="file"
              accept=".whl"
              className="input"
              onChange={(e) => {
                setWheelFile(e.target.files?.[0] || null);
                setWheelError("");
              }}
            />
            {wheelError && <div className="text-red-300 text-xs">{wheelError}</div>}
            <div className="flex flex-wrap gap-2">
              <button className="btn btn-primary" onClick={() => handleUploadWheel(false)} disabled={!wheelFile}>Upload wheel</button>
              <button className="btn btn-secondary" onClick={() => handleUploadWheel(true)} disabled={!wheelFile}>Upload & Trigger worker</button>
            </div>
            <div className="text-slate-400 text-xs">
              Parses wheel metadata on upload, stores the immutable input in object storage, and registers it for planning.
            </div>
          </div>
        </div>
        <div className="glass p-4 space-y-3">
          <div className="flex items-center justify-between gap-2 flex-wrap">
            <div className="text-lg font-semibold flex items-center gap-2 whitespace-nowrap" style={{ whiteSpace: "nowrap" }}>
              <span>Pending inputs</span>
              <span className="chip text-xs">🧾</span>
            </div>
            <div className="flex flex-wrap gap-2 whitespace-nowrap">
              <button
                className="btn btn-secondary px-2 py-1 text-xs"
                onClick={async () => {
                  setPendingRefreshing(true);
                  try {
                    await load({ force: true });
                  } finally {
                    setPendingRefreshing(false);
                  }
                }}
                disabled={pendingRefreshing}
              >
                {pendingRefreshing ? "Refreshing..." : "Refresh"}
              </button>
              {pendingInputs.some((pi) => pi.status === "pending") && (
                <button
                  className="btn btn-secondary px-2 py-1 text-xs"
                  onClick={handleClearPendingInputs}
                  disabled={clearingPendingInputs}
                >
                  {clearingPendingInputs ? "Clearing..." : "Clear pending inputs"}
                </button>
              )}
            </div>
          </div>
          {visiblePendingInputs.length === 0 ? (
            <EmptyState title="No pending uploads" detail="New uploads will appear here until planned." icon="✅" />
          ) : (
            <div className="flex flex-col gap-2 text-sm text-slate-200">
              {visiblePendingInputs.map((pi) => (
                <div key={pi.id} className="glass subtle px-3 py-3 rounded-lg space-y-3 w-full">
                  <div className="flex items-start gap-3">
                    <div className="space-y-2 min-w-0 flex-1">
                      <div className="font-semibold text-slate-100 truncate" title={pi.filename}>
                        {pi.filename}
                      </div>
                      <div className="flex flex-wrap gap-2 text-xs text-slate-500">
                        <span className="chip">{pi.status}</span>
                        {pi.source_type && <span className="chip">{pi.source_type}</span>}
                        {pi.size_bytes > 0 && <span className="chip">{formatBytes(pi.size_bytes)}</span>}
                        <span className="text-slate-500">id {pi.id}</span>
                      </div>
                      {pi.error && <div className="text-red-300 text-xs">{pi.error}</div>}
                    </div>
                    <div className="flex flex-col gap-2 items-stretch w-[120px] shrink-0">
                      {pi.status === "pending" && (
                        <button
                          className="btn btn-secondary px-2 py-1 text-xs"
                          disabled={!!pendingActions[pi.id]}
                          onClick={() => enqueuePlanForInput(pi, "Enqueued for planning")}
                        >
                          {pendingActions[pi.id] ? "Enqueuing..." : "Enqueue"}
                        </button>
                      )}
                      {pi.status === "failed" && (
                        <button
                          className="btn btn-secondary px-2 py-1 text-xs"
                          disabled={!!pendingActions[pi.id]}
                          onClick={() => enqueuePlanForInput(pi, "Replan queued")}
                        >
                          {pendingActions[pi.id] ? "Replanning..." : "Replan"}
                        </button>
                      )}
                      <button
                        className="btn btn-secondary px-2 py-1 text-xs"
                        disabled={!!pendingActions[pi.id]}
                        onClick={() => handleDeletePendingInput(pi)}
                      >
                        {pendingActions[pi.id] === "delete" ? "Deleting..." : "Delete"}
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );

  const renderQueues = () => (
    <div className="space-y-4">
      <PageHeader
        title="Queues & retries"
        subtitle="Manage legacy retry queue and trigger worker runs."
        badge={`${queueLength} retry`}
      />
      <div className="grid lg:grid-cols-[360px,1fr] gap-4 items-start">
        <div className="space-y-4">
          <div className="glass p-4 space-y-3">
            <div className="text-lg font-semibold flex items-center gap-2">
              <span>Queue controls</span>
              <span className="chip text-xs">🧰</span>
            </div>
            <div className="space-y-2 text-sm text-slate-200">
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Retry queue</span>
                <span className="chip">{queueLength}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Worker mode</span>
                <span className="chip">{workerMode}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Plan queue</span>
                <div className="flex items-center gap-2">
                  <span className="chip">{metrics?.pending?.plan_queue ?? 0}</span>
                  <button
                    className="btn btn-secondary px-2 py-1 text-xs"
                    onClick={handleClearPlanQueue}
                    disabled={clearingPlanQueue || !planQueueLength}
                  >
                    {clearingPlanQueue ? "Clearing..." : "Clear plan queue"}
                  </button>
                </div>
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
        <div className="glass p-4 space-y-3">
          <div className="text-lg font-semibold flex items-center gap-2">
            <span>Retry queue items</span>
            <span className="chip text-xs">📦</span>
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
                    <th className="text-left px-2 py-2">Attempts</th>
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
                        <td className="px-2 py-2 text-slate-400">{q.attempts ?? 0}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          ) : (
            <EmptyState title="Queue is empty" detail="No retry requests pending." actionLabel="Refresh" onAction={() => load({ packageFilter: pkgFilter, statusFilter })} />
          )}
        </div>
      </div>
    </div>
  );

  const renderPlans = () => (
    <div className="space-y-4">
      <PageHeader
        title="Plans"
        subtitle="Review planned DAGs, inspect nodes, and manually enqueue builds when auto-build is off."
        badge={planListBadge}
      />
      <div className="grid lg:grid-cols-[360px,1fr] gap-4 items-start">
        <div className="glass p-4 space-y-3">
          <div className="flex items-center justify-between gap-2">
            <div className="text-lg font-semibold flex items-center gap-2">
              <span>Plan library</span>
              <span className="chip text-xs">🗂️</span>
            </div>
            <div className="flex flex-wrap gap-2">
              <button className="btn btn-secondary px-2 py-1 text-xs" onClick={loadPlanList} disabled={planListLoading}>
                Refresh
              </button>
              <button
                className="btn btn-secondary px-2 py-1 text-xs"
                onClick={() => handleClearPlans(0)}
                disabled={clearingPlans || !planList.length}
              >
                {clearingPlans ? "Clearing..." : "Clear all"}
              </button>
            </div>
          </div>
          <div className="text-xs text-slate-400">
            Plan queue: {planQueueLength}. Auto-build is {settingsData?.auto_build ? "on" : "off"}.
          </div>
          {planListLoading ? (
            <div className="text-xs text-slate-500">Loading plans…</div>
          ) : planList.length ? (
            <div className="space-y-2 max-h-80 overflow-auto text-sm">
              {planList.map((plan) => {
                const selected = plan.id === selectedPlanId;
                return (
                  <button
                    key={plan.id}
                    className={`w-full text-left border border-border rounded-lg p-2 transition ${selected ? "bg-slate-800/60" : "hover:bg-slate-800/30"}`}
                    onClick={() => setSelectedPlanId(plan.id)}
                  >
                    <div className="flex items-center justify-between">
                      <span className="font-semibold text-slate-100">Plan #{plan.id}</span>
                      <span className="text-xs text-slate-500">{formatEpoch(plan.created_at)}</span>
                    </div>
                    <div className="text-xs text-slate-400 flex flex-wrap gap-2 mt-1">
                      {plan.run_id && <span className="chip">run {plan.run_id}</span>}
                      <span className="chip">{plan.build_count ?? 0} builds</span>
                      <span className="chip">{plan.node_count ?? 0} nodes</span>
                    </div>
                  </button>
                );
              })}
            </div>
          ) : (
            <div className="text-xs text-slate-500">No plans available yet.</div>
          )}
          {planListError && <div className="text-xs text-amber-200">{planListError}</div>}
        </div>
        <div className="glass subtle p-4 space-y-3">
          <div className="text-lg font-semibold flex items-center gap-2">
            <span>Selected plan</span>
            <span className="chip text-xs">📌</span>
          </div>
          {planDetailsLoading ? (
            <div className="text-xs text-slate-500">Loading plan details…</div>
          ) : selectedPlan ? (
            <div className="space-y-2 text-sm text-slate-200">
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Plan ID</span>
                <span className="chip">{selectedPlan.id}</span>
              </div>
              {selectedPlan.run_id && (
                <div className="flex items-center justify-between">
                  <span className="text-slate-400">Run ID</span>
                  <span className="chip">{selectedPlan.run_id}</span>
                </div>
              )}
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Build nodes</span>
                <span className="chip">{selectedPlanBuilds.length}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Total nodes</span>
                <span className="chip">{selectedPlanNodes.length}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Python version</span>
                <span className="chip">{planPythonVersion}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Platform tag</span>
                <span className="chip">{planPlatformTag}</span>
              </div>
              <div className="flex flex-wrap gap-2">
                <button className="btn btn-primary" onClick={handleEnqueueBuilds} disabled={enqueueingBuilds}>
                  {enqueueingBuilds ? "Enqueueing..." : "Enqueue builds"}
                </button>
                <button className="btn btn-secondary" onClick={() => loadPlanDetails(selectedPlan.id)} disabled={planDetailsLoading}>
                  Refresh details
                </button>
                <button
                  className="btn btn-secondary"
                  onClick={() => handleClearPlans(selectedPlan.id)}
                  disabled={clearingPlans}
                >
                  {clearingPlans ? "Clearing..." : "Clear selected"}
                </button>
              </div>
              {selectedPlanNodes.length > 0 ? (
                <div className="max-h-64 overflow-auto text-xs text-slate-300 space-y-1">
                  {selectedPlanNodes.slice(0, 12).map((node, idx) => (
                    <div key={`${node.name}-${node.version}-${idx}`} className="flex items-center justify-between">
                      <span>{node.name} {node.version}</span>
                      <span className="chip">{node.action}</span>
                    </div>
                  ))}
                  {selectedPlanNodes.length > 12 && (
                    <div className="text-slate-500">…and {selectedPlanNodes.length - 12} more</div>
                  )}
                </div>
              ) : (
                <div className="text-xs text-slate-500">No nodes available for this plan.</div>
              )}
            </div>
          ) : (
            <div className="text-xs text-slate-500">{planDetailsError || "Select a plan to review details."}</div>
          )}
          {planDetailsError && selectedPlan && <div className="text-xs text-amber-200">{planDetailsError}</div>}
        </div>
      </div>
    </div>
  );

  const renderBuilds = () => (
    <div className="space-y-4">
      <PageHeader
        title="Build queue & events"
        subtitle="Monitor queued builds and recent execution events. Manage plan libraries on the Plans page."
        badge={`${buildQueueLength} queued`}
      />
      <div className="grid lg:grid-cols-[280px,1fr] gap-4 items-start min-w-0">
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
                <input className="input w-1/2" placeholder="Recent limit" title="How many recent events to show" value={recentLimit} onChange={(e) => setRecentLimit(Number(e.target.value) || 25)} />
                <input className="input w-1/2" placeholder="Poll ms (0=off)" title="Refresh cadence in milliseconds" value={pollMs} onChange={(e) => setPollMs(Number(e.target.value) || 0)} />
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              {STATUS_CHIPS.map((s) => {
                const active = statusFilter === s;
                return (
                  <button
                    key={s}
                    className={`chip cursor-pointer ${active ? "chip-active" : "hover:bg-slate-800"}`}
                    onClick={() => setStatusFilter(active ? "" : s)}
                    title="Toggle status filter"
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
              <button
                className="btn btn-secondary px-2 py-1 text-xs"
                onClick={() => {
                  setPkgFilter("");
                  setSearch("");
                  setStatusFilter("");
                  load({ packageFilter: "", statusFilter: "" });
                }}
              >
                Clear all
              </button>
            </div>
          </div>
        </div>
        <div className="space-y-4 min-w-0">
          <div className="glass p-4 space-y-3">
            <div className="flex items-center justify-between gap-3">
              <div className="text-lg font-semibold flex items-center gap-2">
                <span>Build queue</span>
                <span className="chip text-xs">🏗️</span>
              </div>
              <div className="flex items-center gap-2 text-xs text-slate-400">
                {buildsLoading && <span className="animate-pulse">Refreshing…</span>}
                <button
                  className="btn btn-secondary px-2 py-1 text-xs"
                  onClick={handleClearBuilds}
                  disabled={clearingBuilds || !builds.length}
                >
                  {clearingBuilds ? "Clearing..." : clearBuildsLabel}
                </button>
              </div>
            </div>
            <div className="flex flex-wrap gap-2 text-sm">
              {["", "pending", "retry", "building", "failed", "built"].map((s) => (
                <button
                  key={s || "all"}
                  className={`chip ${buildStatusFilter === s ? "chip-active" : "hover:bg-slate-800"}`}
                  onClick={() => {
                    setBuildStatusFilter(s);
                    load({ packageFilter: pkgFilter, statusFilter, buildStatusFilter: s });
                  }}
                >
                  {s || "all"}
                </button>
              ))}
            </div>
            <div className="text-xs text-slate-400">Oldest queued: {buildQueueOldest === "-" ? "—" : `${buildQueueOldest}s`}</div>
            <div className="overflow-x-auto">
              <table className="min-w-full w-full table-fixed text-xs border border-border rounded-lg" aria-busy={buildsLoading}>
                <thead className="bg-slate-900 text-slate-400 sticky top-0">
                  <tr>
                    <th className="text-left px-2 py-2">Package</th>
                    <th className="text-left px-2 py-2">Version</th>
                    <th className="text-left px-2 py-2">Status</th>
                    <th className="text-left px-2 py-2">Attempts</th>
                    <th className="text-left px-2 py-2">Python</th>
                    <th className="text-left px-2 py-2">Platform</th>
                    <th className="text-left px-2 py-2">Error</th>
                  </tr>
                </thead>
                <tbody>
                  {builds.length ? builds.map((b, idx) => (
                    <tr
                      key={`${b.package}-${b.version}-${idx}`}
                      className="border-t border-slate-800 cursor-pointer hover:bg-slate-900/40"
                      onClick={() => navigate(`/package/${encodeURIComponent(b.package)}`)}
                      title="View package details"
                    >
                      <td className="px-2 py-2">{b.package}</td>
                      <td className="px-2 py-2">{b.version}</td>
                      <td className="px-2 py-2"><span className="chip">{b.status}</span></td>
                      <td className="px-2 py-2">{b.attempts ?? 0}</td>
                      <td className="px-2 py-2 text-slate-400">{b.python_tag || "-"}</td>
                      <td className="px-2 py-2 text-slate-400">{b.platform_tag || "-"}</td>
                      <td className="px-2 py-2 text-slate-400 truncate max-w-[220px]">{b.last_error || "-"}</td>
                    </tr>
                  )) : (
                    <tr>
                      <td className="px-2 py-3 text-slate-400" colSpan="7">No builds found</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
          <EventsTable events={filteredRecent} />
        </div>
      </div>
    </div>
  );

  const renderSettingsView = () => (
    <div className="space-y-4">
      <PageHeader
        title="Settings & access"
        subtitle="Tune defaults, polling, and worker pool sizing."
        badge={settingsDirty ? "Unsaved changes" : "Defaults"}
      />
      <div className="grid lg:grid-cols-2 gap-4">
        <div className="glass subtle p-4 space-y-2">
          <div className="text-xs text-slate-400">Worker token</div>
          <div className="text-xs text-slate-500">
            Required for any action that enqueues work or updates control-plane state. Paste the shared token issued for workers;
            it is stored locally in this browser and attached as the <span className="chip chip-muted">X-Worker-Token</span> header
            on API calls. Until provided, queue actions and worker-trigger operations may be rejected.
          </div>
          <input
            className="input"
            placeholder="Worker token (optional)"
            value={authToken}
            onChange={(e) => setAuthToken(e.target.value)}
          />
          <div className="text-xs text-slate-400 pt-2">API base URL</div>
          <div className="text-xs text-slate-500">
            Set the control-plane base URL (for example: <span className="chip chip-muted">http://localhost:8080</span>).
            Leave blank to use the same-origin <span className="chip chip-muted">/api</span> proxy.
          </div>
          <input
            className="input"
            placeholder="http://control-plane:8080"
            value={apiBaseInput}
            onChange={(e) => {
              setApiBaseInput(e.target.value);
              setSettingsDirty(true);
            }}
          />
          <div className="flex gap-2">
            <button className="btn btn-primary w-full" onClick={handleSaveToken}>Save</button>
            <button className="btn btn-secondary w-full" onClick={() => load({ packageFilter: pkgFilter, statusFilter })} disabled={loading}>Refresh</button>
          </div>
          <div className="text-xs text-slate-500">Token required for queue and build actions.</div>
        </div>
        <div className="glass p-4 space-y-3">
          <div className="flex items-center justify-between">
            <div className="text-lg font-semibold flex items-center gap-2">
              <span>Defaults</span>
              <span className="chip text-xs">⚙️</span>
            </div>
            <button className="btn btn-secondary px-2 py-1 text-xs" onClick={() => handleSaveSettings()} disabled={settingsSaving || !settingsData}>
              {settingsSaving ? "Saving..." : "Save defaults"}
            </button>
          </div>
          <div className="grid md:grid-cols-2 gap-3 text-sm text-slate-200">
            <div className="space-y-1">
              <div className="text-xs text-slate-400">Python version</div>
              <input
                className="input"
                placeholder="e.g. 3.11"
                value={settingsData?.python_version || ""}
                onChange={(e) => {
                  setSettingsData((s) => ({ ...(s || {}), python_version: e.target.value }));
                  setSettingsDirty(true);
                }}
              />
            </div>
            <div className="space-y-1">
              <div className="text-xs text-slate-400">Platform tag</div>
              <input
                className="input"
                placeholder="manylinux2014_s390x"
                value={settingsData?.platform_tag || ""}
                onChange={(e) => {
                  setSettingsData((s) => ({ ...(s || {}), platform_tag: e.target.value }));
                  setSettingsDirty(true);
                }}
              />
            </div>
            <div className="space-y-1">
              <div className="text-xs text-slate-400">Recent limit</div>
              <input
                className="input"
                type="number"
                value={recentLimit}
                onChange={(e) => {
                  setRecentLimit(Number(e.target.value) || 25);
                  setSettingsDirty(true);
                }}
              />
            </div>
            <div className="space-y-1">
              <div className="text-xs text-slate-400">Poll (ms, 0=off)</div>
              <input
                className="input"
                type="number"
                value={pollMs}
                onChange={(e) => {
                  setPollMs(Number(e.target.value) || 0);
                  setSettingsDirty(true);
                }}
              />
            </div>
            <div className="space-y-1">
              <div className="text-xs text-slate-400">Plan worker pool</div>
              <input
                className="input"
                type="number"
                min="1"
                value={settingsData?.plan_pool_size || 0}
                onChange={(e) => {
                  setSettingsData((s) => ({ ...(s || {}), plan_pool_size: Number(e.target.value) || 1 }));
                  setSettingsDirty(true);
                }}
              />
            </div>
            <div className="space-y-1">
              <div className="text-xs text-slate-400">Build worker pool</div>
              <input
                className="input"
                type="number"
                min="1"
                value={settingsData?.build_pool_size || 0}
                onChange={(e) => {
                  setSettingsData((s) => ({ ...(s || {}), build_pool_size: Number(e.target.value) || 1 }));
                  setSettingsDirty(true);
                }}
              />
            </div>
          </div>
          <div className="grid md:grid-cols-2 gap-3 text-sm text-slate-200">
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={!!settingsData?.auto_plan}
                onChange={(e) => {
                  setSettingsData((s) => ({ ...(s || {}), auto_plan: e.target.checked }));
                  setSettingsDirty(true);
                }}
              />
              <span>Auto-plan new uploads</span>
            </label>
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={!!settingsData?.auto_build}
                onChange={(e) => {
                  setSettingsData((s) => ({ ...(s || {}), auto_build: e.target.checked }));
                  setSettingsDirty(true);
                }}
              />
              <span>Auto-build planned items</span>
            </label>
          </div>
          <div className="text-xs text-slate-500">
            Defaults inform queue enqueues and UI polling limits. Worker runtime Python still follows the configured worker image/env.
          </div>
        </div>
      </div>
    </div>
  );

  const renderHintsView = () => (
    <div className="space-y-4">
      <PageHeader
        title="Hint catalog"
        subtitle="Search, edit, and bulk import hint recipes for common build failures."
        badge={`${hintsCount} total`}
      />
      {hintsError && (
        <div className="glass p-3 border border-amber-500/40 text-sm text-amber-200 flex items-center justify-between">
          <span>{hintsError}</span>
          <button className="btn btn-secondary px-2 py-1 text-xs" onClick={() => loadHints({ force: true })}>Retry</button>
        </div>
      )}
      <div className="grid lg:grid-cols-[320px,1fr] gap-4 items-stretch">
        <div className="glass p-4 space-y-3 flex flex-col min-h-[520px]">
          <div className="text-lg font-semibold flex items-center gap-2">
            <span>Hints</span>
            <span className="chip text-xs">🧠</span>
            {hintsLoading && <span className="text-xs text-slate-400">Loading…</span>}
          </div>
          <input
            className="input"
            placeholder="Search by id, pattern, tags"
            value={hintSearch}
            onChange={(e) => setHintSearch(e.target.value)}
            list="hint-search-suggestions"
          />
          <datalist id="hint-search-suggestions">
            {hintSuggestionValues.map((value) => (
              <option key={value} value={value} />
            ))}
          </datalist>
          <div className="flex items-center justify-between text-xs text-slate-400">
            <span>{hintTotalPages ? `Page ${hintPage} / ${hintTotalPages}` : `Page ${hintPage}`}</span>
            <div className="flex items-center gap-2">
              <button
                className="btn btn-secondary px-2 py-1 text-xs"
                disabled={hintPage === 1 || hintsLoading}
                onClick={() => loadHints({ page: Math.max(1, hintPage - 1), query: hintSearch })}
              >
                Prev
              </button>
              <button
                className="btn btn-secondary px-2 py-1 text-xs"
                disabled={hintNextDisabled || hintsLoading}
                onClick={() => loadHints({ page: hintPage + 1, query: hintSearch })}
              >
                Next
              </button>
            </div>
          </div>
          <button
            className="btn btn-secondary w-full"
            onClick={() => {
              setSelectedHintId("");
              setHintForm(normalizeHintForm({}));
              setHintFormError("");
            }}
          >
            New hint
          </button>
          <div className="space-y-2 text-sm flex-1 min-h-0 overflow-auto">
            {filteredHints.length ? filteredHints.map((h) => (
              <button
                key={h.id}
                className={`w-full text-left border border-border rounded-lg p-2 hover:bg-slate-800/40 ${selectedHintId === h.id ? "bg-slate-800/60" : ""}`}
                onClick={() => {
                  setSelectedHintId(h.id);
                  setHintForm(normalizeHintForm(h));
                  setHintFormError("");
                }}
              >
                <div className="font-semibold text-slate-100">{h.id}</div>
                <div className="text-xs text-slate-400 truncate">{h.pattern}</div>
              </button>
            )) : <EmptyState title="No hints" detail="No hints match your search." />}
          </div>
        </div>
        <div className="glass p-4 space-y-3 flex flex-col min-h-[520px]">
          <div className="flex items-center justify-between">
            <div className="text-lg font-semibold flex items-center gap-2">
              <span>{selectedHintId ? "Edit hint" : "Create hint"}</span>
              <span className="chip text-xs">✍️</span>
            </div>
            {selectedHintId && (
              <button className="btn btn-secondary px-2 py-1 text-xs" onClick={handleHintDelete}>Delete</button>
            )}
          </div>
          {!hintForm ? (
            <div className="flex-1 min-h-0 flex items-center">
              <EmptyState title="Select a hint" detail="Pick a hint from the list to edit or create a new one." />
            </div>
          ) : (
            <div className="space-y-3 text-sm text-slate-200 flex-1 min-h-0 overflow-auto pr-1">
              <div className="grid md:grid-cols-2 gap-3">
                <div className="space-y-1">
                  <div className="text-xs text-slate-400">ID</div>
                  <input
                    className="input"
                    placeholder="unique-id"
                    value={hintForm.id}
                    onChange={(e) => setHintForm((s) => ({ ...s, id: e.target.value }))}
                  />
                </div>
                <div className="space-y-1">
                  <div className="text-xs text-slate-400">Severity</div>
                  <input
                    className="input"
                    placeholder="error|warn|info"
                    value={hintForm.severity}
                    onChange={(e) => setHintForm((s) => ({ ...s, severity: e.target.value }))}
                  />
                </div>
              </div>
              <div className="space-y-1">
                <div className="text-xs text-slate-400">Pattern</div>
                <input
                  className="input"
                  placeholder="regex or literal"
                  value={hintForm.pattern}
                  onChange={(e) => setHintForm((s) => ({ ...s, pattern: e.target.value }))}
                />
              </div>
              <div className="space-y-1">
                <div className="text-xs text-slate-400">Note</div>
                <textarea
                  className="input min-h-[80px]"
                  placeholder="Explain the fix and why it helps."
                  value={hintForm.note}
                  onChange={(e) => setHintForm((s) => ({ ...s, note: e.target.value }))}
                />
              </div>
              <div className="grid md:grid-cols-2 gap-3">
                <div className="space-y-1">
                  <div className="text-xs text-slate-400">Tags (comma)</div>
                  <input
                    className="input"
                    placeholder="ssl, missing-header"
                    value={hintForm.tags}
                    onChange={(e) => setHintForm((s) => ({ ...s, tags: e.target.value }))}
                  />
                </div>
                <div className="space-y-1">
                  <div className="text-xs text-slate-400">Confidence</div>
                  <input
                    className="input"
                    placeholder="low|medium|high"
                    value={hintForm.confidence}
                    onChange={(e) => setHintForm((s) => ({ ...s, confidence: e.target.value }))}
                  />
                </div>
              </div>
              <div className="grid md:grid-cols-2 gap-3">
                <div className="space-y-1">
                  <div className="text-xs text-slate-400">Examples (one per line)</div>
                  <textarea
                    className="input min-h-[90px]"
                    value={hintForm.examples}
                    onChange={(e) => setHintForm((s) => ({ ...s, examples: e.target.value }))}
                  />
                </div>
                <div className="space-y-1">
                  <div className="text-xs text-slate-400">Applies-to (JSON)</div>
                  <textarea
                    className="input min-h-[90px]"
                    placeholder='{"platforms":["linux"],"arch":["s390x"]}'
                    value={hintForm.appliesTo}
                    onChange={(e) => setHintForm((s) => ({ ...s, appliesTo: e.target.value }))}
                  />
                </div>
              </div>
              <div className="grid md:grid-cols-2 gap-3">
                <div className="space-y-1">
                  <div className="text-xs text-slate-400">dnf packages</div>
                  <textarea className="input min-h-[80px]" value={hintForm.recipes.dnf} onChange={(e) => setHintForm((s) => ({ ...s, recipes: { ...s.recipes, dnf: e.target.value } }))} />
                </div>
                <div className="space-y-1">
                  <div className="text-xs text-slate-400">apt packages</div>
                  <textarea className="input min-h-[80px]" value={hintForm.recipes.apt} onChange={(e) => setHintForm((s) => ({ ...s, recipes: { ...s.recipes, apt: e.target.value } }))} />
                </div>
                <div className="space-y-1">
                  <div className="text-xs text-slate-400">apk packages</div>
                  <textarea className="input min-h-[80px]" value={hintForm.recipes.apk} onChange={(e) => setHintForm((s) => ({ ...s, recipes: { ...s.recipes, apk: e.target.value } }))} />
                </div>
                <div className="space-y-1">
                  <div className="text-xs text-slate-400">brew packages</div>
                  <textarea className="input min-h-[80px]" value={hintForm.recipes.brew} onChange={(e) => setHintForm((s) => ({ ...s, recipes: { ...s.recipes, brew: e.target.value } }))} />
                </div>
              </div>
              {hintFormError && <div className="text-amber-300 text-xs">{hintFormError}</div>}
              <button className="btn btn-primary" onClick={handleHintSave} disabled={hintSaving}>
                {hintSaving ? "Saving..." : "Save hint"}
              </button>
            </div>
          )}
        </div>
      </div>
      <div className="glass p-4 space-y-3">
        <div className="text-lg font-semibold flex items-center gap-2">
          <span>Bulk import</span>
          <span className="chip text-xs">📦</span>
        </div>
        <input
          type="file"
          accept=".yaml,.yml,.json"
          className="input"
          onChange={(e) => setBulkFile(e.target.files?.[0] || null)}
        />
        <div className="flex flex-wrap gap-2">
          <button className="btn btn-primary" onClick={handleBulkUpload} disabled={!bulkFile || bulkUploading}>
            {bulkUploading ? "Uploading..." : "Upload & seed"}
          </button>
        </div>
        {bulkStatus && (
          <div className="text-xs text-slate-400">
            Loaded: {bulkStatus.loaded || 0} · Skipped: {bulkStatus.skipped || 0} · Errors: {(bulkStatus.errors || []).length}
          </div>
        )}
        {bulkErrorLines.length > 0 && (
          <div className="text-xs text-amber-200 space-y-1">
            {bulkErrorLines.slice(0, 6).map((line, idx) => (
              <div key={`${line}-${idx}`}>{line}</div>
            ))}
            {bulkErrorLines.length > 6 && <div>…and {bulkErrorLines.length - 6} more</div>}
          </div>
        )}
      </div>
    </div>
  );

  const renderView = () => {
    switch (viewKey) {
      case "inputs":
        return renderInputs();
      case "queues":
        return renderQueues();
      case "plans":
        return renderPlans();
      case "builds":
        return renderBuilds();
      case "hints":
        return renderHintsView();
      case "settings":
        return renderSettingsView();
      default:
        return renderOverview();
    }
  };

  return (
    <div className="max-w-6xl mx-auto px-4 py-6 space-y-4">
      {error && (
        <div className="glass p-3 border border-red-500/40 text-sm text-red-200 flex items-center justify-between">
          <span>{error}</span>
          <button className="btn btn-secondary px-2 py-1 text-xs" onClick={() => load({ packageFilter: pkgFilter, statusFilter, force: true })}>Retry</button>
        </div>
      )}
      {message && <div className="text-green-400 text-sm">{message}</div>}
      {renderView()}
    </div>
  );
}

export default function App() {
  const [token, setToken] = useState(localStorage.getItem("refinery_token") || "");
  const [toasts, setToasts] = useState([]);
  const [theme, setTheme] = useState(() => localStorage.getItem("refinery_theme") || "dark");
  const [metrics, setMetrics] = useState(null);
  const [apiBase, setApiBase] = useState(getApiBase());
  const [apiStatus, setApiStatus] = useState("unknown");

  const dismissToast = (id) => setToasts((ts) => ts.filter((t) => t.id !== id));
  const pushToast = ({ type = "success", title, message }) => {
    const id = `${Date.now()}-${Math.random()}`;
    setToasts((ts) => [...ts, { id, type, title, message }]);
    setTimeout(() => dismissToast(id), 4000);
  };

  const toggleTheme = () => {
    setTheme((t) => {
      const next = t === "light" ? "dark" : "light";
      localStorage.setItem("refinery_theme", next);
      return next;
    });
  };

  return (
    <Layout tokenActive={Boolean(token)} theme={theme} onToggleTheme={toggleTheme} metrics={metrics} apiBase={apiBase} apiStatus={apiStatus}>
      <Routes>
        <Route path="/" element={<Navigate to="/overview" replace />} />
        <Route path="/overview" element={<Dashboard token={token} onTokenChange={setToken} pushToast={pushToast} onMetrics={setMetrics} onApiStatus={setApiStatus} apiBase={apiBase} onApiBaseChange={setApiBase} view="overview" />} />
        <Route path="/inputs" element={<Dashboard token={token} onTokenChange={setToken} pushToast={pushToast} onMetrics={setMetrics} onApiStatus={setApiStatus} apiBase={apiBase} onApiBaseChange={setApiBase} view="inputs" />} />
        <Route path="/plans" element={<Dashboard token={token} onTokenChange={setToken} pushToast={pushToast} onMetrics={setMetrics} onApiStatus={setApiStatus} apiBase={apiBase} onApiBaseChange={setApiBase} view="plans" />} />
        <Route path="/queues" element={<Dashboard token={token} onTokenChange={setToken} pushToast={pushToast} onMetrics={setMetrics} onApiStatus={setApiStatus} apiBase={apiBase} onApiBaseChange={setApiBase} view="queues" />} />
        <Route path="/builds" element={<Dashboard token={token} onTokenChange={setToken} pushToast={pushToast} onMetrics={setMetrics} onApiStatus={setApiStatus} apiBase={apiBase} onApiBaseChange={setApiBase} view="builds" />} />
        <Route path="/hints" element={<Dashboard token={token} onTokenChange={setToken} pushToast={pushToast} onMetrics={setMetrics} onApiStatus={setApiStatus} apiBase={apiBase} onApiBaseChange={setApiBase} view="hints" />} />
        <Route path="/settings" element={<Dashboard token={token} onTokenChange={setToken} pushToast={pushToast} onMetrics={setMetrics} onApiStatus={setApiStatus} apiBase={apiBase} onApiBaseChange={setApiBase} view="settings" />} />
        <Route path="/package/:name" element={<PackageDetail token={token} pushToast={pushToast} apiBase={apiBase} />} />
      </Routes>
      <Toasts toasts={toasts} onDismiss={dismissToast} />
    </Layout>
  );
}
