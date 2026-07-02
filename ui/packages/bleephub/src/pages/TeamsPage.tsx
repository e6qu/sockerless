import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import {
  addTeamMember,
  addTeamRepo,
  createTeam,
  deleteTeam,
  fetchChildTeams,
  fetchTeamMembers,
  fetchTeamRepos,
  fetchTeams,
  removeTeamMember,
  removeTeamRepo,
  updateTeam,
} from "../api.js";
import type { BleephubTeam, GithubTeamMember, GithubTeamRepo } from "../types.js";
import {
  Button,
  Modal,
  FormLabel,
  ErrorBanner,
  DialogActions,
  PageTitle,
  Tabs,
  Box,
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
  const [viewing, setViewing] = useState<BleephubTeam | null>(null);

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
            <Button size="sm" variant="ghost" onClick={() => setViewing(team)}>
              view
            </Button>
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
      {viewing && <TeamDetailDialog team={viewing} onClose={() => setViewing(null)} />}
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

function TeamDetailDialog({ team, onClose }: { team: BleephubTeam; onClose: () => void }) {
  const [tab, setTab] = useState<"members" | "repos" | "children">("members");
  const org = team.organization?.login ?? "";
  const slug = team.slug;

  const tabs = [
    { key: "members" as const, label: "Members" },
    { key: "repos" as const, label: "Repositories" },
    { key: "children" as const, label: "Child teams" },
  ];

  return (
    <Modal title={`${team.name} (@${team.slug})`} onClose={onClose}>
      <div className="mb-4" style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
        {team.description || "No description."} · {team.privacy}
      </div>
      <Tabs items={tabs} active={tab} onChange={setTab} />
      {tab === "members" && <TeamMembersPanel org={org} slug={slug} />}
      {tab === "repos" && <TeamReposPanel org={org} slug={slug} />}
      {tab === "children" && <TeamChildrenPanel org={org} slug={slug} />}
    </Modal>
  );
}

function TeamMembersPanel({ org, slug }: { org: string; slug: string }) {
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [username, setUsername] = useState("");
  const [role, setRole] = useState("member");

  const query = useQuery({
    queryKey: ["team-members", org, slug],
    queryFn: () => fetchTeamMembers(org, slug),
    enabled: !!org && !!slug,
  });

  const addMut = useMutation({
    mutationFn: () => addTeamMember(org, slug, username.trim(), role),
    onSuccess: () => {
      setError(null);
      setUsername("");
      queryClient.invalidateQueries({ queryKey: ["team-members", org, slug] });
    },
    onError: (err: Error) => setError(err.message),
  });

  const removeMut = useMutation({
    mutationFn: (username: string) => removeTeamMember(org, slug, username),
    onSuccess: () => {
      setError(null);
      queryClient.invalidateQueries({ queryKey: ["team-members", org, slug] });
    },
    onError: (err: Error) => setError(err.message),
  });

  if (query.isLoading) return <Spinner label="loading members" />;
  if (query.isError) return <InlineError title="Failed to load members" />;

  const members = query.data ?? [];

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <Box header={<span style={{ fontWeight: 600 }}>Add member</span>}>
        <div style={{ padding: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
          <div className="flex gap-2">
            <input
              type="text"
              placeholder="Username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full"
            />
            <select value={role} onChange={(e) => setRole(e.target.value)}>
              <option value="member">member</option>
              <option value="maintainer">maintainer</option>
            </select>
            <Button
              variant="primary"
              onClick={() => {
                setError(null);
                addMut.mutate();
              }}
              disabled={addMut.isPending || !username.trim()}
            >
              Add
            </Button>
          </div>
        </div>
      </Box>
      <Box header={<span style={{ fontWeight: 600 }}>Members</span>}>
        <div style={{ padding: "0" }}>
          {members.length === 0 ? (
            <div style={{ padding: "1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
              No members.
            </div>
          ) : (
            <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
              {members.map((m: GithubTeamMember) => (
                <li
                  key={m.id}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    padding: "0.6rem 1rem",
                    borderBottom: "1px solid var(--color-border)",
                  }}
                >
                  <span style={{ fontWeight: 500 }}>@{m.login}</span>
                  <span style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
                    {m.role ?? "member"}
                  </span>
                  <Button
                    size="sm"
                    variant="danger"
                    onClick={() => {
                      if (confirm(`Remove ${m.login} from team?`)) {
                        removeMut.mutate(m.login);
                      }
                    }}
                    disabled={removeMut.isPending}
                  >
                    remove
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </Box>
    </div>
  );
}

function TeamReposPanel({ org, slug }: { org: string; slug: string }) {
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [repoInput, setRepoInput] = useState("");
  const [permission, setPermission] = useState("push");

  const query = useQuery({
    queryKey: ["team-repos", org, slug],
    queryFn: () => fetchTeamRepos(org, slug),
    enabled: !!org && !!slug,
  });

  const addMut = useMutation({
    mutationFn: () => {
      const [owner, repo] = repoInput.trim().split("/");
      if (!owner || !repo) throw new Error("Enter repo as owner/name");
      return addTeamRepo(org, slug, owner, repo, permission);
    },
    onSuccess: () => {
      setError(null);
      setRepoInput("");
      queryClient.invalidateQueries({ queryKey: ["team-repos", org, slug] });
    },
    onError: (err: Error) => setError(err.message),
  });

  const removeMut = useMutation({
    mutationFn: ({ owner, repo }: { owner: string; repo: string }) => removeTeamRepo(org, slug, owner, repo),
    onSuccess: () => {
      setError(null);
      queryClient.invalidateQueries({ queryKey: ["team-repos", org, slug] });
    },
    onError: (err: Error) => setError(err.message),
  });

  if (query.isLoading) return <Spinner label="loading repos" />;
  if (query.isError) return <InlineError title="Failed to load repos" />;

  const repos = query.data ?? [];

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <Box header={<span style={{ fontWeight: 600 }}>Add repository</span>}>
        <div style={{ padding: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
          <div className="flex gap-2">
            <input
              type="text"
              placeholder="owner/name"
              value={repoInput}
              onChange={(e) => setRepoInput(e.target.value)}
              className="w-full"
            />
            <select value={permission} onChange={(e) => setPermission(e.target.value)}>
              <option value="pull">pull</option>
              <option value="triage">triage</option>
              <option value="push">push</option>
              <option value="maintain">maintain</option>
              <option value="admin">admin</option>
            </select>
            <Button
              variant="primary"
              onClick={() => {
                setError(null);
                addMut.mutate();
              }}
              disabled={addMut.isPending || !repoInput.includes("/")}
            >
              Add
            </Button>
          </div>
        </div>
      </Box>
      <Box header={<span style={{ fontWeight: 600 }}>Repositories</span>}>
        <div style={{ padding: "0" }}>
          {repos.length === 0 ? (
            <div style={{ padding: "1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
              No repositories.
            </div>
          ) : (
            <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
              {repos.map((r: GithubTeamRepo) => (
                <li
                  key={r.id}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    padding: "0.6rem 1rem",
                    borderBottom: "1px solid var(--color-border)",
                  }}
                >
                  <span style={{ fontWeight: 500 }}>{r.full_name}</span>
                  <span style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
                    {r.role_name ?? "—"}
                  </span>
                  <Button
                    size="sm"
                    variant="danger"
                    onClick={() => {
                      if (confirm(`Remove ${r.full_name} from team?`)) {
                        removeMut.mutate({ owner: r.owner.login, repo: r.name });
                      }
                    }}
                    disabled={removeMut.isPending}
                  >
                    remove
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </Box>
    </div>
  );
}

function TeamChildrenPanel({ org, slug }: { org: string; slug: string }) {
  const query = useQuery({
    queryKey: ["team-children", org, slug],
    queryFn: () => fetchChildTeams(org, slug),
    enabled: !!org && !!slug,
  });

  if (query.isLoading) return <Spinner label="loading child teams" />;
  if (query.isError) return <InlineError title="Failed to load child teams" />;

  const children = query.data ?? [];

  return (
    <Box header={<span style={{ fontWeight: 600 }}>Child teams</span>}>
      <div style={{ padding: "0" }}>
        {children.length === 0 ? (
          <div style={{ padding: "1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
            No child teams.
          </div>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {children.map((t: BleephubTeam) => (
              <li
                key={t.id}
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  padding: "0.6rem 1rem",
                  borderBottom: "1px solid var(--color-border)",
                }}
              >
                <span style={{ fontWeight: 500 }}>@{t.slug}</span>
                <span style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
                  {t.privacy}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Box>
  );
}
