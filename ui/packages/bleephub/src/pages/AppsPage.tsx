import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { useState } from "react";
import { Link } from "react-router";
import {
  createApp,
  createOAuthApp,
  deleteInstallation,
  fetchApps,
  fetchInstallations,
  fetchOAuthApps,
  suspendInstallation,
} from "../api.js";
import type { BleephubApp, BleephubInstallation, BleephubOAuthApp } from "../types.js";
import {
  PageTitle,
  Tabs,
  Button,
  Modal,
  FormLabel,
  ErrorBanner,
  DialogActions,
  CodeBlock,
} from "../components/ui.js";

type Tab = "apps" | "installations" | "oauth-apps";

export function AppsPage() {
  const [tab, setTab] = useState<Tab>("apps");
  const [showCreate, setShowCreate] = useState<"app" | "oauth-app" | null>(null);

  return (
    <div>
      <PageTitle
        title="Apps & installations"
        meta="GitHub Apps, OAuth Apps, and the active installations between them."
        actions={
          tab === "oauth-apps" ? (
            <Button variant="primary" size="sm" onClick={() => setShowCreate("oauth-app")}>
              New OAuth app
            </Button>
          ) : (
            <Button variant="primary" size="sm" onClick={() => setShowCreate("app")}>
              New GitHub app
            </Button>
          )
        }
      />

      <Tabs
        items={[
          { key: "apps", label: "GitHub Apps" },
          { key: "installations", label: "Installations" },
          { key: "oauth-apps", label: "OAuth Apps" },
        ]}
        active={tab}
        onChange={setTab}
      />

      {tab === "apps" && <AppsTab />}
      {tab === "installations" && <InstallationsTab />}
      {tab === "oauth-apps" && <OAuthAppsTab />}

      {showCreate === "app" && <CreateAppDialog onClose={() => setShowCreate(null)} />}
      {showCreate === "oauth-app" && <CreateOAuthAppDialog onClose={() => setShowCreate(null)} />}
    </div>
  );
}

const appsCol = createColumnHelper<BleephubApp>();

function AppsTab() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["apps"],
    queryFn: fetchApps,
    refetchInterval: 5000,
  });
  if (isError) return <InlineError title="Failed to load apps" />;
  if (isLoading || !data) return <Spinner label="loading apps" />;

  const columns = [
    appsCol.accessor("id", {
      header: "Identifier",
      cell: (info) => (
        <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
          {info.getValue()}
        </span>
      ),
    }),
    appsCol.accessor("slug", {
      header: "Slug",
      cell: (info) => <Link to={`/ui/apps/${info.getValue()}/marketplace`} style={{ color: "var(--color-accent)", fontWeight: 600 }}>{info.getValue()}</Link>,
    }),
    appsCol.accessor("name", {
      header: "Name",
      cell: (info) => <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>{info.getValue()}</span>,
    }),
    appsCol.accessor("description", {
      header: "Description",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>,
    }),
    appsCol.accessor("createdAt", {
      header: "Created",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
  ];

  return (
    <DataTable
      data={data}
      columns={columns}
      filterPlaceholder="Filter apps…"
      emptyMessage="No apps yet. Create a GitHub App through the manifest flow."
    />
  );
}

const installsCol = createColumnHelper<BleephubInstallation>();

function InstallationsTab() {
  const queryClient = useQueryClient();
  const [mutationError, setMutationError] = useState<string | null>(null);
  const { data, isLoading, isError } = useQuery({
    queryKey: ["installations"],
    queryFn: fetchInstallations,
    refetchInterval: 5000,
  });

  const suspendMut = useMutation({
    mutationFn: ({ id, suspend }: { id: number; suspend: boolean }) => suspendInstallation(id, suspend),
    onSuccess: () => {
      setMutationError(null);
      queryClient.invalidateQueries({ queryKey: ["installations"] });
    },
    onError: (err: Error) => setMutationError(err.message),
  });
  const deleteMut = useMutation({
    mutationFn: (id: number) => deleteInstallation(id),
    onSuccess: () => {
      setMutationError(null);
      queryClient.invalidateQueries({ queryKey: ["installations"] });
    },
    onError: (err: Error) => setMutationError(err.message),
  });

  if (isError) return <InlineError title="Failed to load installations" />;
  if (isLoading || !data) return <Spinner label="loading installations" />;

  const columns = [
    installsCol.accessor("id", {
      header: "Identifier",
      cell: (info) => (
        <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
          {info.getValue()}
        </span>
      ),
    }),
    installsCol.accessor("appSlug", {
      header: "App",
      cell: (info) => <span style={{ color: "var(--color-accent)" }}>{info.getValue()}</span>,
    }),
    installsCol.accessor("targetLogin", {
      header: "Target",
      cell: (info) => <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>{info.getValue()}</span>,
    }),
    installsCol.accessor("targetType", {
      header: "Type",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>,
    }),
    installsCol.accessor("repositorySelection", {
      header: "Repo selection",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>,
    }),
    installsCol.accessor("suspendedAt", {
      header: "Status",
      cell: (info) => {
        const suspended = !!info.getValue();
        return (
          <span
            style={{
              fontSize: "0.78rem",
              fontWeight: 500,
              color: suspended ? "var(--color-status-warn)" : "var(--color-status-ok)",
            }}
          >
            {suspended ? "suspended" : "active"}
          </span>
        );
      },
    }),
    installsCol.display({
      id: "actions",
      header: "Actions",
      cell: (info) => {
        const inst = info.row.original;
        const suspended = !!inst.suspendedAt;
        return (
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="ghost"
              onClick={() => suspendMut.mutate({ id: inst.id, suspend: !suspended })}
              disabled={suspendMut.isPending}
            >
              {suspended ? "unsuspend" : "suspend"}
            </Button>
            <Button
              size="sm"
              variant="danger"
              onClick={() => {
                if (confirm(`Delete installation #${inst.id}?`)) {
                  deleteMut.mutate(inst.id);
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
    installsCol.accessor("createdAt", {
      header: "Created",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
  ];

  return (
    <>
      {mutationError && <ErrorBanner>{mutationError}</ErrorBanner>}
      <DataTable
        data={data}
        columns={columns}
        filterPlaceholder="Filter installations…"
        emptyMessage="No installations."
      />
    </>
  );
}

const oauthCol = createColumnHelper<BleephubOAuthApp>();

function OAuthAppsTab() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["oauth-apps"],
    queryFn: fetchOAuthApps,
    refetchInterval: 5000,
  });
  if (isError) return <InlineError title="Failed to load oauth apps" />;
  if (isLoading || !data) return <Spinner label="loading oauth apps" />;

  const columns = [
    oauthCol.accessor("clientId", {
      header: "Client identifier",
      cell: (info) => (
        <span className="font-mono" style={{ color: "var(--color-accent)" }}>
          {info.getValue()}
        </span>
      ),
    }),
    oauthCol.accessor("name", {
      header: "Name",
      cell: (info) => <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>{info.getValue()}</span>,
    }),
    oauthCol.accessor("description", {
      header: "Description",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>,
    }),
    oauthCol.accessor("callbackUrl", {
      header: "Callback",
      cell: (info) => (
        <span className="font-mono" style={{ color: "var(--color-fg-muted)" }}>
          {info.getValue() || "—"}
        </span>
      ),
    }),
    oauthCol.accessor("createdAt", {
      header: "Created",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
  ];

  return (
    <DataTable
      data={data}
      columns={columns}
      filterPlaceholder="Filter OAuth apps…"
      emptyMessage="No OAuth Apps yet."
    />
  );
}

const allPermScopes = [
  "metadata",
  "contents",
  "issues",
  "pull_requests",
  "actions",
  "checks",
  "secrets",
  "administration",
  "members",
];

const allEvents = [
  "push",
  "pull_request",
  "issues",
  "issue_comment",
  "installation",
  "installation_repositories",
  "check_run",
  "check_suite",
];

function CreateAppDialog({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [perms, setPerms] = useState<Record<string, string>>({});
  const [events, setEvents] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<{
    pem: string;
    client_id?: string;
    client_secret: string;
    webhook_secret: string;
  } | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      createApp({
        name,
        description,
        permissions: Object.keys(perms).length ? perms : undefined,
        events: events.length ? events : undefined,
      }),
    onSuccess: (resp) => {
      queryClient.invalidateQueries({ queryKey: ["apps"] });
      setCreated({
        pem: resp.pem,
        client_id: resp.clientId,
        client_secret: resp.client_secret,
        webhook_secret: resp.webhook_secret,
      });
    },
    onError: (err: Error) => setError(err.message),
  });

  if (created) {
    return <CreatedAppDialog created={created} onClose={onClose} />;
  }

  return (
    <Modal title="Create GitHub app" onClose={onClose}>
      <FormLabel id="app-name">Name</FormLabel>
      <input
        id="app-name"
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="app-desc">Description</FormLabel>
      <textarea
        id="app-desc"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        rows={2}
        className="mb-4 w-full"
        style={{ resize: "vertical" }}
      />

      <FormLabel>Permissions</FormLabel>
      <div className="mb-4 grid grid-cols-2 gap-2 sm:grid-cols-3">
        {allPermScopes.map((scope) => (
          <select
            key={scope}
            value={perms[scope] || ""}
            onChange={(e) => {
              const v = e.target.value;
              setPerms((cur) => {
                const next = { ...cur };
                if (v === "") delete next[scope];
                else next[scope] = v;
                return next;
              });
            }}
            style={{ fontSize: "0.78rem", padding: "0.3rem 0.4rem" }}
          >
            <option value="">{scope}: —</option>
            <option value="read">{scope}: read</option>
            <option value="write">{scope}: write</option>
            <option value="admin">{scope}: admin</option>
          </select>
        ))}
      </div>

      <FormLabel>Events</FormLabel>
      <div className="mb-4 flex flex-wrap gap-2">
        {allEvents.map((ev) => {
          const on = events.includes(ev);
          return (
            <button
              type="button"
              key={ev}
              onClick={() => setEvents((cur) => (on ? cur.filter((e) => e !== ev) : [...cur, ev]))}
              style={{
                fontSize: "0.76rem",
                padding: "0.2rem 0.55rem",
                background: on ? "var(--color-accent)" : "var(--color-bg-subtle)",
                color: on ? "var(--color-accent-fg)" : "var(--color-fg-muted)",
                border: "1px solid var(--color-border)",
                borderRadius: "2rem",
                cursor: "pointer",
              }}
            >
              {ev}
            </button>
          );
        })}
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
          disabled={mutation.isPending || !name.trim()}
          variant="primary"
        >
          {mutation.isPending ? "Creating…" : "Create app"}
        </Button>
      </DialogActions>
    </Modal>
  );
}

function CreatedAppDialog({
  created,
  onClose,
}: {
  created: { pem: string; client_id?: string; client_secret: string; webhook_secret: string };
  onClose: () => void;
}) {
  return (
    <Modal title="Save these now" onClose={onClose}>
      <p className="mb-4" style={{ fontSize: "0.82rem", color: "var(--color-status-warn)" }}>
        These values will not be shown again. Copy them before closing this dialog.
      </p>

      {created.client_id && (
        <>
          <FormLabel>Client identifier</FormLabel>
          <div className="mb-4">
            <CodeBlock>{created.client_id}</CodeBlock>
          </div>
        </>
      )}

      <FormLabel>Client secret</FormLabel>
      <div className="mb-4">
        <CodeBlock>{created.client_secret}</CodeBlock>
      </div>

      <FormLabel>Webhook secret</FormLabel>
      <div className="mb-4">
        <CodeBlock>{created.webhook_secret}</CodeBlock>
      </div>

      <FormLabel>Privacy Enhanced Mail private key</FormLabel>
      <div className="mb-4">
        <CodeBlock>{created.pem}</CodeBlock>
      </div>

      <DialogActions>
        <Button onClick={onClose} variant="primary">
          I copied them
        </Button>
      </DialogActions>
    </Modal>
  );
}

function CreateOAuthAppDialog({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [url, setURL] = useState("");
  const [callbackURL, setCallbackURL] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<{ client_id: string; client_secret: string } | null>(null);

  const mutation = useMutation({
    mutationFn: () => createOAuthApp({ name, description, url, callback_url: callbackURL }),
    onSuccess: (resp) => {
      queryClient.invalidateQueries({ queryKey: ["oauth-apps"] });
      setCreated({ client_id: resp.clientId, client_secret: resp.client_secret });
    },
    onError: (err: Error) => setError(err.message),
  });

  if (created) {
    return (
      <Modal title="Save your credentials" onClose={onClose}>
        <p className="mb-4" style={{ fontSize: "0.82rem", color: "var(--color-status-warn)" }}>
          The client secret is shown once. Copy it now.
        </p>
        <FormLabel>Client identifier</FormLabel>
        <div className="mb-4">
          <CodeBlock>{created.client_id}</CodeBlock>
        </div>
        <FormLabel>Client secret</FormLabel>
        <div className="mb-4">
          <CodeBlock>{created.client_secret}</CodeBlock>
        </div>
        <DialogActions>
          <Button onClick={onClose} variant="primary">
            I copied it
          </Button>
        </DialogActions>
      </Modal>
    );
  }

  return (
    <Modal title="Create OAuth app" onClose={onClose}>
      <FormLabel id="oa-name">Name</FormLabel>
      <input id="oa-name" type="text" value={name} onChange={(e) => setName(e.target.value)} className="mb-4 w-full" />

      <FormLabel id="oa-desc">Description</FormLabel>
      <input
        id="oa-desc"
        type="text"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="oa-url">Homepage URL</FormLabel>
      <input
        id="oa-url"
        type="text"
        value={url}
        onChange={(e) => setURL(e.target.value)}
        className="mb-4 w-full"
        placeholder="https://example.test"
      />

      <FormLabel id="oa-cb">Callback URL</FormLabel>
      <input
        id="oa-cb"
        type="text"
        value={callbackURL}
        onChange={(e) => setCallbackURL(e.target.value)}
        className="mb-4 w-full"
        placeholder="https://example.test/auth/callback"
      />

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
          disabled={mutation.isPending || !name.trim()}
          variant="primary"
        >
          {mutation.isPending ? "Creating…" : "Create app"}
        </Button>
      </DialogActions>
    </Modal>
  );
}
