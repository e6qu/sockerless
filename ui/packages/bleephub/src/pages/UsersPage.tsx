import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import {
  createUser,
  deleteUser,
  fetchUsers,
  updateUser,
} from "../api.js";
import type { BleephubUser } from "../types.js";
import {
  Button,
  Modal,
  FormLabel,
  ErrorBanner,
  DialogActions,
  PageTitle,
} from "../components/ui.js";

const col = createColumnHelper<BleephubUser>();

export function UsersPage() {
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div>
      <PageTitle
        title="Users"
        meta="Internal bleephub user accounts."
        actions={
          <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
            New user
          </Button>
        }
      />
      <UsersTable />
      {showCreate && <CreateUserDialog onClose={() => setShowCreate(false)} />}
    </div>
  );
}

function UsersTable() {
  const queryClient = useQueryClient();
  const [mutationError, setMutationError] = useState<string | null>(null);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["users"],
    queryFn: fetchUsers,
    refetchInterval: 5000,
  });

  const updateMut = useMutation({
    mutationFn: ({ id, site_admin }: { id: number; site_admin: boolean }) =>
      updateUser(id, { site_admin }),
    onSuccess: () => {
      setMutationError(null);
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (err: Error) => setMutationError(err.message),
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => deleteUser(id),
    onSuccess: () => {
      setMutationError(null);
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (err: Error) => setMutationError(err.message),
  });

  if (isError) return <InlineError title="Failed to load users" />;
  if (isLoading || !data) return <Spinner label="loading users" />;

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
    col.accessor("type", {
      header: "Type",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>,
    }),
    col.accessor("site_admin", {
      header: "Site admin",
      cell: (info) => {
        const user = info.row.original;
        return (
          <label className="inline-flex cursor-pointer items-center gap-2">
            <input
              type="checkbox"
              checked={info.getValue()}
              onChange={(e) =>
                updateMut.mutate({ id: user.id, site_admin: e.target.checked })
              }
              disabled={updateMut.isPending}
            />
            <span style={{ fontSize: "0.82rem" }}>{info.getValue() ? "yes" : "no"}</span>
          </label>
        );
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
        const user = info.row.original;
        return (
          <Button
            size="sm"
            variant="danger"
            onClick={() => {
              if (confirm(`Delete user @${user.login}?`)) {
                deleteMut.mutate(user.id);
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
        filterPlaceholder="Filter users…"
        emptyMessage="No users yet."
      />
    </>
  );
}

function CreateUserDialog({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [siteAdmin, setSiteAdmin] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      createUser({
        login: login.trim(),
        password: password || undefined,
        site_admin: siteAdmin,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Modal title="Create user" onClose={onClose}>
      <FormLabel id="user-login">Login</FormLabel>
      <input
        id="user-login"
        type="text"
        value={login}
        onChange={(e) => setLogin(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="user-password">Password</FormLabel>
      <input
        id="user-password"
        type="password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        className="mb-4 w-full"
      />

      <label className="mb-4 inline-flex items-center gap-2">
        <input
          type="checkbox"
          checked={siteAdmin}
          onChange={(e) => setSiteAdmin(e.target.checked)}
        />
        <span style={{ fontSize: "0.82rem" }}>Site administrator</span>
      </label>

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
          {mutation.isPending ? "Creating…" : "Create user"}
        </Button>
      </DialogActions>
    </Modal>
  );
}
