import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import { createRepo, fetchGitignoreTemplates, fetchLicenseTemplates } from "../api.js";
import { Button, Modal } from "./ui.js";

interface RepoCreateDialogProps {
  open: boolean;
  onClose: () => void;
  onCreated: () => void;
}

export function RepoCreateDialog({ open, onClose, onCreated }: RepoCreateDialogProps) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [visibility, setVisibility] = useState<"public" | "private">("public");
  const [autoInit, setAutoInit] = useState(true);
  const [gitignore, setGitignore] = useState("");
  const [license, setLicense] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const gitignoresQ = useQuery({
    queryKey: ["gitignore-templates"],
    queryFn: fetchGitignoreTemplates,
    enabled: open,
  });
  const licensesQ = useQuery({
    queryKey: ["license-templates"],
    queryFn: fetchLicenseTemplates,
    enabled: open,
  });

  const reset = () => {
    setName("");
    setDescription("");
    setVisibility("public");
    setAutoInit(true);
    setGitignore("");
    setLicense("");
    setError(null);
  };

  const handleClose = () => {
    reset();
    onClose();
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await createRepo({
        name: name.trim(),
        description: description.trim(),
        visibility,
        auto_init: autoInit,
        gitignore_template: gitignore || undefined,
        license_template: license || undefined,
      });
      reset();
      onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  if (!open) return null;

  return (
    <Modal onClose={handleClose} title="Create a new repository">
      <form onSubmit={handleSubmit} style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
        <label style={{ display: "flex", flexDirection: "column", gap: "0.25rem" }}>
          <span style={{ fontSize: "0.85rem", fontWeight: 500 }}>Repository name *</span>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="name"
            required
            disabled={busy}
            style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
          />
        </label>

        <label style={{ display: "flex", flexDirection: "column", gap: "0.25rem" }}>
          <span style={{ fontSize: "0.85rem", fontWeight: 500 }}>Description</span>
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Short description of this repository"
            disabled={busy}
            style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
          />
        </label>

        <fieldset style={{ border: "none", padding: 0, margin: 0, display: "flex", gap: "1rem" }}>
          <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
            <input
              type="radio"
              name="visibility"
              value="public"
              checked={visibility === "public"}
              onChange={() => setVisibility("public")}
              disabled={busy}
            />
            Public
          </label>
          <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
            <input
              type="radio"
              name="visibility"
              value="private"
              checked={visibility === "private"}
              onChange={() => setVisibility("private")}
              disabled={busy}
            />
            Private
          </label>
        </fieldset>

        <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
          <input
            type="checkbox"
            checked={autoInit}
            onChange={(e) => setAutoInit(e.target.checked)}
            disabled={busy}
          />
          Initialize this repository with a README
        </label>

        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "0.75rem" }}>
          <label style={{ display: "flex", flexDirection: "column", gap: "0.25rem" }}>
            <span style={{ fontSize: "0.85rem", fontWeight: 500 }}>.gitignore template</span>
            {gitignoresQ.isLoading && <Spinner label="loading" />}
            {gitignoresQ.isError && (
              <InlineError inline title="Failed to load templates" detail={String(gitignoresQ.error)} />
            )}
            {gitignoresQ.data && (
              <select
                value={gitignore}
                onChange={(e) => setGitignore(e.target.value)}
                disabled={busy}
                style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
              >
                <option value="">None</option>
                {gitignoresQ.data.map((t) => (
                  <option key={t} value={t}>
                    {t}
                  </option>
                ))}
              </select>
            )}
          </label>

          <label style={{ display: "flex", flexDirection: "column", gap: "0.25rem" }}>
            <span style={{ fontSize: "0.85rem", fontWeight: 500 }}>License</span>
            {licensesQ.isLoading && <Spinner label="loading" />}
            {licensesQ.isError && (
              <InlineError inline title="Failed to load licenses" detail={String(licensesQ.error)} />
            )}
            {licensesQ.data && (
              <select
                value={license}
                onChange={(e) => setLicense(e.target.value)}
                disabled={busy}
                style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
              >
                <option value="">None</option>
                {licensesQ.data.map((l) => (
                  <option key={l.key} value={l.key}>
                    {l.name}
                  </option>
                ))}
              </select>
            )}
          </label>
        </div>

        {error && (
          <div style={{ fontSize: "0.85rem", color: "var(--color-danger-fg)", background: "var(--color-danger-soft)", padding: "0.5rem", borderRadius: "var(--radius-md)" }}>
            {error}
          </div>
        )}

        <div style={{ display: "flex", justifyContent: "flex-end", gap: "0.5rem", marginTop: "0.5rem" }}>
          <Button type="button" variant="secondary" onClick={handleClose} disabled={busy}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
            Create repository
          </Button>
        </div>
      </form>
    </Modal>
  );
}
