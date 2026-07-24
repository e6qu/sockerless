import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AzureCommandBar, AzureEssentials } from "../portal/AzurePortal.js";
import {
  createAppRegistration,
  deleteAppRegistration,
  fetchAppRegistrations,
  type AppRegistration,
} from "../api.js";

// The Microsoft Entra ID App registrations blade: it lists the directory's
// application registrations from Microsoft Graph and registers new ones the
// way the real portal does — the application object plus its service
// principal — so the appId can immediately authenticate a client_credentials
// grant once a client secret is minted on the Certificates & secrets blade.
export function AppRegistrationsPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["entra-apps"],
    queryFn: fetchAppRegistrations,
  });
  const [registering, setRegistering] = useState(false);
  const [name, setName] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const create = useMutation({
    mutationFn: (displayName: string) => createAppRegistration(displayName),
    onSuccess: (app) => {
      void queryClient.invalidateQueries({ queryKey: ["entra-apps"] });
      setRegistering(false);
      setName("");
      void navigate(`/ui/entra/app-registrations/${app.id}`);
    },
  });

  const remove = useMutation({
    mutationFn: async (objectIds: string[]) => {
      for (const objectId of objectIds) {
        await deleteAppRegistration(objectId);
      }
    },
    onSuccess: () => {
      setSelected(new Set());
      void queryClient.invalidateQueries({ queryKey: ["entra-apps"] });
    },
  });

  const rows = data ?? [];
  const mutationError = create.error ?? remove.error;

  return (
    <>
      <AzureCommandBar
        commands={[
          { label: "New registration", icon: "add", onSelect: () => setRegistering(true) },
          { label: "Refresh", icon: "refresh", onSelect: () => void refetch(), disabled: isFetching },
          {
            label: "Delete",
            icon: "delete",
            disabled: selected.size === 0 || remove.isPending,
            onSelect: () => remove.mutate([...selected]),
          },
          { label: "Feedback", icon: "feedback" },
        ]}
      />
      <div className="az-main">
        <AzureEssentials
          properties={[
            { label: "Directory", value: "Simulator" },
            { label: "App registrations", value: String(rows.length) },
          ]}
        />

        {mutationError ? (
          <div className="az-message az-message-error" role="alert" data-testid="entra-apps-error">
            <strong>The directory operation failed.</strong>{" "}
            {mutationError instanceof Error ? mutationError.message : "Microsoft Graph did not respond."}
          </div>
        ) : null}

        {registering ? (
          <form
            className="az-form"
            data-testid="entra-new-registration-form"
            onSubmit={(event) => {
              event.preventDefault();
              if (name.trim()) create.mutate(name.trim());
            }}
          >
            <h2>Register an application</h2>
            <label>
              Name
              <input
                className="az-input"
                data-testid="entra-app-name-input"
                value={name}
                placeholder="Enter a display name for the application"
                onChange={(event) => setName(event.target.value)}
              />
            </label>
            <div className="az-form-actions">
              <button
                type="submit"
                className="az-button-primary"
                data-testid="entra-register-submit"
                disabled={!name.trim() || create.isPending}
              >
                Register
              </button>
              <button type="button" className="az-button" onClick={() => setRegistering(false)}>
                Cancel
              </button>
            </div>
          </form>
        ) : null}

        <table className="az-table" data-testid="entra-apps-table">
          <thead>
            <tr>
              <th className="az-table-select">
                <input
                  type="checkbox"
                  aria-label="Select all app registrations"
                  checked={rows.length > 0 && rows.every((row) => selected.has(row.id))}
                  disabled={rows.length === 0}
                  onChange={() =>
                    setSelected((current) =>
                      current.size === rows.length ? new Set() : new Set(rows.map((row) => row.id)),
                    )
                  }
                />
              </th>
              <th>Display name</th>
              <th>Application (client) ID</th>
              <th>Object ID</th>
            </tr>
          </thead>
          <tbody>
            {isError ? (
              <tr>
                <td className="az-table-state" colSpan={4}>
                  <div className="az-message az-message-error" role="alert">
                    <strong>Could not load app registrations.</strong>{" "}
                    {error instanceof Error ? error.message : "Microsoft Graph did not respond."}
                  </div>
                </td>
              </tr>
            ) : isLoading ? (
              <tr>
                <td className="az-table-state" colSpan={4}>
                  <div className="az-empty" role="status">Loading app registrations…</div>
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td className="az-table-state" colSpan={4}>
                  <div className="az-empty">
                    <strong>No app registrations to display</strong>
                    <p>Applications registered in this directory appear here.</p>
                  </div>
                </td>
              </tr>
            ) : (
              rows.map((row: AppRegistration) => (
                <tr key={row.id} data-testid="entra-app-row">
                  <td className="az-table-select">
                    <input
                      type="checkbox"
                      aria-label={`Select ${row.displayName}`}
                      checked={selected.has(row.id)}
                      onChange={() =>
                        setSelected((current) => {
                          const next = new Set(current);
                          if (next.has(row.id)) next.delete(row.id);
                          else next.add(row.id);
                          return next;
                        })
                      }
                    />
                  </td>
                  <td>
                    <Link to={`/ui/entra/app-registrations/${row.id}`}>{row.displayName}</Link>
                  </td>
                  <td>
                    <code>{row.appId}</code>
                  </td>
                  <td>
                    <code>{row.id}</code>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </>
  );
}
