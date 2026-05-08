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
  fetchApps,
  fetchInstallations,
} from "../api.js";
import type { BleephubApp, BleephubInstallation } from "../types.js";

type Tab = "apps" | "installations";

export function AppsPage() {
  const [tab, setTab] = useState<Tab>("apps");
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div>
      <PageHeading
        kicker="github · apps"
        title={<>Apps &amp; installations</>}
        meta="GitHub-shape App + Installation registry. Tokens minted via /api/v3/app/installations/{id}/access_tokens."
        actions={
          <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
            + new app
          </Button>
        }
      />

      <div
        className="mb-5 flex gap-1 -mx-1"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <TabButton active={tab === "apps"} onClick={() => setTab("apps")}>
          Apps
        </TabButton>
        <TabButton
          active={tab === "installations"}
          onClick={() => setTab("installations")}
        >
          Installations
        </TabButton>
      </div>

      {tab === "apps" ? <AppsTab /> : <InstallationsTab />}
      {showCreate && <CreateAppDialog onClose={() => setShowCreate(false)} />}
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
      emptyMessage="No apps yet. Click + new app or POST /api/v3/bleephub/apps."
    />
  );
}

const installsCol = createColumnHelper<BleephubInstallation>();

function InstallationsTab() {
  const { data, isLoading } = useQuery({
    queryKey: ["installations"],
    queryFn: fetchInstallations,
    refetchInterval: 5000,
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
        <span
          className="font-mono"
          style={{ color: "var(--color-fg-muted)" }}
        >
          {info.getValue()}
        </span>
      ),
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

function CreateAppDialog({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => createApp({ name, description }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["apps"] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ background: "color-mix(in oklch, var(--color-fg) 60%, transparent)" }}
      onClick={onClose}
    >
      <div
        className="w-full max-w-md p-6"
        style={{
          background: "var(--color-surface-raised)",
          border: "1px solid var(--color-border-strong)",
          borderRadius: "var(--radius-sm)",
          boxShadow: "8px 8px 0 var(--color-accent)",
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div
          className="mb-1 text-[10px] uppercase tracking-[0.22em]"
          style={{ color: "var(--color-fg-subtle)" }}
        >
          new github app
        </div>
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
          Create app
        </h3>

        <label
          htmlFor="app-name"
          className="mb-1 block text-[10px] uppercase tracking-[0.18em]"
          style={{ color: "var(--color-fg-subtle)" }}
        >
          Name
        </label>
        <input
          id="app-name"
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="mb-4 w-full"
        />

        <label
          htmlFor="app-desc"
          className="mb-1 block text-[10px] uppercase tracking-[0.18em]"
          style={{ color: "var(--color-fg-subtle)" }}
        >
          Description
        </label>
        <textarea
          id="app-desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          rows={3}
          className="mb-4 w-full"
          style={{ resize: "vertical" }}
        />

        {error && (
          <div
            className="mb-4 px-3 py-2 font-mono text-xs"
            style={{
              background: "var(--color-status-error-soft)",
              color: "var(--color-status-error)",
              border: "1px solid var(--color-status-error)",
              borderRadius: "var(--radius-sm)",
            }}
          >
            {error}
          </div>
        )}

        <div className="flex justify-end gap-2">
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
        </div>
      </div>
    </div>
  );
}
