import { useEffect, useMemo, useState } from "react";
import { Routes, Route, Link, useParams } from "react-router-dom";
import { enqueueRetry, fetchDashboard, fetchLog, fetchPackageDetail, fetchRecent, setCookieToken, triggerWorker } from "./api";

function StatCard({ title, children }) {
  return (
    <div className="card">
      <div className="title">{title}</div>
      {children}
    </div>
  );
}

function Summary({ summary }) {
  if (!summary) return null;
  const { status_counts = {}, failures = [] } = summary;
  return (
    <div className="grid">
      <StatCard title="Status counts (recent)">
        <div className="list">
          {Object.entries(status_counts).map(([k, v]) => (
            <div key={k} className="row" style={{ justifyContent: "space-between" }}>
              <span className="muted">{k}</span>
              <span className="badge">{v}</span>
            </div>
          ))}
        </div>
      </StatCard>
      <StatCard title="Recent failures">
        <div className="list">
          {failures.map((f) => (
            <div key={`${f.name}-${f.version}`} className="row" style={{ justifyContent: "space-between" }}>
              <span>{f.name} {f.version}</span>
              <span className="badge">{f.status}</span>
            </div>
          ))}
          {!failures.length && <div className="muted">No recent failures</div>}
        </div>
      </StatCard>
    </div>
  );
}

function EventsTable({ events }) {
  if (!events?.length) return <div className="muted">No events yet.</div>;
  return (
    <div className="card">
      <div className="title">Recent events</div>
      <table className="table">
        <thead>
          <tr>
            <th>Status</th>
            <th>Package</th>
            <th>Python/Platform</th>
            <th>Detail</th>
          </tr>
        </thead>
        <tbody>
          {events.map((e) => (
            <tr key={`${e.name}-${e.version}-${e.timestamp}`}>
              <td><span className={`status ${e.status}`}>{e.status}</span></td>
              <td><Link to={`/package/${e.name}`}>{e.name} {e.version}</Link></td>
              <td className="muted">{e.python_tag}/{e.platform_tag}</td>
              <td className="muted">{e.detail || ""}</td>
            </tr>
          ))}
        </tbody>
      </table>
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

function PackageDetail({ token }) {
  const { name } = useParams();
  const [data, setData] = useState(null);
  const [logContent, setLogContent] = useState("");
  const [selectedEvent, setSelectedEvent] = useState(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState("");

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
    load();
  }, [name, token]);

  const loadLog = async (ev) => {
    setSelectedEvent(ev);
    setLogContent("");
    setMessage("");
    try {
      const resp = await fetchLog(ev.name, ev.version, token);
      if (resp.content) {
        setLogContent(resp.content);
      } else {
        setMessage("No log content available");
      }
    } catch (e) {
      setError(e.message);
    }
  };

  if (loading) return <div className="muted">Loading package...</div>;
  if (error) return <div className="error">{error}</div>;
  if (!data) return null;

  const { summary, variants, failures, events } = data;

  return (
    <div className="layout">
      <div className="row" style={{ justifyContent: "space-between" }}>
        <div>
          <h2 style={{ margin: 0 }}>{summary.name}</h2>
          <div className="muted">Status counts: {Object.entries(summary.status_counts || {}).map(([k, v]) => `${k}:${v}`).join("  ")}</div>
          {summary.latest && <div className="muted">Latest: {summary.latest.status} {summary.latest.version} at {summary.latest.timestamp}</div>}
        </div>
        <Link to="/" className="button secondary">Back</Link>
      </div>
      <div className="grid" style={{ marginTop: 16 }}>
        <StatCard title="Recent failures">
          <div className="list">
            {failures?.length ? failures.map((f) => (
              <div key={`${f.name}-${f.version}-${f.timestamp}`} className="row" style={{ justifyContent: "space-between" }}>
                <span>{f.name} {f.version}</span>
                <span className="badge">{f.status}</span>
              </div>
            )) : <div className="muted">No failures</div>}
          </div>
        </StatCard>
        <StatCard title="Variants">
          <div className="list">
            {variants?.length ? variants.map((v, idx) => (
              <div key={idx} className="row" style={{ justifyContent: "space-between" }}>
                <span className="muted">{v.metadata?.variant || "unknown"}</span>
                <span className="badge">{v.status}</span>
              </div>
            )) : <div className="muted">No variant history</div>}
          </div>
        </StatCard>
      </div>

      <div className="card" style={{ marginTop: 16 }}>
        <div className="title">Events</div>
        <table className="table">
          <thead>
            <tr>
              <th>Status</th>
              <th>Version</th>
              <th>Detail</th>
              <th>Log</th>
            </tr>
          </thead>
          <tbody>
            {events.map((e) => (
              <tr key={`${e.name}-${e.version}-${e.timestamp}`}>
                <td><span className={`status ${e.status}`}>{e.status}</span></td>
                <td>{e.version}</td>
                <td className="muted">{e.detail || ""}</td>
                <td><button className="button secondary" onClick={() => loadLog(e)}>View log</button></td>
              </tr>
            ))}
          </tbody>
        </table>
        {message && <div className="muted" style={{ marginTop: 6 }}>{message}</div>}
        {selectedEvent && (
          <div className="card" style={{ marginTop: 12 }}>
            <div className="row" style={{ justifyContent: "space-between" }}>
              <div className="title">Log: {selectedEvent.name} {selectedEvent.version}</div>
              <span className="muted">{selectedEvent.timestamp}</span>
            </div>
            <pre style={{ maxHeight: 300, overflow: "auto", background: "#0b1220", padding: 12, borderRadius: 8 }}>{logContent || "No content"}</pre>
          </div>
        )}
      </div>
    </div>
  );
}

function Dashboard({ token, onTokenChange }) {
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
      await load();
    } catch (e) {
      setError(e.message);
    }
  };

  const handleRetry = async () => {
    setMessage("");
    if (!retryPkg) {
      setError("Enter a package name");
      return;
    }
    try {
      const resp = await enqueueRetry(retryPkg, retryVersion, authToken);
      setMessage(resp.detail || "Enqueued");
      await load();
    } catch (e) {
      setError(e.message);
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
  };

  const queueLength = dashboard?.queue?.length ?? 0;
  const workerMode = dashboard?.queue?.worker_mode || "unknown";
  const queueItems = dashboard?.queue?.items || [];
  const hints = dashboard?.hints || [];
  const metrics = dashboard?.metrics;
  const recent = dashboard?.recent || [];

  return (
    <div className="layout">
      <div className="header">
        <div>
          <h1 style={{ margin: 0 }}>s390x Wheel Refinery</h1>
          <div className="muted">Data-driven control plane (React SPA)</div>
        </div>
        <div className="row">
          <input
            className="input"
            style={{ width: 200 }}
            placeholder="Worker token (optional)"
            value={authToken}
            onChange={(e) => setAuthToken(e.target.value)}
          />
          <button className="button secondary" onClick={handleSaveToken}>Save token</button>
          <button className="button secondary" onClick={load} disabled={loading}>Refresh</button>
        </div>
      </div>

      {error && <div className="error" style={{ marginBottom: 12 }}>{error}</div>}
      {message && <div className="success" style={{ marginBottom: 12 }}>{message}</div>}

      <Summary summary={dashboard?.summary} />

      <div className="card" style={{ marginTop: 12 }}>
        <div className="row" style={{ gap: 12 }}>
          <input className="input" style={{ maxWidth: 200 }} placeholder="Filter package" value={pkgFilter} onChange={(e) => setPkgFilter(e.target.value)} />
          <input className="input" style={{ maxWidth: 200 }} placeholder="Filter status (built,failed,...)" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} />
          <input className="input" style={{ maxWidth: 140 }} placeholder="Recent limit" value={recentLimit} onChange={(e) => setRecentLimit(Number(e.target.value) || 25)} />
          <input className="input" style={{ maxWidth: 180 }} placeholder="Poll ms (0=off)" value={pollMs} onChange={(e) => setPollMs(Number(e.target.value) || 0)} />
        </div>
      </div>

      <div className="grid" style={{ marginTop: 16 }}>
        <TopList
          title="Top failures"
          items={dashboard?.failures || []}
          render={(f) => (
            <div key={f.name} className="row" style={{ justifyContent: "space-between" }}>
              <span>{f.name}</span>
              <span className="badge">{f.failures} failures</span>
            </div>
          )}
        />
        <TopList
          title="Top slow packages"
          items={dashboard?.slowest || []}
          render={(s) => (
            <div key={s.name} className="row" style={{ justifyContent: "space-between" }}>
              <span>{s.name}</span>
              <span className="badge">{s.avg_duration}s avg</span>
            </div>
          )}
        />
        <StatCard title="Queue & worker">
          <div className="list">
            <div className="row" style={{ justifyContent: "space-between" }}>
              <span className="muted">Queue length</span>
              <span className="badge">{queueLength}</span>
            </div>
            <div className="row" style={{ justifyContent: "space-between" }}>
              <span className="muted">Worker mode</span>
              <span className="badge">{workerMode}</span>
            </div>
            <button className="button" onClick={handleTriggerWorker}>Run worker now</button>
            {queueItems.length > 0 && (
              <div className="muted">
                Queue items: {queueItems.map((q) => `${q.package}@${q.version || "latest"}`).join(", ")}
              </div>
            )}
          </div>
        </StatCard>
        <StatCard title="Hints">
          <div className="list" style={{ maxHeight: 200, overflow: "auto" }}>
            {hints.length ? hints.map((h, idx) => (
              <div key={idx} className="muted">
                <div>Pattern: {h.pattern}</div>
                <div>dnf: {(h.packages?.dnf || []).join(", ") || "-"}</div>
                <div>apt: {(h.packages?.apt || []).join(", ") || "-"}</div>
              </div>
            )) : <div className="muted">No hints loaded</div>}
          </div>
        </StatCard>
      </div>

      {metrics && (
        <div className="grid" style={{ marginTop: 16 }}>
          <StatCard title="Metrics snapshot">
            <div className="list">
              <div className="row" style={{ justifyContent: "space-between" }}>
                <span className="muted">Queue length</span>
                <span className="badge">{metrics.queue_length}</span>
              </div>
              <div className="row" style={{ justifyContent: "space-between" }}>
                <span className="muted">Worker mode</span>
                <span className="badge">{metrics.worker_mode || "unknown"}</span>
              </div>
            </div>
          </StatCard>
        </div>
      )}

      <div className="card" style={{ marginTop: 16 }}>
        <div className="title">Enqueue retry</div>
        <div className="row" style={{ marginTop: 8 }}>
          <input className="input" placeholder="package name" value={retryPkg} onChange={(e) => setRetryPkg(e.target.value)} />
          <input className="input" style={{ maxWidth: 180 }} placeholder="version (or latest)" value={retryVersion} onChange={(e) => setRetryVersion(e.target.value)} />
          <button className="button" onClick={handleRetry}>Enqueue</button>
        </div>
        <div className="muted" style={{ marginTop: 6 }}>
          Uses API: POST /package/&lt;name&gt;/retry (adds hint-derived recipes automatically).
        </div>
      </div>

      <EventsTable events={recent} />
    </div>
  );
}

export default function App() {
  const [token, setToken] = useState(localStorage.getItem("refinery_token") || "");
  return (
    <Routes>
      <Route path="/" element={<Dashboard token={token} onTokenChange={setToken} />} />
      <Route path="/package/:name" element={<PackageDetail token={token} />} />
    </Routes>
  );
}
