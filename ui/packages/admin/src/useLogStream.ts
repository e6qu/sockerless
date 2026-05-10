import { useEffect, useRef, useState } from "react";

export interface LogStreamOptions {
  /** Last N lines to seed the stream with; default 200. */
  lines?: number;
  /** When true, do not open a connection. Toggle to pause/resume. */
  paused?: boolean;
  /** Hard cap on retained lines in the returned buffer; default 5000. */
  maxBuffered?: number;
}

export interface LogStreamHandle {
  lines: string[];
  connected: boolean;
  error: string | null;
  clear: () => void;
}

/**
 * useLogStream subscribes to admin's SSE log endpoint for a topology
 * instance. Returns the rolling buffer + connection state. Caller
 * controls pause / clear; the hook does not auto-reconnect on `error`
 * — EventSource itself reconnects with backoff, and the Connected flag
 * flips back when it does.
 */
export function useLogStream(
  project: string,
  instance: string,
  opts: LogStreamOptions = {},
): LogStreamHandle {
  const { lines = 200, paused = false, maxBuffered = 5000 } = opts;
  const [buffered, setBuffered] = useState<string[]>([]);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const sourceRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (paused) {
      setConnected(false);
      return;
    }
    const url = new URL(
      `/api/v1/topology/projects/${encodeURIComponent(project)}/instances/${encodeURIComponent(instance)}/logs`,
      window.location.origin,
    );
    url.searchParams.set("follow", "1");
    url.searchParams.set("lines", String(lines));

    const es = new EventSource(url.toString());
    sourceRef.current = es;

    es.onopen = () => {
      setConnected(true);
      setError(null);
    };
    es.onmessage = (ev) => {
      const line = ev.data;
      setBuffered((prev) => {
        const next = [...prev, line];
        if (next.length > maxBuffered) {
          return next.slice(next.length - maxBuffered);
        }
        return next;
      });
    };
    es.addEventListener("error", (ev) => {
      const data = (ev as MessageEvent).data;
      if (typeof data === "string" && data.length > 0) {
        setError(data);
      }
    });
    es.onerror = () => {
      setConnected(false);
    };

    return () => {
      es.close();
      sourceRef.current = null;
      setConnected(false);
    };
  }, [project, instance, paused, lines, maxBuffered]);

  return {
    lines: buffered,
    connected,
    error,
    clear: () => setBuffered([]),
  };
}
