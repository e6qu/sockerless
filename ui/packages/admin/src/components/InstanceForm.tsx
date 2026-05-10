import {
  useEffect,
  useMemo,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import {
  Button,
  Modal,
  Spinner,
  useReportError,
} from "@sockerless/ui-core/components";
import {
  type BackendType,
  type CloudType,
  type InstanceKind,
  type TopologyInstance,
  type TopologyProject,
} from "../api.js";

const cloudOptions: CloudType[] = ["aws", "gcp", "azure"];

const backendsByCloud: Record<CloudType, BackendType[]> = {
  aws: ["ecs", "lambda"],
  gcp: ["cloudrun", "gcf"],
  azure: ["aca", "azf"],
};

const kindOptions: { value: InstanceKind; label: string }[] = [
  { value: "sim", label: "sim — cloud simulator" },
  { value: "backend", label: "backend — sockerless backend" },
  { value: "bleephub", label: "bleephub — github simulator" },
];

const labelStyle: CSSProperties = {
  display: "block",
  marginBottom: 4,
  fontSize: "0.68rem",
  letterSpacing: "0.18em",
  textTransform: "uppercase",
  color: "var(--color-fg-subtle)",
};

const inputStyle: CSSProperties = {
  width: "100%",
  padding: "0.4rem 0.55rem",
  background: "var(--color-bg)",
  color: "var(--color-fg)",
  border: "1px solid var(--color-border-strong)",
  borderRadius: "var(--radius-sm)",
  fontFamily: "var(--font-mono, ui-monospace, monospace)",
  fontSize: "0.85rem",
};

export interface InstanceFormProps {
  open: boolean;
  onClose: () => void;
  /** Project the instance belongs to. Required so the form can list sibling sims for backend → sim ref. */
  project: TopologyProject;
  /** When set, the form opens in edit mode and seeds from this instance. Name + kind become read-only. */
  editing?: TopologyInstance;
  /** Allocate a port from the configured pool for the selected kind. */
  onAllocatePort: (kind: InstanceKind) => Promise<number>;
  /** Submit the new / updated instance. */
  onSubmit: (inst: TopologyInstance) => Promise<void>;
}

interface FormState {
  name: string;
  kind: InstanceKind;
  cloud: CloudType | "";
  backend: BackendType | "";
  port: string;
  sim: string;
  configRows: { key: string; value: string }[];
}

function emptyForm(): FormState {
  return {
    name: "",
    kind: "sim",
    cloud: "",
    backend: "",
    port: "",
    sim: "",
    configRows: [{ key: "", value: "" }],
  };
}

function fromInstance(inst: TopologyInstance): FormState {
  return {
    name: inst.name,
    kind: inst.kind,
    cloud: inst.cloud ?? "",
    backend: inst.backend ?? "",
    port: String(inst.port),
    sim: inst.sim ?? "",
    configRows: Object.keys(inst.config ?? {}).length
      ? Object.entries(inst.config!).map(([key, value]) => ({ key, value }))
      : [{ key: "", value: "" }],
  };
}

/**
 * InstanceForm — modal-shaped add/edit form for a single Instance.
 *
 * Per-kind fields:
 *   - sim:      cloud + port
 *   - backend:  cloud + backend + port + (optional) sim ref
 *   - bleephub: port only
 *
 * The form does not enforce server-side validation rules — the admin
 * REST surface returns a 400 with a readable message that the caller
 * surfaces via the error path.
 */
export function InstanceForm({
  open,
  onClose,
  project,
  editing,
  onAllocatePort,
  onSubmit,
}: InstanceFormProps) {
  const reportError = useReportError();
  const [form, setForm] = useState<FormState>(() =>
    editing ? fromInstance(editing) : emptyForm(),
  );
  const [submitting, setSubmitting] = useState(false);
  const [allocating, setAllocating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset the form when it opens with a new editing target.
  useEffect(() => {
    if (open) {
      setForm(editing ? fromInstance(editing) : emptyForm());
      setError(null);
    }
  }, [open, editing]);

  const sims = useMemo(
    () => (project.instances ?? []).filter((i) => i.kind === "sim"),
    [project],
  );

  const updateField = <K extends keyof FormState>(
    key: K,
    value: FormState[K],
  ) => {
    setForm((prev) => {
      const next = { ...prev, [key]: value };
      // Clear backend when cloud changes since the backend list is
      // cloud-scoped.
      if (key === "cloud" && prev.cloud !== value) {
        next.backend = "";
      }
      // Clear cloud + backend + sim when kind changes to bleephub.
      if (key === "kind") {
        if (value === "bleephub") {
          next.cloud = "";
          next.backend = "";
          next.sim = "";
        } else if (value === "sim") {
          next.backend = "";
          next.sim = "";
        }
      }
      return next;
    });
  };

  const addConfigRow = () => {
    setForm((prev) => ({
      ...prev,
      configRows: [...prev.configRows, { key: "", value: "" }],
    }));
  };

  const removeConfigRow = (index: number) => {
    setForm((prev) => ({
      ...prev,
      configRows: prev.configRows.filter((_, i) => i !== index),
    }));
  };

  const updateConfigRow = (
    index: number,
    field: "key" | "value",
    value: string,
  ) => {
    setForm((prev) => ({
      ...prev,
      configRows: prev.configRows.map((row, i) =>
        i === index ? { ...row, [field]: value } : row,
      ),
    }));
  };

  const allocate = async () => {
    setAllocating(true);
    setError(null);
    try {
      const port = await onAllocatePort(form.kind);
      setForm((prev) => ({ ...prev, port: String(port) }));
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(`Auto-allocate failed: ${msg}`);
      reportError(err, "Auto-allocate failed");
    } finally {
      setAllocating(false);
    }
  };

  const submit = async (event: { preventDefault: () => void }) => {
    event.preventDefault();
    setError(null);
    const portNum = Number.parseInt(form.port, 10);
    if (Number.isNaN(portNum) || portNum <= 0) {
      setError("Port must be a positive integer.");
      return;
    }
    const config: Record<string, string> = {};
    for (const row of form.configRows) {
      const key = row.key.trim();
      if (!key) continue;
      config[key] = row.value;
    }
    const inst: TopologyInstance = {
      name: form.name.trim(),
      kind: form.kind,
      port: portNum,
      cloud: form.cloud || undefined,
      backend: form.backend || undefined,
      sim: form.sim || undefined,
      config: Object.keys(config).length ? config : undefined,
    };
    setSubmitting(true);
    try {
      await onSubmit(inst);
      onClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(msg);
    } finally {
      setSubmitting(false);
    }
  };

  const cloudFieldsVisible =
    form.kind === "sim" || form.kind === "backend";
  const backendFieldVisible = form.kind === "backend";
  const simFieldVisible = form.kind === "backend";
  const isEdit = !!editing;

  return (
    <Modal
      open={open}
      onClose={onClose}
      kicker={`project · ${project.name}`}
      title={isEdit ? `edit ${editing!.name}` : "add instance"}
      size="md"
      footer={
        <FormFooter
          onCancel={onClose}
          onSubmit={() => {
            const formEl = document.getElementById(
              "instance-form",
            ) as HTMLFormElement | null;
            formEl?.requestSubmit();
          }}
          submitting={submitting}
          submitLabel={isEdit ? "Save" : "Add"}
        />
      }
    >
      <form id="instance-form" onSubmit={submit} className="flex flex-col gap-4">
        <Field label="name">
          <input
            type="text"
            value={form.name}
            onChange={(e) => updateField("name", e.target.value)}
            disabled={isEdit}
            required
            placeholder="ecs-prod-sim"
            style={inputStyle}
          />
        </Field>

        <Field label="kind">
          <select
            value={form.kind}
            onChange={(e) =>
              updateField("kind", e.target.value as InstanceKind)
            }
            disabled={isEdit}
            style={inputStyle}
          >
            {kindOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </Field>

        {cloudFieldsVisible && (
          <Field label="cloud">
            <select
              value={form.cloud}
              onChange={(e) =>
                updateField("cloud", e.target.value as CloudType | "")
              }
              required
              style={inputStyle}
            >
              <option value="">— select —</option>
              {cloudOptions.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </Field>
        )}

        {backendFieldVisible && form.cloud && (
          <Field label="backend">
            <select
              value={form.backend}
              onChange={(e) =>
                updateField("backend", e.target.value as BackendType | "")
              }
              required
              style={inputStyle}
            >
              <option value="">— select —</option>
              {backendsByCloud[form.cloud as CloudType].map((b) => (
                <option key={b} value={b}>
                  {b}
                </option>
              ))}
            </select>
          </Field>
        )}

        {simFieldVisible && (
          <Field label="sim ref (optional — same project)">
            <select
              value={form.sim}
              onChange={(e) => updateField("sim", e.target.value)}
              style={inputStyle}
            >
              <option value="">— none —</option>
              {sims.map((s) => (
                <option key={s.name} value={s.name}>
                  {s.name} (:{s.port})
                </option>
              ))}
            </select>
          </Field>
        )}

        <Field label="port">
          <div className="flex gap-2">
            <input
              type="number"
              min={1}
              max={65535}
              value={form.port}
              onChange={(e) => updateField("port", e.target.value)}
              required
              placeholder="3300"
              style={{ ...inputStyle, flex: 1 }}
            />
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={allocate}
              disabled={allocating}
            >
              {allocating ? "…" : "auto-allocate"}
            </Button>
          </div>
        </Field>

        <Field label={`env config (${form.configRows.length} ${form.configRows.length === 1 ? "row" : "rows"})`}>
          <div className="flex flex-col gap-2">
            {form.configRows.map((row, i) => (
              <div key={i} className="flex gap-2">
                <input
                  type="text"
                  placeholder="KEY"
                  value={row.key}
                  onChange={(e) => updateConfigRow(i, "key", e.target.value)}
                  style={{ ...inputStyle, flex: 1 }}
                />
                <input
                  type="text"
                  placeholder="value"
                  value={row.value}
                  onChange={(e) =>
                    updateConfigRow(i, "value", e.target.value)
                  }
                  style={{ ...inputStyle, flex: 2 }}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => removeConfigRow(i)}
                  aria-label={`remove config row ${i + 1}`}
                >
                  ×
                </Button>
              </div>
            ))}
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={addConfigRow}
              style={{ alignSelf: "flex-start" }}
            >
              + add row
            </Button>
          </div>
        </Field>

        {error && (
          <div
            role="alert"
            className="px-3 py-2 font-mono"
            style={{
              background: "var(--color-status-error-soft)",
              color: "var(--color-status-error)",
              border: "1px solid var(--color-status-error)",
              borderRadius: "var(--radius-sm)",
              fontSize: "0.78rem",
            }}
          >
            {error}
          </div>
        )}
      </form>
    </Modal>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <label>
      <span style={labelStyle}>{label}</span>
      {children}
    </label>
  );
}

function FormFooter({
  onCancel,
  onSubmit,
  submitting,
  submitLabel,
}: {
  onCancel: () => void;
  onSubmit: () => void;
  submitting: boolean;
  submitLabel: string;
}) {
  return (
    <>
      <Button variant="ghost" size="sm" onClick={onCancel}>
        Cancel
      </Button>
      <Button
        variant="primary"
        size="sm"
        onClick={onSubmit}
        disabled={submitting}
      >
        {submitting ? <Spinner label="" /> : submitLabel}
      </Button>
    </>
  );
}
