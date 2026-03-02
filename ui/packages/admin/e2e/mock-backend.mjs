// Minimal mock backend server for Playwright E2E tests.
// Responds to the management API endpoints that the admin server proxies.
import { createServer } from "node:http";

const port = parseInt(process.env.MOCK_BACKEND_PORT || "19100", 10);
let startedAt = Date.now();

const containers = [
  { id: "abc123def456", name: "my-web-app", image: "nginx:latest", state: "running", created: "2026-03-01T10:00:00Z" },
  { id: "789xyz000111", name: "my-worker", image: "alpine:3.19", state: "exited", created: "2026-03-01T09:00:00Z" },
];

const resources = [
  { containerId: "abc123def456", backend: "memory", resourceType: "container", resourceId: "res-001", instanceId: "inst-1", createdAt: "2026-03-01T10:00:00Z", cleanedUp: false, status: "active" },
];

const server = createServer((req, res) => {
  res.setHeader("Content-Type", "application/json");

  if (req.url === "/internal/v1/healthz") {
    res.end(JSON.stringify({
      status: "ok",
      component: "backend",
      uptime_seconds: Math.floor((Date.now() - startedAt) / 1000),
    }));
  } else if (req.url === "/internal/v1/status") {
    res.end(JSON.stringify({
      status: "ok",
      component: "backend",
      backend_type: "memory",
      instance_id: "mock-001",
      uptime_seconds: Math.floor((Date.now() - startedAt) / 1000),
      containers: containers.length,
      active_resources: resources.length,
      context: "test",
    }));
  } else if (req.url === "/internal/v1/containers/summary") {
    res.end(JSON.stringify(containers));
  } else if (req.url === "/internal/v1/metrics") {
    res.end(JSON.stringify({
      requests: { "GET /v1.47/containers/json": 42, "POST /v1.47/containers/create": 5 },
      latency_ms: { "GET /v1.47/containers/json": { p50: 1.2, p95: 3.4, p99: 5.6 } },
      goroutines: 12,
      heap_alloc_mb: 8.5,
      uptime_seconds: Math.floor((Date.now() - startedAt) / 1000),
      containers: containers.length,
      active_resources: resources.length,
    }));
  } else if (req.url?.startsWith("/internal/v1/resources")) {
    res.end(JSON.stringify(resources));
  } else if (req.url === "/internal/v1/reload" && req.method === "POST") {
    res.end(JSON.stringify({ status: "ok", context: "test", changed: 0 }));
  } else {
    res.statusCode = 404;
    res.end(JSON.stringify({ error: "not found" }));
  }
});

server.listen(port, () => {
  console.log(`Mock backend listening on :${port}`);
});
