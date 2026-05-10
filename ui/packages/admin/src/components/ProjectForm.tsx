import { useEffect, useState, type CSSProperties } from "react";
import { Button, Modal, Spinner } from "@sockerless/ui-core/components";
import type { TopologyProject } from "../api.js";

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

export interface ProjectFormProps {
  open: boolean;
  onClose: () => void;
  onSubmit: (p: TopologyProject) => Promise<void>;
}

/**
 * ProjectForm — minimal modal for creating a new (empty) project.
 *
 * Project name is the only field admin requires up-front; instances are
 * added separately via InstanceForm. Cloud + backend on the project
 * itself are legacy (only used by the old per-project tuple shape) and
 * are intentionally not exposed here — operators set them on individual
 * Instance entries instead.
 */
export function ProjectForm({ open, onClose, onSubmit }: ProjectFormProps) {
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setName("");
      setError(null);
    }
  }, [open]);

  const submit = async (event: { preventDefault: () => void }) => {
    event.preventDefault();
    setError(null);
    const trimmed = name.trim();
    if (!trimmed) {
      setError("Name is required.");
      return;
    }
    setSubmitting(true);
    try {
      await onSubmit({ name: trimmed });
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      kicker="topology"
      title="add project"
      size="sm"
      footer={
        <>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={() => {
              const formEl = document.getElementById(
                "project-form",
              ) as HTMLFormElement | null;
              formEl?.requestSubmit();
            }}
            disabled={submitting}
          >
            {submitting ? <Spinner label="" /> : "Create"}
          </Button>
        </>
      }
    >
      <form id="project-form" onSubmit={submit} className="flex flex-col gap-3">
        <label>
          <span
            style={{
              display: "block",
              marginBottom: 4,
              fontSize: "0.68rem",
              letterSpacing: "0.18em",
              textTransform: "uppercase",
              color: "var(--color-fg-subtle)",
            }}
          >
            project name
          </span>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="my-project"
            required
            autoFocus
            style={inputStyle}
          />
        </label>
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
