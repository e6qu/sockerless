import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import {
  createOrg,
  deleteOrg,
  fetchOrgs,
  updateOrg,
} from "../api.js";
import type { BleephubOrg } from "../types.js";
import {
  Button,
  Modal,
  FormLabel,
  ErrorBanner,
  DialogActions,
  PageTitle,
} from "../components/ui.js";

const col = createColumnHelper<BleephubOrg>();

export function OrgsPage() {
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div>
      <PageTitle
        title="Organizations"
        meta="Internal bleephub organizations."
        actions={
          <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
            New org
          </Button>
        }
      />
      <OrgsTable />
      {showCreate && <CreateOrgDialog onClose={() => setShowCreate(false)} />}
    </div>
  );
}

function OrgsTable() {
  const queryClient = useQueryClient();
  const [mutationError, setMutationError] = useState<string | null>(null);
  const [editing, setEditing] = useState<BleephubOrg | null>(null);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["orgs"],
    queryFn: fetchOrgs,
    refetchInterval: 5000,
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => deleteOrg(id),
    onSuccess: () => {
      setMutationError(null);
      queryClient.invalidateQueries({ queryKey: ["orgs"] });
    },
    onError: (err: Error) => setMutationError(err.message),
  });

  if (isError) return <InlineError title="Failed to load organizations" />;
  if (isLoading || !data) return <Spinner label="loading organizations" />;

  const columns = [
    col.accessor("id", {
      header: "ID",
      cell: (info) => (
        <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
          {info.getValue()}
        </span>
      ),
    }),
    col.accessor("login", {
      header: "Login",
      cell: (info) => <span style={{ fontWeight: 500, color: "var(--color-fg)" }}>{info.getValue()}</span>,
    }),
    col.accessor("name", {
      header: "Name",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue() || "—"}</span>,
    }),
    col.accessor("description", {
      header: "Description",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue() || "—"}</span>,
    }),
    col.accessor("created_at", {
      header: "Created",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
    col.display({
      id: "actions",
      header: "Actions",
      cell: (info) => {
        const org = info.row.original;
        return (
          <div className="flex gap-2">
            <Button size="sm" variant="ghost" onClick={() => setEditing(org)}>
              edit
            </Button>
            <Button
              size="sm"
              variant="danger"
              onClick={() => {
                if (confirm(`Delete org @${org.login}?`)) {
                  deleteMut.mutate(org.id);
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
        filterPlaceholder="Filter organizations…"
        emptyMessage="No organizations yet."
      />
      {editing && <EditOrgDialog org={editing} onClose={() => setEditing(null)} />}
    </>
  );
}

function CreateOrgDialog({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [login, setLogin] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [billingEmail, setBillingEmail] = useState("");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      createOrg({
        login: login.trim(),
        name: name || undefined,
        description: description || undefined,
        billing_email: billingEmail || undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["orgs"] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Modal title="Create organization" onClose={onClose}>
      <FormLabel id="org-login">Login</FormLabel>
      <input
        id="org-login"
        type="text"
        value={login}
        onChange={(e) => setLogin(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="org-name">Display name</FormLabel>
      <input
        id="org-name"
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="org-desc">Description</FormLabel>
      <input
        id="org-desc"
        type="text"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="org-email">Billing email</FormLabel>
      <input
        id="org-email"
        type="email"
        value={billingEmail}
        onChange={(e) => setBillingEmail(e.target.value)}
        className="mb-4 w-full"
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
          disabled={mutation.isPending || !login.trim()}
          variant="primary"
        >
          {mutation.isPending ? "Creating…" : "Create org"}
        </Button>
      </DialogActions>
    </Modal>
  );
}

function EditOrgDialog({ org, onClose }: { org: BleephubOrg; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(org.name || "");
  const [description, setDescription] = useState(org.description || "");
  const [billingEmail, setBillingEmail] = useState(org.billing_email || "");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      updateOrg(org.id, {
        name: name || undefined,
        description: description || undefined,
        billing_email: billingEmail || undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["orgs"] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Modal title={`Edit @${org.login}`} onClose={onClose}>
      <FormLabel id="org-edit-name">Display name</FormLabel>
      <input
        id="org-edit-name"
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="org-edit-desc">Description</FormLabel>
      <input
        id="org-edit-desc"
        type="text"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="org-edit-email">Billing email</FormLabel>
      <input
        id="org-edit-email"
        type="email"
        value={billingEmail}
        onChange={(e) => setBillingEmail(e.target.value)}
        className="mb-4 w-full"
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
          disabled={mutation.isPending}
          variant="primary"
        >
          {mutation.isPending ? "Saving…" : "Save"}
        </Button>
      </DialogActions>
    </Modal>
  );
}
