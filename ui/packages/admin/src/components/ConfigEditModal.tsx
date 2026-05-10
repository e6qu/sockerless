import { useEffect, useMemo, useState } from "react";
import {
  Button,
  Modal,
  StatusBadge,
  useReportError,
} from "@sockerless/ui-core/components";
import {
  AdminApiClient,
  type ConfigKeyMeta,
  type ConfigUpdateResponse,
  type TopologyInstance,
} from "../api.js";

const api = new AdminApiClient();

export interface ConfigEditModalProps {
  open: boolean;
  onClose: () => void;
  project: string;
  instance: TopologyInstance;
  metadata: ConfigKeyMeta[];
  /** Called after the operator picks Reload — fires the reload API. */
  onReload: (project: string, name: string) => Promise<void>;
  /** Called after the operator picks Restart — fires stop + start. */
  onRestart: (project: string, name: string) => Promise<void>;
}

interface Row {
  key: string;
  value: string;
}

function rowsFromInstance(inst: TopologyInstance): Row[] {
  const cfg = inst.config ?? {};
  const keys = Object.keys(cfg);
  if (keys.length === 0) return [{ key: "", value: "" }];
  return keys.map((k) => ({ key: k, value: cfg[k]! }));
}

/**
 * ConfigEditModal — config-only edit flow.
 *
 * Each row shows a `hot` or `restart` indicator pulled from the
 * curated `metadata` table. Save → PUT `/api/v1/topology/.../config`
 * which classifies actual changes server-side and tells the UI which
 * action to offer next (Reload for hot-only, Restart otherwise).
 *
 * We don't reuse InstanceForm because its full-instance edit flow has
 * different invariants (name/kind locked, port/cloud editable, backend
 * dependencies). Config edit is conceptually separate — only the
 * Config map mutates.
 */
export function ConfigEditModal({
  open,
  onClose,
  project,
  instance,
  metadata,
  onReload,
  onRestart,
}: ConfigEditModalProps) {
  const reportError = useReportError();
  const [rows, setRows] = useState<Row[]>(() => rowsFromInstance(instance));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [postSave, setPostSave] = useState<ConfigUpdateResponse | null>(null);
  const [actionPending, setActionPending] = useState(false);

  // Reset on (re)open or when the underlying instance changes.
  useEffect(() => {
    if (open) {
      setRows(rowsFromInstance(instance));
      setError(null);
      setPostSave(null);
    }
  }, [open, instance]);

  const lookup = useMemo(() => {
    const m = new Map<string, ConfigKeyMeta>();
    metadata.forEach((k) => m.set(k.name, k));
    return m;
  }, [metadata]);

  const annotateKey = (name: string): ConfigKeyMeta => {
    return lookup.get(name) ?? { name, hot_reloadable: false };
  };

  const updateRow = (i: number, field: "key" | "value", value: string) => {
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, [field]: value } : r)));
  };
  const addRow = () => setRows((prev) => [...prev, { key: "", value: "" }]);
  const removeRow = (i: number) =>
    setRows((prev) => prev.filter((_, idx) => idx !== i));

  const submit = async () => {
    const config: Record<string, string> = {};
    for (const r of rows) {
      const k = r.key.trim();
      if (!k) continue;
      config[k] = r.value;
    }
    setSaving(true);
    setError(null);
    try {
      const resp = await api.topologyInstanceConfig(project, instance.name, config);
      setPostSave(resp);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(msg);
      reportError(err, "Config update failed");
    } finally {
      setSaving(false);
    }
  };

  const reload = async () => {
    setActionPending(true);
    try {
      await onReload(project, instance.name);
      onClose();
    } catch (err) {
      reportError(err, "Reload failed");
    } finally {
      setActionPending(false);
    }
  };

  const restart = async () => {
    setActionPending(true);
    try {
      await onRestart(project, instance.name);
      onClose();
    } catch (err) {
      reportError(err, "Restart failed");
    } finally {
      setActionPending(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      kicker={`config · ${project} · ${instance.name}`}
      title="Edit instance config"
      size="md"
      footer={
        postSave ? (
          <PostSaveActions
            response={postSave}
            onClose={onClose}
            onReload={reload}
            onRestart={restart}
            pending={actionPending}
          />
        ) : (
          <>
            <Button variant="ghost" size="sm" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" size="sm" onClick={submit} disabled={saving}>
              {saving ? "Saving…" : "Save"}
            </Button>
          </>
        )
      }
    >
      {postSave ? (
        <PostSaveSummary response={postSave} />
      ) : (
        <div className="flex flex-col gap-3">
          <p
            className="font-mono"
            style={{ color: "var(--color-fg-muted)", fontSize: "0.78rem" }}
          >
            Each row shows whether the key is hot-reloadable (
            <ReloadabilityBadge hot />) or restart-required (
            <ReloadabilityBadge hot={false} />). Unknown keys default to
            restart-required.
          </p>
          {error && (
            <div
              role="alert"
              className="font-mono"
              style={{
                background: "var(--color-status-error-soft)",
                border: "1px solid var(--color-status-error)",
                borderRadius: "var(--radius-xs)",
                padding: "0.4rem 0.6rem",
                fontSize: "0.78rem",
              }}
            >
              {error}
            </div>
          )}
          <div className="flex flex-col gap-2">
            {rows.map((r, i) => {
              const meta = r.key.trim() ? annotateKey(r.key.trim()) : null;
              return (
                <div key={i} className="flex items-center gap-2">
                  <input
                    type="text"
                    placeholder="KEY"
                    value={r.key}
                    onChange={(e) => updateRow(i, "key", e.target.value)}
                    className="font-mono"
                    style={{
                      flex: "1 1 12rem",
                      padding: "0.35rem 0.5rem",
                      background: "var(--color-bg)",
                      color: "var(--color-fg)",
                      border: "1px solid var(--color-border)",
                      borderRadius: "var(--radius-xs)",
                      fontSize: "0.78rem",
                    }}
                  />
                  <input
                    type="text"
                    placeholder="value"
                    value={r.value}
                    onChange={(e) => updateRow(i, "value", e.target.value)}
                    className="font-mono"
                    style={{
                      flex: "2 1 16rem",
                      padding: "0.35rem 0.5rem",
                      background: "var(--color-bg)",
                      color: "var(--color-fg)",
                      border: "1px solid var(--color-border)",
                      borderRadius: "var(--radius-xs)",
                      fontSize: "0.78rem",
                    }}
                  />
                  {meta && <ReloadabilityBadge hot={meta.hot_reloadable} />}
                  <Button variant="ghost" size="sm" onClick={() => removeRow(i)}>
                    ✕
                  </Button>
                </div>
              );
            })}
          </div>
          <div>
            <Button variant="ghost" size="sm" onClick={addRow}>
              + row
            </Button>
          </div>
        </div>
      )}
    </Modal>
  );
}

function ReloadabilityBadge({ hot }: { hot: boolean }) {
  return <StatusBadge status={hot ? "ok" : "warning"} />;
}

function PostSaveSummary({ response }: { response: ConfigUpdateResponse }) {
  const { hot_reloadable_changes, restart_required_changes } = response;
  const noop = hot_reloadable_changes.length === 0 && restart_required_changes.length === 0;

  return (
    <div className="flex flex-col gap-3 font-mono" style={{ fontSize: "0.82rem" }}>
      <div
        className="font-display"
        style={{
          fontStyle: "italic",
          fontWeight: 600,
          fontSize: "1rem",
          letterSpacing: "-0.02em",
        }}
      >
        Config saved
      </div>
      {noop && (
        <p style={{ color: "var(--color-fg-muted)" }}>
          No changes detected. Nothing to apply.
        </p>
      )}
      {hot_reloadable_changes.length > 0 && (
        <div>
          <div
            className="uppercase tracking-[0.18em]"
            style={{ color: "var(--color-status-ok)", fontSize: "0.62rem" }}
          >
            hot-reloadable changes
          </div>
          <ul style={{ marginTop: "0.25rem" }}>
            {hot_reloadable_changes.map((k) => (
              <li key={k}>{k}</li>
            ))}
          </ul>
        </div>
      )}
      {restart_required_changes.length > 0 && (
        <div>
          <div
            className="uppercase tracking-[0.18em]"
            style={{ color: "var(--color-status-warn)", fontSize: "0.62rem" }}
          >
            restart-required changes
          </div>
          <ul style={{ marginTop: "0.25rem" }}>
            {restart_required_changes.map((k) => (
              <li key={k}>{k}</li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function PostSaveActions({
  response,
  onClose,
  onReload,
  onRestart,
  pending,
}: {
  response: ConfigUpdateResponse;
  onClose: () => void;
  onReload: () => void;
  onRestart: () => void;
  pending: boolean;
}) {
  const { hot_reloadable_changes, restart_required_changes } = response;
  const noop = hot_reloadable_changes.length === 0 && restart_required_changes.length === 0;
  if (noop) {
    return (
      <Button variant="primary" size="sm" onClick={onClose}>
        Close
      </Button>
    );
  }
  // Restart trumps reload — if any restart-required key changed, reload alone
  // wouldn't pick the change up. The UI offers both for the operator to
  // confirm which they want, but restart is the recommended primary.
  if (restart_required_changes.length > 0) {
    return (
      <>
        <Button variant="ghost" size="sm" onClick={onClose} disabled={pending}>
          Close
        </Button>
        <Button variant="secondary" size="sm" onClick={onReload} disabled={pending}>
          Reload (partial)
        </Button>
        <Button variant="primary" size="sm" onClick={onRestart} disabled={pending}>
          {pending ? "…" : "Restart"}
        </Button>
      </>
    );
  }
  return (
    <>
      <Button variant="ghost" size="sm" onClick={onClose} disabled={pending}>
        Close
      </Button>
      <Button variant="primary" size="sm" onClick={onReload} disabled={pending}>
        {pending ? "…" : "Reload"}
      </Button>
    </>
  );
}
