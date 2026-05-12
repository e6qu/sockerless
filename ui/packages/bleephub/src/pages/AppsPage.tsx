import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  DataTable,
  PageHeading,
  Spinner,
} from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { useState } from "react";
import {
  createApp,
  createOAuthApp,
  deleteInstallation,
  fetchApps,
  fetchInstallations,
  fetchOAuthApps,
  suspendInstallation,
} from "../api.js";
import type {
  BleephubApp,
  BleephubInstallation,
  BleephubOAuthApp,
} from "../types.js";

type Tab = "apps" | "installations" | "oauth-apps";

export function AppsPage() {
  const [tab, setTab] = useState<Tab>("apps");
  const [showCreate, setShowCreate] = useState<"app" | "oauth-app" | null>(null);

  return (
    <div>
      <PageHeading
        kicker="github · apps + oauth-apps"
        title={<>Apps &amp; installations</>}
        meta="GitHub Apps (JWT + ghs_), OAuth Apps (client_id/secret + gho_), and the active installations between them."
        actions={
          tab === "oauth-apps" ? (
            <Button variant="primary" size="sm" onClick={() => setShowCreate("oauth-app")}>
              + new oauth app
            </Button>
          ) : (
            <Button variant="primary" size="sm" onClick={() => setShowCreate("app")}>
              + new github app
            </Button>
          )
        }
      />

      <div
        className="mb-5 flex gap-1 -mx-1"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <TabButton active={tab === "apps"} onClick={() => setTab("apps")}>
          GitHub Apps
        </TabButton>
        <TabButton
          active={tab === "installations"}
          onClick={() => setTab("installations")}
        >
          Installations
        </TabButton>
        <TabButton active={tab === "oauth-apps"} onClick={() => setTab("oauth-apps")}>
          OAuth Apps
        </TabButton>
      </div>

      {tab === "apps" && <AppsTab />}
      {tab === "installations" && <InstallationsTab />}
      {tab === "oauth-apps" && <OAuthAppsTab />}

      {showCreate === "app" && <CreateAppDialog onClose={() => setShowCreate(null)} />}
      {showCreate === "oauth-app" && (
        <CreateOAuthAppDialog onClose={() => setShowCreate(null)} />
      )}
    </div>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="px-4 py-2 font-mono uppercase tracking-[0.12em]"
      style={{
        fontSize: "0.7rem",
        color: active ? "var(--color-fg)" : "var(--color-fg-muted)",
        borderBottom: `2px solid ${active ? "var(--color-accent)" : "transparent"}`,
        marginBottom: "-1px",
        transition: "all 0.12s var(--ease-out-quint)",
      }}
    >
      {children}
    </button>
  );
}

const appsCol = createColumnHelper<BleephubApp>();

function AppsTab() {
  const { data, isLoading } = useQuery({
    queryKey: ["apps"],
    queryFn: fetchApps,
    refetchInterval: 5000,
  });
  if (isLoading || !data) return <Spinner label="loading apps" />;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const columns: any[] = [
    appsCol.accessor("id", {
      header: "ID",
      cell: (info) => (
        <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
          {info.getValue()}
        </span>
      ),
    }),
    appsCol.accessor("slug", {
      header: "Slug",
      cell: (info) => (
        <span style={{ color: "var(--color-accent)" }}>{info.getValue()}</span>
      ),
    }),
    appsCol.accessor("name", {
      header: "Name",
      cell: (info) => (
        <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>
          {info.getValue()}
        </span>
      ),
    }),
    appsCol.accessor("description", {
      header: "Description",
      cell: (info) => (
        <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>
      ),
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
      emptyMessage="No apps yet. Click + new github app or POST /api/v3/bleephub/apps."
    />
  );
}

const installsCol = createColumnHelper<BleephubInstallation>();

function InstallationsTab() {
  const queryClient = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: ["installations"],
    queryFn: fetchInstallations,
    refetchInterval: 5000,
  });

  const suspendMut = useMutation({
    mutationFn: ({ id, suspend }: { id: number; suspend: boolean }) =>
      suspendInstallation(id, suspend),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["installations"] }),
  });
  const deleteMut = useMutation({
    mutationFn: (id: number) => deleteInstallation(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["installations"] }),
  });

  if (isLoading || !data) return <Spinner label="loading installations" />;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const columns: any[] = [
    installsCol.accessor("id", {
      header: "ID",
      cell: (info) => (
        <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
          {info.getValue()}
        </span>
      ),
    }),
    installsCol.accessor("appSlug", {
      header: "App",
      cell: (info) => (
        <span style={{ color: "var(--color-accent)" }}>{info.getValue()}</span>
      ),
    }),
    installsCol.accessor("targetLogin", {
      header: "Target",
      cell: (info) => (
        <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>
          {info.getValue()}
        </span>
      ),
    }),
    installsCol.accessor("targetType", {
      header: "Type",
      cell: (info) => (
        <span
          className="font-mono uppercase tracking-[0.1em]"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.65rem" }}
        >
          {info.getValue()}
        </span>
      ),
    }),
    installsCol.accessor("repositorySelection", {
      header: "Repo selection",
      cell: (info) => (
        <span className="font-mono" style={{ color: "var(--color-fg-muted)" }}>
          {info.getValue()}
        </span>
      ),
    }),
    installsCol.accessor("suspendedAt", {
      header: "Status",
      cell: (info) => {
        const suspended = !!info.getValue();
        return (
          <span
            className="font-mono uppercase tracking-[0.1em]"
            style={{
              fontSize: "0.65rem",
              color: suspended ? "var(--color-status-warning)" : "var(--color-status-ok)",
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
              variant="ghost"
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
    <DataTable
      data={data}
      columns={columns}
      filterPlaceholder="Filter installations…"
      emptyMessage="No installations. POST /api/v3/bleephub/apps/{app_id}/installations."
    />
  );
}

const oauthCol = createColumnHelper<BleephubOAuthApp>();

function OAuthAppsTab() {
  const { data, isLoading } = useQuery({
    queryKey: ["oauth-apps"],
    queryFn: fetchOAuthApps,
    refetchInterval: 5000,
  });
  if (isLoading || !data) return <Spinner label="loading oauth apps" />;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const columns: any[] = [
    oauthCol.accessor("clientId", {
      header: "Client ID",
      cell: (info) => (
        <span className="font-mono" style={{ color: "var(--color-accent)" }}>
          {info.getValue()}
        </span>
      ),
    }),
    oauthCol.accessor("name", {
      header: "Name",
      cell: (info) => (
        <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>{info.getValue()}</span>
      ),
    }),
    oauthCol.accessor("description", {
      header: "Description",
      cell: (info) => (
        <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>
      ),
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
      emptyMessage="No OAuth apps yet. Click + new oauth app or POST /api/v3/bleephub/oauth-apps."
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
    <Modal onClose={onClose}>
      <Kicker>new github app</Kicker>
      <DialogTitle>Create app</DialogTitle>

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
      <div className="mb-4 grid grid-cols-3 gap-2">
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
            className="font-mono"
            style={{ fontSize: "0.7rem", padding: "4px" }}
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
              onClick={() =>
                setEvents((cur) => (on ? cur.filter((e) => e !== ev) : [...cur, ev]))
              }
              className="font-mono"
              style={{
                fontSize: "0.7rem",
                padding: "4px 8px",
                background: on ? "var(--color-accent)" : "var(--color-bg-subtle)",
                color: on ? "var(--color-bg)" : "var(--color-fg-muted)",
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-sm)",
                cursor: "pointer",
              }}
            >
              {ev}
            </button>
          );
        })}
      </div>

      {error && <ErrorBlock>{error}</ErrorBlock>}

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
          {mutation.isPending ? "Creating…" : "Create →"}
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
    <Modal onClose={onClose}>
      <Kicker>app created — secrets shown once</Kicker>
      <DialogTitle>Save these now</DialogTitle>
      <p
        className="mb-4 text-xs"
        style={{ color: "var(--color-status-warning)" }}
      >
        These values will not be shown again. Copy them before closing this dialog.
      </p>

      {created.client_id && (
        <>
          <FormLabel>Client ID</FormLabel>
          <CodeBlock>{created.client_id}</CodeBlock>
        </>
      )}

      <FormLabel>Client Secret</FormLabel>
      <CodeBlock>{created.client_secret}</CodeBlock>

      <FormLabel>Webhook Secret</FormLabel>
      <CodeBlock>{created.webhook_secret}</CodeBlock>

      <FormLabel>PEM Private Key</FormLabel>
      <CodeBlock>{created.pem}</CodeBlock>

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
  const [created, setCreated] = useState<{
    client_id: string;
    client_secret: string;
  } | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      createOAuthApp({
        name,
        description,
        url,
        callback_url: callbackURL,
      }),
    onSuccess: (resp) => {
      queryClient.invalidateQueries({ queryKey: ["oauth-apps"] });
      setCreated({ client_id: resp.clientId, client_secret: resp.client_secret });
    },
    onError: (err: Error) => setError(err.message),
  });

  if (created) {
    return (
      <Modal onClose={onClose}>
        <Kicker>oauth app created</Kicker>
        <DialogTitle>Save your credentials</DialogTitle>
        <p className="mb-4 text-xs" style={{ color: "var(--color-status-warning)" }}>
          The client secret is shown once. Copy it now.
        </p>
        <FormLabel>Client ID</FormLabel>
        <CodeBlock>{created.client_id}</CodeBlock>
        <FormLabel>Client Secret</FormLabel>
        <CodeBlock>{created.client_secret}</CodeBlock>
        <DialogActions>
          <Button onClick={onClose} variant="primary">
            I copied it
          </Button>
        </DialogActions>
      </Modal>
    );
  }

  return (
    <Modal onClose={onClose}>
      <Kicker>new oauth app</Kicker>
      <DialogTitle>Create OAuth app</DialogTitle>

      <FormLabel id="oa-name">Name</FormLabel>
      <input
        id="oa-name"
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        className="mb-4 w-full"
      />

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

      {error && <ErrorBlock>{error}</ErrorBlock>}

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
          {mutation.isPending ? "Creating…" : "Create →"}
        </Button>
      </DialogActions>
    </Modal>
  );
}

// --- Shared dialog primitives ---

function Modal({
  onClose,
  children,
}: {
  onClose: () => void;
  children: React.ReactNode;
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center overflow-auto p-6"
      style={{ background: "color-mix(in oklch, var(--color-fg) 60%, transparent)" }}
      onClick={onClose}
    >
      <div
        className="w-full max-w-md p-6 my-auto"
        style={{
          background: "var(--color-surface-raised)",
          border: "1px solid var(--color-border-strong)",
          borderRadius: "var(--radius-sm)",
          boxShadow: "8px 8px 0 var(--color-accent)",
          maxHeight: "90vh",
          overflowY: "auto",
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
}

function Kicker({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="mb-1 text-[10px] uppercase tracking-[0.22em]"
      style={{ color: "var(--color-fg-subtle)" }}
    >
      {children}
    </div>
  );
}

function DialogTitle({ children }: { children: React.ReactNode }) {
  return (
    <h3
      className="mb-5 font-display"
      style={{
        fontStyle: "italic",
        fontWeight: 600,
        fontSize: "1.6rem",
        letterSpacing: "-0.025em",
        lineHeight: 1.05,
      }}
    >
      {children}
    </h3>
  );
}

function FormLabel({ id, children }: { id?: string; children: React.ReactNode }) {
  return (
    <label
      htmlFor={id}
      className="mb-1 block text-[10px] uppercase tracking-[0.18em]"
      style={{ color: "var(--color-fg-subtle)" }}
    >
      {children}
    </label>
  );
}

function ErrorBlock({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="mb-4 px-3 py-2 font-mono text-xs"
      style={{
        background: "var(--color-status-error-soft)",
        color: "var(--color-status-error)",
        border: "1px solid var(--color-status-error)",
        borderRadius: "var(--radius-sm)",
      }}
    >
      {children}
    </div>
  );
}

function DialogActions({ children }: { children: React.ReactNode }) {
  return <div className="flex justify-end gap-2">{children}</div>;
}

function CodeBlock({ children }: { children: React.ReactNode }) {
  return (
    <pre
      className="mb-4 px-3 py-2 font-mono"
      style={{
        background: "var(--color-bg-subtle)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-sm)",
        fontSize: "0.65rem",
        color: "var(--color-fg)",
        overflow: "auto",
        whiteSpace: "pre-wrap",
        wordBreak: "break-all",
      }}
    >
      {children}
    </pre>
  );
}
