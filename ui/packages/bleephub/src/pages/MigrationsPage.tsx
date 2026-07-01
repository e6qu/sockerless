import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import {
  createOrgMigration,
  createUserMigration,
  deleteMigrationArchive,
  downloadMigrationArchive,
  fetchOrgMigrationLockStatus,
  fetchOrgMigrations,
  fetchRepos,
  fetchUserMigrations,
  unlockMigrationRepo,
} from "../api.js";
import type { BleephubRepo, GithubMigration, GithubMigrationState } from "../types.js";
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
  Tabs,
} from "../components/ui.js";
import { DownloadIcon, MigrationIcon } from "../components/octicons.js";

type Scope = { kind: "user" } | { kind: "org"; org: string };

const col = createColumnHelper<GithubMigration>();

function stateLabel(state: GithubMigrationState): { state: "open" | "closed" | "draft"; label: string } {
  switch (state) {
    case "exported":
      return { state: "open", label: "exported" };
    case "failed":
      return { state: "closed", label: "failed" };
    case "pending":
      return { state: "draft", label: "pending" };
    case "exporting":
      return { state: "draft", label: "exporting" };
  }
}

export function MigrationsPage() {
  const [tab, setTab] = useState<"user" | "org">("user");
  const [orgInput, setOrgInput] = useState("");
  const [loadedOrg, setLoadedOrg] = useState("");
  const [showCreate, setShowCreate] = useState(false);

  const activeScope: Scope = tab === "user" ? { kind: "user" } : { kind: "org", org: loadedOrg };

  return (
    <div>
      <PageTitle
        icon={<MigrationIcon size={20} />}
        title="Migrations"
        meta="Export user and organization repositories."
        actions={
          <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
            New migration
          </Button>
        }
      />

      <Tabs<"user" | "org">
        items={[
          { key: "user", label: "User" },
          { key: "org", label: "Organization" },
        ]}
        active={tab}
        onChange={(k) => {
          setTab(k);
          if (k === "org" && orgInput.trim()) {
            setLoadedOrg(orgInput.trim());
          }
        }}
      />

      {tab === "org" && (
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <FormLabel id="migration-org">Organization</FormLabel>
          <input
            id="migration-org"
            type="text"
            value={orgInput}
            onChange={(e) => setOrgInput(e.target.value)}
            placeholder="org-login"
            className="w-64"
          />
          <Button
            size="sm"
            variant="secondary"
            onClick={() => setLoadedOrg(orgInput.trim())}
            disabled={!orgInput.trim()}
          >
            Load
          </Button>
        </div>
      )}

      {tab === "user" ? (
        <MigrationsList scope={{ kind: "user" }} />
      ) : loadedOrg ? (
        <MigrationsList scope={{ kind: "org", org: loadedOrg }} />
      ) : (
        <Blankslate title="Organization migrations" icon={<MigrationIcon size={32} />}>
          Enter an organization login and click Load to see its migrations.
        </Blankslate>
      )}

      {showCreate && (
        <CreateMigrationDialog
          initialScope={activeScope}
          onClose={() => setShowCreate(false)}
        />
      )}
    </div>
  );
}

function MigrationsList({ scope }: { scope: Scope }) {
  const queryClient = useQueryClient();
  const [detail, setDetail] = useState<GithubMigration | null>(null);

  const queryKey =
    scope.kind === "user" ? ["migrations", "user"] : ["migrations", "org", scope.org];

  const {
    data,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey,
    queryFn: () =>
      scope.kind === "user" ? fetchUserMigrations() : fetchOrgMigrations(scope.org),
    refetchInterval: 10000,
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => deleteMigrationArchive(scope, id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey }),
  });

  const columns = useMemo(
    () => [
      col.accessor("id", {
        header: "ID",
        cell: (info) => (
          <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
            {info.getValue()}
          </span>
        ),
      }),
      col.accessor("state", {
        header: "State",
        cell: (info) => {
          const s = stateLabel(info.getValue<GithubMigrationState>());
          return <StateLabel state={s.state}>{s.label}</StateLabel>;
        },
      }),
      col.accessor("repositories", {
        header: "Repositories",
        cell: (info) => info.getValue<BleephubRepo[]>().length,
      }),
      col.accessor("lock_repositories", {
        header: "Locked",
        cell: (info) => (info.getValue() ? "yes" : "no"),
      }),
      col.accessor("exported_at", {
        header: "Exported",
        cell: (info) => new Date(info.getValue<string>()).toLocaleString(),
      }),
      col.display({
        id: "actions",
        header: "Actions",
        cell: (info) => {
          const migration = info.row.original;
          return (
            <div className="flex flex-wrap items-center gap-2">
              <Button
                size="sm"
                variant="secondary"
                onClick={() =>
                  downloadMigrationArchive(scope, migration.id, `migration-${migration.id}.tar.gz`)
                }
              >
                <DownloadIcon size={14} /> Download
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  if (confirm("Delete this migration archive?")) {
                    deleteMut.mutate(migration.id);
                  }
                }}
                disabled={deleteMut.isPending}
              >
                Delete archive
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setDetail(migration)}>
                Details
              </Button>
            </div>
          );
        },
      }),
    ],
    [deleteMut, scope],
  );

  if (isError) {
    return (
      <InlineError
        title={`Failed to load ${scope.kind} migrations`}
        detail={String(error)}
      />
    );
  }
  if (isLoading || !data) {
    return <Spinner label={`loading ${scope.kind} migrations`} />;
  }

  return (
    <>
      <DataTable
        data={data}
        columns={columns}
        filterPlaceholder="Filter migrations…"
        emptyMessage={`No ${scope.kind} migrations yet.`}
      />
      {detail && (
        <MigrationDetailDialog migration={detail} scope={scope} onClose={() => setDetail(null)} />
      )}
    </>
  );
}

function MigrationDetailDialog({
  migration,
  scope,
  onClose,
}: {
  migration: GithubMigration;
  scope: Scope;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [unlocked, setUnlocked] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string | null>(null);

  const unlockMut = useMutation({
    mutationFn: (repoName: string) => unlockMigrationRepo(scope, migration.id, repoName),
    onSuccess: (_, repoName) => {
      setError(null);
      setUnlocked((prev) => {
        const next = new Set(prev);
        next.add(repoName);
        return next;
      });
      queryClient.invalidateQueries({
        queryKey:
          scope.kind === "user" ? ["migrations", "user"] : ["migrations", "org", scope.org],
      });
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Modal title={`Migration ${migration.id}`} onClose={onClose}>
      <div className="mb-4 grid grid-cols-2 gap-3 text-sm">
        <Box header="State">
          <div className="p-3">
            {(() => {
              const s = stateLabel(migration.state);
              return <StateLabel state={s.state}>{s.label}</StateLabel>;
            })()}
          </div>
        </Box>
        <Box header="GUID">
          <div
            className="p-3 font-mono"
            style={{ color: "var(--color-fg-muted)", fontSize: "0.78rem", wordBreak: "break-all" }}
          >
            {migration.guid}
          </div>
        </Box>
      </div>

      <div className="mb-4 text-sm" style={{ color: "var(--color-fg-muted)" }}>
        Exported {new Date(migration.exported_at).toLocaleString()} ·{" "}
        {migration.repositories.length} repository
        {migration.repositories.length === 1 ? "" : "ies"}
      </div>

      {error && <ErrorBanner>{error}</ErrorBanner>}

      <div className="mb-2" style={{ fontSize: "0.82rem", fontWeight: 600 }}>
        Repositories
      </div>
      <div
        className="mb-4 divide-y"
        style={{ border: "1px solid var(--color-border)", borderRadius: "var(--radius-md)" }}
      >
        {migration.repositories.map((repo) => (
          <RepoLockRow
            key={repo.id}
            repo={repo}
            scope={scope}
            migrationId={migration.id}
            unlocked={unlocked.has(repo.name)}
            onUnlock={() => unlockMut.mutate(repo.name)}
            isPending={unlockMut.isPending}
          />
        ))}
      </div>

      <DialogActions>
        <Button onClick={onClose} variant="ghost">
          Close
        </Button>
        <Button
          variant="secondary"
          onClick={() =>
            downloadMigrationArchive(scope, migration.id, `migration-${migration.id}.tar.gz`)
          }
        >
          <DownloadIcon size={14} /> Download archive
        </Button>
      </DialogActions>
    </Modal>
  );
}

function RepoLockRow({
  repo,
  scope,
  migrationId,
  unlocked,
  onUnlock,
  isPending,
}: {
  repo: BleephubRepo;
  scope: Scope;
  migrationId: number;
  unlocked: boolean;
  onUnlock: () => void;
  isPending: boolean;
}) {
  const lockQ = useQuery({
    queryKey: ["migration-lock", scope.kind === "org" ? scope.org : "user", migrationId, repo.name],
    queryFn: () =>
      scope.kind === "org"
        ? fetchOrgMigrationLockStatus(scope.org, migrationId, repo.name)
        : Promise.resolve({ locked: !unlocked }),
    enabled: scope.kind === "org",
    staleTime: 5000,
  });

  const locked = scope.kind === "org" ? lockQ.data?.locked ?? !unlocked : !unlocked;

  return (
    <div
      className="flex items-center justify-between gap-3 p-3"
      style={{ fontSize: "0.85rem" }}
    >
      <span className="font-mono" style={{ color: "var(--color-fg)" }}>
        {repo.full_name}
      </span>
      {locked ? (
        <Button size="sm" variant="ghost" onClick={onUnlock} disabled={isPending}>
          Unlock
        </Button>
      ) : (
        <span style={{ color: "var(--color-fg-muted)" }}>unlocked</span>
      )}
    </div>
  );
}

function CreateMigrationDialog({
  initialScope,
  onClose,
}: {
  initialScope: Scope;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<"user" | "org">(initialScope.kind);
  const [org, setOrg] = useState(initialScope.kind === "org" ? initialScope.org : "");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [lock, setLock] = useState(true);
  const [exclude, setExclude] = useState({
    metadata: false,
    gitData: false,
    attachments: false,
    releases: false,
    ownerProjects: false,
    orgMetaOnly: false,
  });
  const [filter, setFilter] = useState("");
  const [error, setError] = useState<string | null>(null);

  const reposQ = useQuery({ queryKey: ["repos"], queryFn: fetchRepos });

  const mutation = useMutation({
    mutationFn: async () => {
      const payload = {
        repositories: Array.from(selected),
        lock_repositories: lock,
        exclude_metadata: exclude.metadata,
        exclude_git_data: exclude.gitData,
        exclude_attachments: exclude.attachments,
        exclude_releases: exclude.releases,
        exclude_owner_projects: exclude.ownerProjects,
        org_metadata_only: exclude.orgMetaOnly,
      };
      return tab === "user"
        ? createUserMigration(payload)
        : createOrgMigration(org.trim(), payload);
    },
    onSuccess: () => {
      const key =
        tab === "user" ? ["migrations", "user"] : ["migrations", "org", org.trim()];
      queryClient.invalidateQueries({ queryKey: key });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  const filteredRepos = useMemo(() => {
    if (!reposQ.data) return [];
    const f = filter.toLowerCase();
    return reposQ.data.filter(
      (r) =>
        r.full_name.toLowerCase().includes(f) || r.name.toLowerCase().includes(f),
    );
  }, [reposQ.data, filter]);

  const valid = selected.size > 0 && (tab === "user" || org.trim() !== "");

  const toggleRepo = (fullName: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(fullName)) next.delete(fullName);
      else next.add(fullName);
      return next;
    });
  };

  return (
    <Modal title="New migration" onClose={onClose}>
      <Tabs<"user" | "org">
        items={[
          { key: "user", label: "User migration" },
          { key: "org", label: "Organization migration" },
        ]}
        active={tab}
        onChange={setTab}
      />

      {tab === "org" && (
        <div className="mb-4">
          <FormLabel id="create-org">Organization</FormLabel>
          <input
            id="create-org"
            type="text"
            value={org}
            onChange={(e) => setOrg(e.target.value)}
            placeholder="org-login"
            className="w-full"
          />
        </div>
      )}

      <div className="mb-4">
        <label className="inline-flex items-center gap-2">
          <input
            type="checkbox"
            checked={lock}
            onChange={(e) => setLock(e.target.checked)}
          />
          <span style={{ fontSize: "0.85rem" }}>Lock repositories during migration</span>
        </label>
      </div>

      <div className="mb-4 grid grid-cols-2 gap-x-4 gap-y-2">
        <Checkbox
          label="Exclude metadata"
          checked={exclude.metadata}
          onChange={(v) => setExclude((c) => ({ ...c, metadata: v }))}
        />
        <Checkbox
          label="Exclude git data"
          checked={exclude.gitData}
          onChange={(v) => setExclude((c) => ({ ...c, gitData: v }))}
        />
        <Checkbox
          label="Exclude attachments"
          checked={exclude.attachments}
          onChange={(v) => setExclude((c) => ({ ...c, attachments: v }))}
        />
        <Checkbox
          label="Exclude releases"
          checked={exclude.releases}
          onChange={(v) => setExclude((c) => ({ ...c, releases: v }))}
        />
        <Checkbox
          label="Exclude owner projects"
          checked={exclude.ownerProjects}
          onChange={(v) => setExclude((c) => ({ ...c, ownerProjects: v }))}
        />
        {tab === "org" && (
          <Checkbox
            label="Org metadata only"
            checked={exclude.orgMetaOnly}
            onChange={(v) => setExclude((c) => ({ ...c, orgMetaOnly: v }))}
          />
        )}
      </div>

      <div className="mb-2">
        <FormLabel id="repo-filter">Repositories ({selected.size} selected)</FormLabel>
        <input
          id="repo-filter"
          type="text"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Filter repositories…"
          className="w-full"
        />
      </div>

      <div
        className="mb-4 overflow-y-auto"
        style={{
          maxHeight: "16rem",
          border: "1px solid var(--color-border)",
          borderRadius: "var(--radius-md)",
          padding: "0.5rem",
        }}
      >
        {reposQ.isLoading && <Spinner label="loading repos" />}
        {reposQ.isError && <InlineError title="Failed to load repos" />}
        {reposQ.data &&
          (filteredRepos.length === 0 ? (
            <div style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
              No repositories match.
            </div>
          ) : (
            filteredRepos.map((repo) => (
              <label
                key={repo.id}
                className="flex items-center gap-2 py-1"
                style={{ fontSize: "0.85rem" }}
              >
                <input
                  type="checkbox"
                  checked={selected.has(repo.full_name)}
                  onChange={() => toggleRepo(repo.full_name)}
                />
                <span className="font-mono">{repo.full_name}</span>
              </label>
            ))
          ))}
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
          {mutation.isPending ? "Starting…" : "Start migration"}
        </Button>
      </DialogActions>
    </Modal>
  );
}

function Checkbox({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="inline-flex items-center gap-2">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span style={{ fontSize: "0.82rem" }}>{label}</span>
    </label>
  );
}
