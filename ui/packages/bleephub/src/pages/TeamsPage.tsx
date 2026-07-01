import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import {
  createTeam,
  deleteTeam,
  fetchTeams,
  updateTeam,
} from "../api.js";
import type { BleephubTeam } from "../types.js";
import {
  Button,
  Modal,
  FormLabel,
  ErrorBanner,
  DialogActions,
  PageTitle,
} from "../components/ui.js";

const col = createColumnHelper<BleephubTeam>();

export function TeamsPage() {
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div>
      <PageTitle
        title="Teams"
        meta="Internal bleephub teams."
        actions={
          <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
            New team
          </Button>
        }
      />
      <TeamsTable />
      {showCreate && <CreateTeamDialog onClose={() => setShowCreate(false)} />}
    </div>
  );
}

function TeamsTable() {
  const queryClient = useQueryClient();
  const [mutationError, setMutationError] = useState<string | null>(null);
  const [editing, setEditing] = useState<BleephubTeam | null>(null);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["teams"],
    queryFn: fetchTeams,
    refetchInterval: 5000,
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => deleteTeam(id),
    onSuccess: () => {
      setMutationError(null);
      queryClient.invalidateQueries({ queryKey: ["teams"] });
    },
    onError: (err: Error) => setMutationError(err.message),
  });

  if (isError) return <InlineError title="Failed to load teams" />;
  if (isLoading || !data) return <Spinner label="loading teams" />;

  const columns = [
    col.accessor("id", {
      header: "ID",
      cell: (info) => (
        <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
          {info.getValue()}
        </span>
      ),
    }),
    col.accessor("slug", {
      header: "Slug",
      cell: (info) => <span style={{ fontWeight: 500, color: "var(--color-fg)" }}>{info.getValue()}</span>,
    }),
    col.accessor("name", {
      header: "Name",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>,
    }),
    col.accessor("privacy", {
      header: "Privacy",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>,
    }),
    col.accessor("organization", {
      header: "Organization",
      cell: (info) => {
        const org = info.getValue();
        return <span style={{ color: "var(--color-fg-muted)" }}>{org ? `@${org.login}` : "—"}</span>;
      },
    }),
    col.accessor("created_at", {
      header: "Created",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
    col.display({
      id: "actions",
      header: "Actions",
      cell: (info) => {
        const team = info.row.original;
        return (
          <div className="flex gap-2">
            <Button size="sm" variant="ghost" onClick={() => setEditing(team)}>
              edit
            </Button>
            <Button
              size="sm"
              variant="danger"
              onClick={() => {
                if (confirm(`Delete team ${team.slug}?`)) {
                  deleteMut.mutate(team.id);
                }
              }}
              disabled={deleteMut.isPending}
            >
              delete
            </Button>
          </div>
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
        filterPlaceholder="Filter teams…"
        emptyMessage="No teams yet."
      />
      {editing && <EditTeamDialog team={editing} onClose={() => setEditing(null)} />}
    </>
  );
}

function CreateTeamDialog({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [org, setOrg] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [privacy, setPrivacy] = useState<"secret" | "closed">("secret");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      createTeam({
        org: org.trim(),
        name: name.trim(),
        description: description || undefined,
        privacy,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["teams"] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Modal title="Create team" onClose={onClose}>
      <FormLabel id="team-org">Organization login</FormLabel>
      <input
        id="team-org"
        type="text"
        value={org}
        onChange={(e) => setOrg(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="team-name">Name</FormLabel>
      <input
        id="team-name"
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="team-desc">Description</FormLabel>
      <input
        id="team-desc"
        type="text"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="team-privacy">Privacy</FormLabel>
      <select
        id="team-privacy"
        value={privacy}
        onChange={(e) => setPrivacy(e.target.value as "secret" | "closed")}
        className="mb-4 w-full"
      >
        <option value="secret">secret</option>
        <option value="closed">closed</option>
      </select>

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
          disabled={mutation.isPending || !org.trim() || !name.trim()}
          variant="primary"
        >
          {mutation.isPending ? "Creating…" : "Create team"}
        </Button>
      </DialogActions>
    </Modal>
  );
}

function EditTeamDialog({ team, onClose }: { team: BleephubTeam; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(team.name || "");
  const [description, setDescription] = useState(team.description || "");
  const [privacy, setPrivacy] = useState<"secret" | "closed">(team.privacy);
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      updateTeam(team.id, {
        name: name.trim() || undefined,
        description: description || undefined,
        privacy,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["teams"] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Modal title={`Edit ${team.slug}`} onClose={onClose}>
      <FormLabel id="team-edit-name">Name</FormLabel>
      <input
        id="team-edit-name"
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="team-edit-desc">Description</FormLabel>
      <input
        id="team-edit-desc"
        type="text"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="team-edit-privacy">Privacy</FormLabel>
      <select
        id="team-edit-privacy"
        value={privacy}
        onChange={(e) => setPrivacy(e.target.value as "secret" | "closed")}
        className="mb-4 w-full"
      >
        <option value="secret">secret</option>
        <option value="closed">closed</option>
      </select>

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
          {mutation.isPending ? "Saving…" : "Save"}
        </Button>
      </DialogActions>
    </Modal>
  );
}
