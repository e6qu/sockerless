import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "react-router";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import {
  createRepoCodespace,
  createUserCodespace,
  deleteCodespace,
  fetchCodespaceMachines,
  fetchRepoCodespaces,
  fetchRepos,
  fetchUserCodespaces,
  startCodespace,
  stopCodespace,
} from "../api.js";
import type { BleephubRepo, GithubCodespace, GithubCodespaceState } from "../types.js";
import {
  Blankslate,
  Box,
  Button,
  DialogActions,
  ErrorBanner,
  FormLabel,
  Modal,
  PageTitle,
  StateLabel,
} from "../components/ui.js";
import { CodespaceIcon, PlayIcon, SquareIcon, TrashIcon } from "../components/octicons.js";

const col = createColumnHelper<GithubCodespace>();

function stateLabel(state: GithubCodespaceState): { state: "open" | "closed" | "draft"; label: string } {
  switch (state) {
    case "Available":
      return { state: "open", label: "available" };
    case "Shutdown":
      return { state: "closed", label: "shutdown" };
    case "Creating":
      return { state: "draft", label: "creating" };
    case "Unavailable":
      return { state: "closed", label: "unavailable" };
  }
}

export function CodespacesPage() {
  const { owner, repo } = useParams<{ owner?: string; repo?: string }>();
  const repoScope = owner && repo ? `${owner}/${repo}` : null;
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div>
      <PageTitle
        icon={<CodespaceIcon size={20} />}
        title="Codespaces"
        meta={repoScope ? `Codespaces for ${repoScope}.` : "Your personal codespaces."}
        actions={
          <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
            New codespace
          </Button>
        }
      />

      <CodespacesList repoScope={repoScope} />

      {showCreate && (
        <CreateCodespaceDialog repoScope={repoScope} onClose={() => setShowCreate(false)} />
      )}
    </div>
  );
}

function CodespacesList({ repoScope }: { repoScope: string | null }) {
  const queryClient = useQueryClient();
  const queryKey = repoScope ? ["codespaces", "repo", repoScope] : ["codespaces", "user"];

  const {
    data,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey,
    queryFn: () =>
      repoScope
        ? fetchRepoCodespaces(repoScope.split("/")[0], repoScope.split("/")[1])
        : fetchUserCodespaces(),
    refetchInterval: 10000,
  });

  const startMut = useMutation({
    mutationFn: (name: string) => startCodespace(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey }),
  });
  const stopMut = useMutation({
    mutationFn: (name: string) => stopCodespace(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey }),
  });
  const deleteMut = useMutation({
    mutationFn: (name: string) => deleteCodespace(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey }),
  });

  const columns = useMemo(
    () => [
      col.accessor("name", {
        header: "Name",
        cell: (info) => (
          <span className="font-medium">{info.getValue()}</span>
        ),
      }),
      col.accessor("display_name", {
        header: "Display name",
      }),
      col.accessor("state", {
        header: "State",
        cell: (info) => {
          const s = stateLabel(info.getValue<GithubCodespaceState>());
          return <StateLabel state={s.state}>{s.label}</StateLabel>;
        },
      }),
      col.accessor("machine", {
        header: "Machine",
        cell: (info) => info.getValue<GithubCodespace["machine"]>().display_name,
      }),
      col.accessor("image", {
        header: "Image",
        cell: (info) => (
          <span className="truncate" style={{ maxWidth: 240, color: "var(--color-fg-muted)" }}>
            {info.getValue()}
          </span>
        ),
      }),
      col.accessor("last_used_at", {
        header: "Last used",
        cell: (info) => new Date(info.getValue<string>()).toLocaleString(),
      }),
      col.display({
        id: "actions",
        header: "Actions",
        cell: (info) => {
          const cs = info.row.original;
          return (
            <div className="flex flex-wrap items-center gap-2">
              {cs.state !== "Available" ? (
                <Button size="sm" variant="secondary" onClick={() => startMut.mutate(cs.name)}>
                  <PlayIcon size={14} /> Start
                </Button>
              ) : (
                <Button size="sm" variant="secondary" onClick={() => stopMut.mutate(cs.name)}>
                  <SquareIcon size={14} /> Stop
                </Button>
              )}
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  if (confirm(`Delete codespace ${cs.name}?`)) {
                    deleteMut.mutate(cs.name);
                  }
                }}
              >
                <TrashIcon size={14} /> Delete
              </Button>
            </div>
          );
        },
      }),
    ],
    [startMut, stopMut, deleteMut],
  );

  if (isLoading) return <Spinner />;
  if (isError) return <InlineError title="Failed to load codespaces" detail={error instanceof Error ? error : String(error)} />;

  const items = data?.items ?? [];
  if (items.length === 0) {
    return (
      <Blankslate title="No codespaces" icon={<CodespaceIcon size={32} />}>
        {repoScope
          ? `There are no codespaces for ${repoScope} yet.`
          : "You don't have any codespaces yet."}
      </Blankslate>
    );
  }

  return (
    <Box>
      <DataTable columns={columns} data={items} />
      {startMut.error && <ErrorBanner>{String(startMut.error)}</ErrorBanner>}
      {stopMut.error && <ErrorBanner>{String(stopMut.error)}</ErrorBanner>}
      {deleteMut.error && <ErrorBanner>{String(deleteMut.error)}</ErrorBanner>}
    </Box>
  );
}

function CreateCodespaceDialog({
  repoScope,
  onClose,
}: {
  repoScope: string | null;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const { data: repos } = useQuery({
    queryKey: ["repos"],
    queryFn: fetchRepos,
  });
  const [selectedRepo, setSelectedRepo] = useState<string>(repoScope ?? "");
  const [machine, setMachine] = useState("basicLinux32");
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState<unknown>(null);

  const createMut = useMutation({
    mutationFn: async () => {
      if (repoScope) {
        const [owner, repo] = repoScope.split("/");
        return createRepoCodespace(owner, repo, { machine, display_name: displayName });
      }
      const repo = repos?.find((r: BleephubRepo) => r.full_name === selectedRepo);
      if (!repo) throw new Error("Select a repository.");
      return createUserCodespace({ repository_id: repo.id, machine, display_name: displayName });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: repoScope ? ["codespaces", "repo", repoScope] : ["codespaces", "user"],
      });
      onClose();
    },
    onError: (err) => setError(err),
  });

  return (
    <Modal title="New codespace" onClose={onClose}>
      <div className="flex flex-col gap-4">
        {error ? <ErrorBanner>{error instanceof Error ? error.message : String(error)}</ErrorBanner> : null}

        {!repoScope && (
          <div>
            <FormLabel id="cs-repo">Repository</FormLabel>
            <select
              id="cs-repo"
              value={selectedRepo}
              onChange={(e) => setSelectedRepo(e.target.value)}
              className="w-full"
            >
              <option value="">Select a repository</option>
              {repos?.map((r: BleephubRepo) => (
                <option key={r.id} value={r.full_name}>
                  {r.full_name}
                </option>
              ))}
            </select>
          </div>
        )}

        <div>
          <FormLabel id="cs-machine">Machine</FormLabel>
          <MachineSelect owner={repoScope?.split("/")[0] ?? ""} repo={repoScope?.split("/")[1] ?? ""} value={machine} onChange={setMachine} />
        </div>

        <div>
          <FormLabel id="cs-display">Display name</FormLabel>
          <input
            id="cs-display"
            type="text"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            placeholder="Optional display name"
            className="w-full"
          />
        </div>
      </div>

      <DialogActions>
        <Button variant="secondary" onClick={onClose}>
          Cancel
        </Button>
        <Button variant="primary" onClick={() => createMut.mutate()} disabled={createMut.isPending}>
          Create codespace
        </Button>
      </DialogActions>
    </Modal>
  );
}

function MachineSelect({
  owner,
  repo,
  value,
  onChange,
}: {
  owner: string;
  repo: string;
  value: string;
  onChange: (v: string) => void;
}) {
  const { data, isLoading } = useQuery({
    queryKey: ["codespaces", "machines", owner, repo],
    queryFn: () => fetchCodespaceMachines(owner, repo),
    enabled: !!owner && !!repo,
  });

  if (!owner || !repo) {
    return (
      <select value={value} onChange={(e) => onChange(e.target.value)} className="w-full">
        <option value="basicLinux32">basicLinux32</option>
        <option value="standardLinux32">standardLinux32</option>
        <option value="premiumLinux64">premiumLinux64</option>
      </select>
    );
  }

  if (isLoading) return <Spinner label="Loading machines" />;

  return (
    <select value={value} onChange={(e) => onChange(e.target.value)} className="w-full">
      {data?.items.map((m) => (
        <option key={m.name} value={m.name}>
          {m.display_name}
        </option>
      ))}
    </select>
  );
}
