import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import {
  createGist,
  deleteGist,
  fetchGist,
  fetchGistCommits,
  fetchGistForks,
  fetchGists,
  fetchPublicGists,
  fetchStarredGists,
  forkGist,
  isGistStarred,
  starGist,
  unstarGist,
  updateGist,
} from "../api.js";
import type { BleephubGist, BleephubGistFile, GithubGistCommit } from "../types.js";
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
  Tabs,
} from "../components/ui.js";
import { GistIcon, StarIcon, BranchIcon } from "../components/octicons.js";

type GistScope = "yours" | "public" | "starred";

const col = createColumnHelper<BleephubGist>();

export function GistsPage() {
  const [scope, setScope] = useState<GistScope>("yours");
  const [showCreate, setShowCreate] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  return (
    <div>
      <PageTitle
        icon={<GistIcon size={20} />}
        title="Gists"
        meta="Code snippets and notes."
        actions={
          <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
            New gist
          </Button>
        }
      />

      <Tabs<GistScope>
        items={[
          { key: "yours", label: "Yours" },
          { key: "public", label: "Public" },
          { key: "starred", label: "Starred" },
        ]}
        active={scope}
        onChange={setScope}
      />

      <GistsTable scope={scope} onSelect={(id) => setSelectedId(id)} />
      {showCreate && <CreateGistDialog onClose={() => setShowCreate(false)} />}
      {selectedId && <GistDetail id={selectedId} onClose={() => setSelectedId(null)} />}
    </div>
  );
}

function gistsQueryFn(scope: GistScope) {
  switch (scope) {
    case "public":
      return fetchPublicGists;
    case "starred":
      return fetchStarredGists;
    case "yours":
    default:
      return fetchGists;
  }
}

function GistsTable({ scope, onSelect }: { scope: GistScope; onSelect: (id: string) => void }) {
  const queryClient = useQueryClient();
  const [mutationError, setMutationError] = useState<string | null>(null);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["gists", scope],
    queryFn: gistsQueryFn(scope),
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
    col.accessor("owner", {
      header: "Owner",
      cell: (info) => (
        <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>{info.getValue()?.login}</span>
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
  const [detailTab, setDetailTab] = useState<"files" | "commits" | "forks">("files");
  const [actionError, setActionError] = useState<string | null>(null);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["gists", id],
    queryFn: () => fetchGist(id),
  });

  const { data: starred, isLoading: starLoading } = useQuery({
    queryKey: ["gists", id, "starred"],
    queryFn: () => isGistStarred(id),
  });

  const starMut = useMutation({
    mutationFn: () => (starred ? unstarGist(id) : starGist(id)),
    onSuccess: () => {
      setActionError(null);
      queryClient.invalidateQueries({ queryKey: ["gists", id, "starred"] });
      queryClient.invalidateQueries({ queryKey: ["gists", "starred"] });
    },
    onError: (err: Error) => setActionError(err.message),
  });

  const forkMut = useMutation({
    mutationFn: () => forkGist(id),
    onSuccess: () => {
      setActionError(null);
      queryClient.invalidateQueries({ queryKey: ["gists"] });
    },
    onError: (err: Error) => setActionError(err.message),
  });

  if (isError) return <InlineError title="Failed to load gist" />;
  if (isLoading || !data) return <Spinner label="loading gist" />;

  const files = Object.entries(data.files);

  return (
    <Modal title={data.description || `Gist ${data.id}`} onClose={onClose}>
      {actionError && <ErrorBanner>{actionError}</ErrorBanner>}

      <div className="mb-4 flex flex-wrap items-center gap-2">
        {data.public ? (
          <StateLabel state="open">public</StateLabel>
        ) : (
          <StateLabel state="closed">secret</StateLabel>
        )}
        <span style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
          {files.length} file{files.length === 1 ? "" : "s"}
        </span>
        <Button
          size="sm"
          variant={starred ? "primary" : "secondary"}
          onClick={() => starMut.mutate()}
          disabled={starLoading || starMut.isPending}
        >
          <StarIcon size={14} /> {starred ? "Unstar" : "Star"}
        </Button>
        <Button
          size="sm"
          variant="secondary"
          onClick={() => forkMut.mutate()}
          disabled={forkMut.isPending}
        >
          <BranchIcon size={14} /> Fork
        </Button>
        <Button onClick={() => setEditing(data)} variant="ghost" size="sm">
          Edit
        </Button>
      </div>

      <Tabs<"files" | "commits" | "forks">
        items={[
          { key: "files", label: "Files" },
          { key: "commits", label: "History" },
          { key: "forks", label: "Forks" },
        ]}
        active={detailTab}
        onChange={setDetailTab}
      />

      {detailTab === "files" && (
        <div>
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
        </div>
      )}

      {detailTab === "commits" && <GistCommits id={id} />}
      {detailTab === "forks" && <GistForks id={id} />}

      <DialogActions>
        <Button onClick={onClose} variant="ghost">
          Close
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

function GistCommits({ id }: { id: string }) {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["gists", id, "commits"],
    queryFn: () => fetchGistCommits(id),
  });

  if (isError) return <InlineError title="Failed to load history" />;
  if (isLoading || !data) return <Spinner label="loading history" />;

  if (data.length === 0) {
    return (
      <div style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>No history available.</div>
    );
  }

  return (
    <div className="space-y-2">
      {data.map((commit) => (
        <CommitRow key={commit.version} commit={commit} />
      ))}
    </div>
  );
}

function CommitRow({ commit }: { commit: GithubGistCommit }) {
  const additions = commit.change_status?.additions ?? 0;
  const deletions = commit.change_status?.deletions ?? 0;
  return (
    <div
      className="flex flex-col gap-1 rounded border p-3"
      style={{ borderColor: "var(--color-border)" }}
    >
      <div className="flex items-center justify-between">
        <span className="font-mono text-sm" style={{ color: "var(--color-fg)" }}>
          {commit.version.slice(0, 7)}
        </span>
        <span style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
          {new Date(commit.committed_at).toLocaleString()}
        </span>
      </div>
      <div style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
        {commit.user?.login ?? "unknown"}
      </div>
      {(additions > 0 || deletions > 0) && (
        <div className="flex gap-3" style={{ fontSize: "0.78rem" }}>
          <span style={{ color: "var(--gh-open-solid)" }}>+{additions}</span>
          <span style={{ color: "var(--color-status-error)" }}>-{deletions}</span>
        </div>
      )}
    </div>
  );
}

function GistForks({ id }: { id: string }) {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["gists", id, "forks"],
    queryFn: () => fetchGistForks(id),
  });

  if (isError) return <InlineError title="Failed to load forks" />;
  if (isLoading || !data) return <Spinner label="loading forks" />;

  if (data.length === 0) {
    return (
      <div style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>No forks yet.</div>
    );
  }

  return (
    <div className="space-y-2">
      {data.map((fork) => (
        <Box key={fork.id} header={fork.owner?.login ?? "unknown"} className="p-3">
          <div className="flex items-center justify-between">
            <span className="font-mono text-sm" style={{ color: "var(--color-fg-muted)" }}>{fork.id}</span>
            <span style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
              {new Date(fork.created_at).toLocaleString()}
            </span>
          </div>
          <div style={{ fontSize: "0.82rem" }}>{fork.description || "(no description)"}</div>
        </Box>
      ))}
    </div>
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
