import { useEffect, useState } from "react";
import { enqueueRetry, fetchDashboard, setCookieToken, triggerWorker } from "./api";

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
              <td>{e.name} {e.version}</td>
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

export default function App() {
  const [token, setToken] = useState(localStorage.getItem("refinery_token") || "");
  const [dashboard, setDashboard] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [retryPkg, setRetryPkg] = useState("");
  const [retryVersion, setRetryVersion] = useState("latest");

  const load = async () => {
    setLoading(true);
    setError("");
    try {
      const data = await fetchDashboard(token);
      setDashboard(data);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, [token]);

  const handleTriggerWorker = async () => {
    setMessage("");
    try {
      const resp = await triggerWorker(token);
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
      const resp = await enqueueRetry(retryPkg, retryVersion, token);
      setMessage(resp.detail || "Enqueued");
      await load();
    } catch (e) {
      setError(e.message);
    }
  };

  const handleSaveToken = async () => {
    localStorage.setItem("refinery_token", token);
    if (token) {
      try {
        await setCookieToken(token);
      } catch {
        // ignore
      }
    }
    setMessage("Token saved");
  };

  const queueLength = dashboard?.queue?.length ?? 0;
  const workerMode = dashboard?.queue?.worker_mode || "unknown";

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
            value={token}
            onChange={(e) => setToken(e.target.value)}
          />
          <button className="button secondary" onClick={handleSaveToken}>Save token</button>
          <button className="button secondary" onClick={load} disabled={loading}>Refresh</button>
        </div>
      </div>

      {error && <div className="error" style={{ marginBottom: 12 }}>{error}</div>}
      {message && <div className="success" style={{ marginBottom: 12 }}>{message}</div>}

      <Summary summary={dashboard?.summary} />

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
          </div>
        </StatCard>
      </div>

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

      <EventsTable events={dashboard?.recent} />
    </div>
  );
}
