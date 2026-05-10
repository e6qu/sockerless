import { useState } from "react";
import { Link, useParams } from "react-router";
import {
  Button,
  LogViewer,
  PageHeading,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { useLogStream } from "../useLogStream.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

const LINE_OPTIONS = [50, 200, 500, 1000];

/**
 * InstanceLogsPage — live SSE tail of `.stack-pids/<instance>.log`.
 *
 * Reads the admin's own bookkeeping log (written by `make
 * start-component` redirecting stdout/stderr). Components remain
 * decoupled — there is no log endpoint on the component side.
 *
 * Controls: pause/resume the stream, clear the buffer, change the seed
 * buffer size. Connection status reflects the live EventSource state.
 */
export function InstanceLogsPage() {
  const { project, instance } = useParams<{
    project: string;
    instance: string;
  }>();
  const [paused, setPaused] = useState(false);
  const [seed, setSeed] = useState(200);

  if (!project || !instance) {
    return <ErrorPanel message="missing project/instance in route" />;
  }

  const { lines, connected, error, clear } = useLogStream(project, instance, {
    paused,
    lines: seed,
  });

  // StatusBadge has known tokens; map our 3-state status to them.
  const status = paused ? "stopped" : connected ? "active" : "starting";

  return (
    <div className="flex flex-col gap-4">
      <PageHeading
        kicker={`admin · topology · ${project} · ${instance}`}
        title={<>Logs</>}
        meta={
          <span className="inline-flex items-center gap-3">
            <Link
              to="/ui/topology"
              style={{ color: "var(--color-accent)", textDecoration: "none" }}
            >
              ← topology
            </Link>
            <Link
              to={`/ui/topology/${encodeURIComponent(project)}/console`}
              style={{ color: "var(--color-accent)", textDecoration: "none" }}
            >
              console →
            </Link>
            <span>{lines.length} buffered</span>
          </span>
        }
      />

      <div
        className="flex flex-wrap items-center gap-2"
        style={{
          background: "var(--color-bg-subtle)",
          border: "1px solid var(--color-border)",
          borderRadius: "var(--radius-sm)",
          padding: "0.5rem 0.75rem",
        }}
      >
        <StatusBadge status={status} />
        <span
          className="font-mono"
          style={{ color: "var(--color-fg-muted)", fontSize: "0.78rem" }}
        >
          /api/v1/topology/projects/{project}/instances/{instance}/logs
        </span>

        <span className="ml-auto inline-flex items-center gap-2">
          <SeedSelect value={seed} onChange={setSeed} />
          <Button
            variant={paused ? "primary" : "secondary"}
            size="sm"
            onClick={() => setPaused((p) => !p)}
          >
            {paused ? "resume" : "pause"}
          </Button>
          <Button variant="ghost" size="sm" onClick={clear}>
            clear
          </Button>
        </span>
      </div>

      {error && <ErrorPanel message={error} />}

      <LogViewer lines={lines} maxHeight="60vh" />
    </div>
  );
}

function SeedSelect({
  value,
  onChange,
}: {
  value: number;
  onChange: (v: number) => void;
}) {
  return (
    <label
      className="inline-flex items-center gap-1 font-mono uppercase tracking-[0.16em]"
      style={{ color: "var(--color-fg-subtle)", fontSize: "0.62rem" }}
    >
      seed
      <select
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="font-mono"
        style={{
          background: "var(--color-surface)",
          color: "var(--color-fg)",
          border: "1px solid var(--color-border)",
          borderRadius: "var(--radius-xs)",
          padding: "0.15rem 0.35rem",
          fontSize: "0.7rem",
        }}
      >
        {LINE_OPTIONS.map((n) => (
          <option key={n} value={n}>
            {n}
          </option>
        ))}
      </select>
    </label>
  );
}
