import { describe, expect, it, beforeEach, vi } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import App from "./App.jsx";

const mockData = {
  "/api/summary": { status_counts: { built: 1 }, failures: [] },
  "/api/recent": [{ name: "pkg", version: "1.0", status: "built", python_tag: "cp311", platform_tag: "manylinux2014_s390x", detail: "ok", timestamp: "now" }],
  "/api/top-failures": [{ name: "bad", failures: 2 }],
  "/api/top-slowest": [{ name: "slow", avg_duration: 12, failures: 0 }],
  "/api/queue": { length: 0, worker_mode: "local", items: [] },
  "/api/hints": [{ pattern: "missing", packages: { dnf: ["foo"], apt: [] } }],
  "/api/metrics": { queue_length: 0, worker_mode: "local", status_counts: {} },
  "/api/package/pkg": { name: "pkg", status_counts: { built: 1 }, latest: { status: "built", version: "1.0", timestamp: "now" } },
  "/api/variants/pkg": [{ metadata: { variant: "default" }, status: "built" }],
  "/api/failures?name=pkg&limit=50": [],
  "/api/recent?package=pkg&limit=50": [{ name: "pkg", version: "1.0", status: "built", detail: "ok", timestamp: "now" }],
  "/api/logs/pkg/1.0": { content: "build log" },
  "/api/queue/enqueue": { detail: "enqueued" },
  "/api/worker/trigger": { detail: "ok" },
};

beforeEach(() => {
  global.fetch = vi.fn((url, opts = {}) => {
    const path = url.replace(/^http:\/\/localhost:3000/, "");
    const key = Object.keys(mockData).find((k) => path.startsWith(k));
    const body = key ? mockData[key] : mockData["/api/summary"]; // fallback to avoid hard 404 in tests
    const status = key ? 200 : 200;
    return Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
});

describe("App dashboard", () => {
  it("renders dashboard and shows recent events", async () => {
    render(
      <MemoryRouter>
        <App />
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getAllByText(/s390x Wheel Refinery/i).length).toBeGreaterThan(0));
    expect(screen.getByText(/pkg 1.0/)).toBeInTheDocument();
    expect(screen.getAllByText(/Queue length/i).length).toBeGreaterThan(0);
  });

  it("allows enqueue retry", async () => {
    render(
      <MemoryRouter>
        <App />
      </MemoryRouter>,
    );
    await waitFor(() => screen.getByText(/Queue length/i));
    fireEvent.change(screen.getByPlaceholderText(/package name/i), { target: { value: "pkg" } });
    fireEvent.click(screen.getAllByText(/Enqueue/i)[1]);
    await waitFor(() => {
      const called = (global.fetch.mock.calls || []).some(([url, opts]) => url.includes("/api/queue/enqueue") && opts?.body?.includes("\"pkg\""));
      expect(called).toBe(true);
    });
  });

  it("navigates to package detail and loads log", async () => {
    render(
      <MemoryRouter initialEntries={["/package/pkg"]}>
        <App />
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByText(/pkg/i)).toBeInTheDocument());
    fireEvent.click(screen.getByText(/Events & Logs/i));
    fireEvent.click(screen.getByText(/View log/i));
    await waitFor(() => expect(screen.getByText(/build log/i)).toBeInTheDocument());
  });
});
