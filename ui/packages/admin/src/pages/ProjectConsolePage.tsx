import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  Button,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import {
  AdminApiClient,
  type ProxyRequest,
  type ProxyResponse,
  type TopologyInstance,
  type TopologyProject,
} from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

const api = new AdminApiClient();

const COMBINED_BUFFER_CAP = 5000;

interface MergedLine {
  /** Epoch millis if a timestamp could be parsed from the line; else 0. */
  ts: number;
  instance: string;
  line: string;
  /** Stable arrival counter — tiebreaker when ts is equal or absent. */
  arrival: number;
}

/**
 * ProjectConsolePage — multi-instance combined timeline + API console.
 *
 * Combined timeline: subscribe to per-instance SSE log streams, tag
 * each line with the instance name, render in a single feed. Sort by
 * parsed leading timestamp when present, otherwise interleave by
 * arrival order. Toggle individual instances on/off.
 *
 * API console: pick an instance, free-form HTTP request (method, path,
 * headers, body), fire via the admin proxy endpoint (server-side dial
 * to the instance's port — no browser CORS dance), inspect response.
 */
export function ProjectConsolePage() {
  const { project } = useParams<{ project: string }>();
  if (!project) {
    return <ErrorPanel message="missing project name in route" />;
  }

  const {
    data: topology,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["topology"],
    queryFn: () => api.topology(),
  });

  if (isLoading) return <Spinner label="loading topology" />;
  if (isError) return <ErrorPanel message={error?.message} />;
  if (!topology) return <Spinner label="loading topology" />;

  const projectCfg = (topology.projects ?? []).find((p) => p.name === project);
  if (!projectCfg) {
    return (
      <ErrorPanel
        message={`project "${project}" not found in topology`}
      />
    );
  }
  const instances = projectCfg.instances ?? [];

  return (
    <div className="flex flex-col gap-6">
      <PageHeading
        kicker={`admin · topology · ${project}`}
        title={<>Console</>}
        meta={
          <span className="inline-flex items-center gap-3">
            <Link
              to="/ui/topology"
              style={{ color: "var(--color-accent)", textDecoration: "none" }}
            >
              ← topology
            </Link>
            <span>{instances.length} instance{instances.length === 1 ? "" : "s"}</span>
          </span>
        }
      />

      <CombinedTimeline project={project} project_={projectCfg} />
      <ApiConsole project={project} instances={instances} />
    </div>
  );
}

function CombinedTimeline({
  project,
  project_,
}: {
  project: string;
  project_: TopologyProject;
}) {
  const instances = project_.instances ?? [];
  const [enabled, setEnabled] = useState<Record<string, boolean>>(() => {
    const init: Record<string, boolean> = {};
    instances.forEach((i) => (init[i.name] = true));
    return init;
  });
  const [paused, setPaused] = useState(false);
  const [sortByTime, setSortByTime] = useState(true);
  const [merged, setMerged] = useState<MergedLine[]>([]);
  const arrivalCounter = useRef(0);

  const handleLine = useCallback((instance: string, line: string) => {
    setMerged((prev) => {
      const next = [
        ...prev,
        {
          ts: parseTimestamp(line),
          instance,
          line,
          arrival: arrivalCounter.current++,
        },
      ];
      return next.length > COMBINED_BUFFER_CAP
        ? next.slice(next.length - COMBINED_BUFFER_CAP)
        : next;
    });
  }, []);

  const display = useMemo(() => {
    if (!sortByTime) return merged;
    return [...merged].sort((a, b) => {
      if (a.ts === 0 && b.ts === 0) return a.arrival - b.arrival;
      if (a.ts === 0) return 1;
      if (b.ts === 0) return -1;
      if (a.ts !== b.ts) return a.ts - b.ts;
      return a.arrival - b.arrival;
    });
  }, [merged, sortByTime]);

  return (
    <section
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-sm)",
      }}
    >
      <header
        className="flex flex-wrap items-center justify-between gap-3 px-4 py-3"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <div>
          <div
            className="font-display"
            style={{
              fontStyle: "italic",
              fontWeight: 600,
              fontSize: "1rem",
              letterSpacing: "-0.02em",
            }}
          >
            Combined timeline
          </div>
          <div
            className="mt-0.5 font-mono uppercase tracking-[0.18em]"
            style={{ color: "var(--color-fg-subtle)", fontSize: "0.62rem" }}
          >
            {display.length} buffered · sort{sortByTime ? " by time" : " by arrival"}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setSortByTime((s) => !s)}
          >
            {sortByTime ? "arrival" : "time"} sort
          </Button>
          <Button
            variant={paused ? "primary" : "secondary"}
            size="sm"
            onClick={() => setPaused((p) => !p)}
          >
            {paused ? "resume" : "pause"}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => setMerged([])}>
            clear
          </Button>
        </div>
      </header>

      <div className="px-4 py-3">
        <div className="mb-3 flex flex-wrap gap-1.5">
          {instances.map((inst) => (
            <button
              key={inst.name}
              type="button"
              onClick={() =>
                setEnabled((e) => ({ ...e, [inst.name]: !e[inst.name] }))
              }
              className="font-mono uppercase tracking-[0.12em]"
              style={{
                fontSize: "0.65rem",
                padding: "0.25rem 0.6rem",
                background: enabled[inst.name]
                  ? "var(--color-accent-soft)"
                  : "var(--color-bg-subtle)",
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-xs)",
                color: enabled[inst.name]
                  ? "var(--color-accent)"
                  : "var(--color-fg-subtle)",
                cursor: "pointer",
              }}
            >
              {inst.name}
            </button>
          ))}
        </div>

        {/* Hooks per instance, fixed order — ensures Rules of Hooks. */}
        {instances.map((inst) => (
          <StreamSubscriber
            key={inst.name}
            project={project}
            instance={inst.name}
            enabled={!!enabled[inst.name] && !paused}
            onLine={handleLine}
          />
        ))}

        <CombinedLogPane lines={display} />
      </div>
    </section>
  );
}

function CombinedLogPane({ lines }: { lines: MergedLine[] }) {
  if (lines.length === 0) {
    return (
      <div
        className="px-4 py-6 font-mono uppercase tracking-[0.2em] text-center"
        style={{
          background: "var(--color-bg-subtle)",
          border: "1px solid var(--color-border)",
          borderRadius: "var(--radius-sm)",
          color: "var(--color-fg-subtle)",
          fontSize: "0.7rem",
        }}
      >
        — no log output —
      </div>
    );
  }
  return (
    <div
      className="overflow-auto"
      style={{
        maxHeight: "50vh",
        background: "oklch(0.13 0.01 60)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-sm)",
      }}
    >
      <pre
        className="font-mono"
        style={{
          padding: "0.85rem 1rem",
          fontSize: "0.72rem",
          lineHeight: 1.55,
          color: "oklch(0.92 0.005 80)",
          margin: 0,
        }}
      >
        {lines.map((l) => (
          <div key={l.arrival} className="flex">
            <span
              className="mr-3 inline-block select-none text-right"
              style={{
                width: "8rem",
                color: instanceColor(l.instance),
                paddingRight: "0.5rem",
              }}
            >
              {l.instance}
            </span>
            <span style={{ flex: 1, whiteSpace: "pre-wrap", wordBreak: "break-all" }}>
              {l.line}
            </span>
          </div>
        ))}
      </pre>
    </div>
  );
}

/** Stable per-instance color from a small palette. */
function instanceColor(name: string): string {
  const palette = [
    "oklch(0.78 0.13 30)",
    "oklch(0.78 0.13 80)",
    "oklch(0.78 0.13 150)",
    "oklch(0.78 0.13 220)",
    "oklch(0.78 0.13 280)",
    "oklch(0.78 0.13 340)",
  ];
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0;
  return palette[h % palette.length]!;
}

/**
 * StreamSubscriber owns one EventSource and forwards each `data:` line
 * to onLine. Returns null. Closes/reopens on `enabled` flips so the
 * caller can pause individual instances without unsubscribing the
 * page.
 */
function StreamSubscriber({
  project,
  instance,
  enabled,
  onLine,
}: {
  project: string;
  instance: string;
  enabled: boolean;
  onLine: (instance: string, line: string) => void;
}) {
  // Latch the latest onLine in a ref so we don't re-open the stream
  // every time the parent re-renders the callback.
  const onLineRef = useRef(onLine);
  useEffect(() => {
    onLineRef.current = onLine;
  }, [onLine]);

  useEffect(() => {
    if (!enabled) return;
    const url = new URL(
      `/api/v1/topology/projects/${encodeURIComponent(project)}/instances/${encodeURIComponent(instance)}/logs`,
      window.location.origin,
    );
    url.searchParams.set("follow", "1");
    url.searchParams.set("lines", "200");

    const es = new EventSource(url.toString());
    es.onmessage = (ev) => {
      onLineRef.current(instance, ev.data);
    };
    return () => es.close();
  }, [project, instance, enabled]);

  return null;
}

/**
 * parseTimestamp tries to extract an epoch-millis timestamp from the
 * leading characters of a log line. Recognises ISO-8601 prefixes and
 * common JSON shapes (`{"time":...}`, `{"ts":...}`,
 * `{"timestamp":...}`). Returns 0 on miss.
 */
export function parseTimestamp(line: string): number {
  const isoMatch = line.match(
    /^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)/,
  );
  if (isoMatch) {
    const t = Date.parse(isoMatch[1]!);
    if (!Number.isNaN(t)) return t;
  }
  if (line.startsWith("{")) {
    try {
      const obj = JSON.parse(line);
      const v = obj.time ?? obj.ts ?? obj.timestamp;
      if (typeof v === "string") {
        const t = Date.parse(v);
        if (!Number.isNaN(t)) return t;
      }
      if (typeof v === "number") {
        return v > 1e12 ? v : v * 1000;
      }
    } catch {
      /* not JSON; fall through */
    }
  }
  return 0;
}

function ApiConsole({
  project,
  instances,
}: {
  project: string;
  instances: TopologyInstance[];
}) {
  const [target, setTarget] = useState(instances[0]?.name ?? "");
  const [method, setMethod] = useState("GET");
  const [path, setPath] = useState("/v1/health");
  const [headersText, setHeadersText] = useState("");
  const [bodyText, setBodyText] = useState("");
  const [response, setResponse] = useState<ProxyResponse | null>(null);

  const mutation = useMutation({
    mutationFn: (req: ProxyRequest) =>
      api.topologyInstanceProxy(project, target, req),
    onSuccess: (resp) => setResponse(resp),
    onError: () => setResponse(null),
  });

  const submit = () => {
    if (!target) return;
    const headers = parseHeaders(headersText);
    const req: ProxyRequest = { method, path };
    if (Object.keys(headers).length > 0) req.headers = headers;
    if (bodyText && method !== "GET" && method !== "HEAD") {
      req.body = bodyText;
    }
    mutation.mutate(req);
  };

  return (
    <section
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-sm)",
      }}
    >
      <header
        className="px-4 py-3"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <div
          className="font-display"
          style={{
            fontStyle: "italic",
            fontWeight: 600,
            fontSize: "1rem",
            letterSpacing: "-0.02em",
          }}
        >
          API console
        </div>
        <div
          className="mt-0.5 font-mono uppercase tracking-[0.18em]"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.62rem" }}
        >
          arbitrary HTTP request via admin proxy
        </div>
      </header>

      <div className="grid gap-3 p-4 md:grid-cols-2">
        <div className="flex flex-col gap-2">
          <Label>instance</Label>
          <select
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            className="font-mono"
            style={fieldStyle}
          >
            {instances.map((inst) => (
              <option key={inst.name} value={inst.name}>
                {inst.name} (:{inst.port})
              </option>
            ))}
          </select>

          <div className="grid gap-2 sm:grid-cols-[8rem_1fr]">
            <div className="flex flex-col gap-2">
              <Label>method</Label>
              <select
                value={method}
                onChange={(e) => setMethod(e.target.value)}
                className="font-mono"
                style={fieldStyle}
              >
                {["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"].map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </select>
            </div>
            <div className="flex flex-col gap-2">
              <Label>path</Label>
              <input
                type="text"
                value={path}
                onChange={(e) => setPath(e.target.value)}
                className="font-mono"
                style={fieldStyle}
                placeholder="/v1/health"
              />
            </div>
          </div>

          <Label>headers (one per line, key: value)</Label>
          <textarea
            value={headersText}
            onChange={(e) => setHeadersText(e.target.value)}
            rows={3}
            className="font-mono"
            style={{ ...fieldStyle, minHeight: "4rem", resize: "vertical" }}
            placeholder="Content-Type: application/json"
          />

          <Label>body</Label>
          <textarea
            value={bodyText}
            onChange={(e) => setBodyText(e.target.value)}
            rows={6}
            className="font-mono"
            style={{ ...fieldStyle, minHeight: "8rem", resize: "vertical" }}
            disabled={method === "GET" || method === "HEAD"}
            placeholder={method === "GET" || method === "HEAD" ? "(body disabled for GET/HEAD)" : ""}
          />

          <div className="mt-2">
            <Button
              variant="primary"
              size="sm"
              onClick={submit}
              disabled={mutation.isPending || !target}
            >
              {mutation.isPending ? "sending…" : "send"}
            </Button>
          </div>
        </div>

        <div className="flex flex-col gap-2">
          <Label>response</Label>
          {mutation.isError && (
            <ErrorPanel message={mutation.error?.message ?? "request failed"} />
          )}
          {response && <ResponsePanel response={response} />}
          {!mutation.isError && !response && (
            <div
              className="px-4 py-6 font-mono uppercase tracking-[0.2em] text-center"
              style={{
                background: "var(--color-bg-subtle)",
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-sm)",
                color: "var(--color-fg-subtle)",
                fontSize: "0.7rem",
              }}
            >
              — send a request to populate —
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

function ResponsePanel({ response }: { response: ProxyResponse }) {
  const isJson = (response.headers["Content-Type"] ?? "").includes("application/json");
  const prettyBody = useMemo(() => {
    if (!isJson || !response.body) return response.body;
    try {
      return JSON.stringify(JSON.parse(response.body), null, 2);
    } catch {
      return response.body;
    }
  }, [isJson, response.body]);

  const statusToken =
    response.status >= 200 && response.status < 300
      ? "ok"
      : response.status >= 500
        ? "error"
        : "warning";

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-3">
        <StatusBadge status={statusToken} />
        <span
          className="font-mono"
          style={{ color: "var(--color-fg)", fontSize: "0.85rem" }}
        >
          {response.status} {response.status_text}
        </span>
        <span
          className="font-mono"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.72rem" }}
        >
          {response.duration_ms} ms
        </span>
      </div>

      {Object.keys(response.headers).length > 0 && (
        <details
          className="font-mono"
          style={{
            background: "var(--color-bg-subtle)",
            border: "1px solid var(--color-border)",
            borderRadius: "var(--radius-xs)",
            padding: "0.4rem 0.6rem",
            fontSize: "0.7rem",
          }}
        >
          <summary
            className="cursor-pointer uppercase tracking-[0.16em]"
            style={{ color: "var(--color-fg-subtle)" }}
          >
            headers ({Object.keys(response.headers).length})
          </summary>
          <ul className="mt-2">
            {Object.entries(response.headers).map(([k, v]) => (
              <li key={k}>
                <span style={{ color: "var(--color-fg-muted)" }}>{k}</span>:
                <span className="ml-2" style={{ color: "var(--color-fg)" }}>
                  {v}
                </span>
              </li>
            ))}
          </ul>
        </details>
      )}

      <pre
        className="font-mono"
        style={{
          background: "oklch(0.13 0.01 60)",
          color: "oklch(0.92 0.005 80)",
          padding: "0.85rem 1rem",
          fontSize: "0.72rem",
          lineHeight: 1.5,
          margin: 0,
          maxHeight: "30vh",
          overflow: "auto",
          borderRadius: "var(--radius-sm)",
          whiteSpace: "pre-wrap",
          wordBreak: "break-all",
        }}
      >
        {prettyBody || "— empty body —"}
      </pre>
    </div>
  );
}

function Label({ children }: { children: React.ReactNode }) {
  return (
    <label
      className="font-mono uppercase tracking-[0.18em]"
      style={{ color: "var(--color-fg-subtle)", fontSize: "0.62rem" }}
    >
      {children}
    </label>
  );
}

const fieldStyle: React.CSSProperties = {
  background: "var(--color-bg)",
  color: "var(--color-fg)",
  border: "1px solid var(--color-border)",
  borderRadius: "var(--radius-xs)",
  padding: "0.35rem 0.55rem",
  fontSize: "0.78rem",
};

function parseHeaders(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const idx = trimmed.indexOf(":");
    if (idx <= 0) continue;
    const k = trimmed.slice(0, idx).trim();
    const v = trimmed.slice(idx + 1).trim();
    if (k) out[k] = v;
  }
  return out;
}
