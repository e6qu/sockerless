import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import {
  createGist,
  deleteGist,
  fetchGist,
  fetchGists,
  updateGist,
} from "../api.js";
import type { BleephubGist, BleephubGistFile } from "../types.js";
import {
  Box,
  Button,
  CodeBlock,
  DialogActions,
  ErrorBanner,
  FormLabel,
  Modal,
  PageTitle,
  StateLabel,
} from "../components/ui.js";

const col = createColumnHelper<BleephubGist>();

export function GistsPage() {
  const [showCreate, setShowCreate] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  return (
    <div>
      <PageTitle
        title="Gists"
        meta="Authenticated user's snippets."
        actions={
          <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
            New gist
          </Button>
        }
      />
      <GistsTable onSelect={(id) => setSelectedId(id)} />
      {showCreate && <CreateGistDialog onClose={() => setShowCreate(false)} />}
      {selectedId && <GistDetail id={selectedId} onClose={() => setSelectedId(null)} />}
    </div>
  );
}

function GistsTable({ onSelect }: { onSelect: (id: string) => void }) {
  const queryClient = useQueryClient();
  const [mutationError, setMutationError] = useState<string | null>(null);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["gists"],
    queryFn: fetchGists,
    refetchInterval: 5000,
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteGist(id),
    onSuccess: () => {
      setMutationError(null);
      queryClient.invalidateQueries({ queryKey: ["gists"] });
    },
    onError: (err: Error) => setMutationError(err.message),
  });

  if (isError) return <InlineError title="Failed to load gists" />;
  if (isLoading || !data) return <Spinner label="loading gists" />;

  const columns = [
    col.accessor("id", {
      header: "ID",
      cell: (info) => (
        <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
          {info.getValue()}
        </span>
      ),
    }),
    col.accessor("description", {
      header: "Description",
      cell: (info) => (
        <Button variant="ghost" size="sm" onClick={() => onSelect(info.row.original.id)}>
          {info.getValue() || "(no description)"}
        </Button>
      ),
    }),
    col.accessor("public", {
      header: "Visibility",
      cell: (info) =>
        info.getValue() ? (
          <StateLabel state="open">public</StateLabel>
        ) : (
          <StateLabel state="closed">secret</StateLabel>
        ),
    }),
    col.accessor("files", {
      header: "Files",
      cell: (info) => (
        <span style={{ color: "var(--color-fg-muted)" }}>{Object.keys(info.getValue()).length}</span>
      ),
    }),
    col.accessor("created_at", {
      header: "Created",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
    col.accessor("updated_at", {
      header: "Updated",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
    col.display({
      id: "actions",
      header: "Actions",
      cell: (info) => {
        const gist = info.row.original;
        return (
          <Button
            size="sm"
            variant="danger"
            onClick={() => {
              if (confirm(`Delete gist ${gist.id}?`)) {
                deleteMut.mutate(gist.id);
              }
            }}
            disabled={deleteMut.isPending}
          >
            delete
          </Button>
        );
      },
    }),
  ];

  return (
    <>
      {mutationError && <ErrorBanner>{mutationError}</ErrorBanner>}
      <DataTable
        data={data}
        columns={columns}
        filterPlaceholder="Filter gists…"
        emptyMessage="No gists yet."
      />
    </>
  );
}

function GistDetail({ id, onClose }: { id: string; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<BleephubGist | null>(null);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["gists", id],
    queryFn: () => fetchGist(id),
  });

  if (isError) return <InlineError title="Failed to load gist" />;
  if (isLoading || !data) return <Spinner label="loading gist" />;

  const files = Object.entries(data.files);

  return (
    <Modal title={data.description || `Gist ${data.id}`} onClose={onClose}>
      <div className="mb-4 flex items-center gap-2">
        {data.public ? (
          <StateLabel state="open">public</StateLabel>
        ) : (
          <StateLabel state="closed">secret</StateLabel>
        )}
        <span style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
          {files.length} file{files.length === 1 ? "" : "s"}
        </span>
      </div>

      {files.map(([filename, file]) => (
        <Box key={filename} header={filename} className="mb-4">
          {file.content != null ? (
            <CodeBlock>{file.content}</CodeBlock>
          ) : (
            <div
              style={{
                padding: "1rem",
                color: "var(--color-fg-muted)",
                fontSize: "0.85rem",
              }}
            >
              Content unavailable
            </div>
          )}
        </Box>
      ))}

      <DialogActions>
        <Button onClick={onClose} variant="ghost">
          Close
        </Button>
        <Button onClick={() => setEditing(data)} variant="secondary">
          Edit
        </Button>
      </DialogActions>

      {editing && (
        <EditGistDialog
          gist={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            queryClient.invalidateQueries({ queryKey: ["gists", id] });
            queryClient.invalidateQueries({ queryKey: ["gists"] });
            setEditing(null);
          }}
        />
      )}
    </Modal>
  );
}

function CreateGistDialog({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [description, setDescription] = useState("");
  const [isPublic, setIsPublic] = useState(false);
  const [files, setFiles] = useState<{ filename: string; content: string }[]>([
    { filename: "", content: "" },
  ]);
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      const fileMap: Record<string, { content: string }> = {};
      files.forEach((f) => {
        if (f.filename.trim()) fileMap[f.filename.trim()] = { content: f.content };
      });
      return createGist({
        description,
        public: isPublic,
        files: fileMap,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gists"] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  const updateFile = (idx: number, patch: Partial<{ filename: string; content: string }>) => {
    setFiles((cur) => cur.map((f, i) => (i === idx ? { ...f, ...patch } : f)));
  };

  const valid = files.some((f) => f.filename.trim());

  return (
    <Modal title="Create gist" onClose={onClose}>
      <FormLabel id="gist-desc">Description</FormLabel>
      <input
        id="gist-desc"
        type="text"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="mb-4 w-full"
      />

      <label className="mb-4 inline-flex items-center gap-2">
        <input
          type="checkbox"
          checked={isPublic}
          onChange={(e) => setIsPublic(e.target.checked)}
        />
        <span style={{ fontSize: "0.82rem" }}>Public gist</span>
      </label>

      <FormLabel>Files</FormLabel>
      {files.map((file, idx) => (
        <div key={idx} className="mb-3 rounded border p-3" style={{ borderColor: "var(--color-border)" }}>
          <input
            type="text"
            value={file.filename}
            onChange={(e) => updateFile(idx, { filename: e.target.value })}
            placeholder="filename.ext"
            className="mb-2 w-full"
          />
          <textarea
            value={file.content}
            onChange={(e) => updateFile(idx, { content: e.target.value })}
            rows={4}
            placeholder="file content"
            className="w-full"
            style={{ resize: "vertical" }}
          />
          <div className="mt-2 flex justify-end">
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setFiles((cur) => cur.filter((_, i) => i !== idx))}
              disabled={files.length === 1}
            >
              remove file
            </Button>
          </div>
        </div>
      ))}

      <div className="mb-4">
        <Button size="sm" variant="secondary" onClick={() => setFiles((cur) => [...cur, { filename: "", content: "" }])}>
          Add file
        </Button>
      </div>

      {error && <ErrorBanner>{error}</ErrorBanner>}

      <DialogActions>
        <Button onClick={onClose} disabled={mutation.isPending} variant="ghost">
          Cancel
        </Button>
        <Button
          onClick={() => {
            setError(null);
            mutation.mutate();
          }}
          disabled={mutation.isPending || !valid}
          variant="primary"
        >
          {mutation.isPending ? "Creating…" : "Create gist"}
        </Button>
      </DialogActions>
    </Modal>
  );
}

function EditGistDialog({
  gist,
  onClose,
  onSaved,
}: {
  gist: BleephubGist;
  onClose: () => void;
  onSaved: () => void;
}) {
  const queryClient = useQueryClient();
  const [description, setDescription] = useState(gist.description);
  const [files, setFiles] = useState<{ filename: string; content?: string; original: string }[]>(
    Object.entries(gist.files).map(([name, file]) => ({
      filename: name,
      content: file.content,
      original: name,
    })),
  );
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      const fileMap: Record<string, BleephubGistFile | null> = {};
      files.forEach((f) => {
        if (f.filename.trim()) {
          fileMap[f.filename.trim()] = { content: f.content };
        }
      });
      Object.keys(gist.files).forEach((name) => {
        if (!files.some((f) => f.filename.trim() === name)) {
          fileMap[name] = null;
        }
      });
      return updateGist(gist.id, { description, files: fileMap });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gists", gist.id] });
      queryClient.invalidateQueries({ queryKey: ["gists"] });
      onSaved();
    },
    onError: (err: Error) => setError(err.message),
  });

  const updateFile = (idx: number, patch: Partial<{ filename: string; content?: string }>) => {
    setFiles((cur) => cur.map((f, i) => (i === idx ? { ...f, ...patch } : f)));
  };

  return (
    <Modal title="Edit gist" onClose={onClose}>
      <FormLabel id="gist-edit-desc">Description</FormLabel>
      <input
        id="gist-edit-desc"
        type="text"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel>Files</FormLabel>
      {files.map((file, idx) => (
        <div key={idx} className="mb-3 rounded border p-3" style={{ borderColor: "var(--color-border)" }}>
          <input
            type="text"
            value={file.filename}
            onChange={(e) => updateFile(idx, { filename: e.target.value })}
            placeholder="filename.ext"
            className="mb-2 w-full"
          />
          <textarea
            value={file.content || ""}
            onChange={(e) => updateFile(idx, { content: e.target.value })}
            rows={4}
            placeholder="file content"
            className="w-full"
            style={{ resize: "vertical" }}
          />
          <div className="mt-2 flex justify-end">
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setFiles((cur) => cur.filter((_, i) => i !== idx))}
              disabled={files.length === 1}
            >
              remove file
            </Button>
          </div>
        </div>
      ))}

      <div className="mb-4">
        <Button
          size="sm"
          variant="secondary"
          onClick={() => setFiles((cur) => [...cur, { filename: "", content: "", original: "" }])}
        >
          Add file
        </Button>
      </div>

      {error && <ErrorBanner>{error}</ErrorBanner>}

      <DialogActions>
        <Button onClick={onClose} disabled={mutation.isPending} variant="ghost">
          Cancel
        </Button>
        <Button
          onClick={() => {
            setError(null);
            mutation.mutate();
          }}
          disabled={mutation.isPending}
          variant="primary"
        >
          {mutation.isPending ? "Saving…" : "Save gist"}
        </Button>
      </DialogActions>
    </Modal>
  );
}
