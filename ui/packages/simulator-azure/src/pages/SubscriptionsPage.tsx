import { useState } from "react";
import { Link } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AzureCommandBar, AzureEssentials } from "../portal/AzurePortal.js";
import {
  createSubscriptionAlias,
  fetchSubscriptions,
  getSubscriptionAlias,
  type Subscription,
} from "../api.js";

// The default enrollment-account billing scope offered by the create form —
// the coordinate a real tenant reads from its Microsoft.Billing accounts; the
// operator can point the form at any scope they hold.
const DEFAULT_BILLING_SCOPE =
  "/providers/Microsoft.Billing/billingAccounts/sim-billing-account/enrollmentAccounts/sim-enrollment";

interface SubscriptionCreateFormProps {
  busy: boolean;
  // The alias's live provisioningState while a creation is in flight; empty
  // when idle.
  provisioningState: string;
  onCreate: (displayName: string, billingScope: string) => void;
  onDismiss: () => void;
}

// SubscriptionCreateForm is the "Add subscription" blade form: a display name
// and the billing scope the subscription is created under, driving the real
// Microsoft.Subscription alias PUT and reporting the alias's provisioning
// state until the subscription materializes.
export function SubscriptionCreateForm({ busy, provisioningState, onCreate, onDismiss }: SubscriptionCreateFormProps) {
  const [name, setName] = useState("");
  const [scope, setScope] = useState(DEFAULT_BILLING_SCOPE);
  return (
    <form
      className="az-form"
      data-testid="subs-create-form"
      onSubmit={(event) => {
        event.preventDefault();
        if (name.trim() && scope.trim()) onCreate(name.trim(), scope.trim());
      }}
    >
      <h2>Create a subscription</h2>
      <label>
        Subscription name
        <input
          className="az-input"
          data-testid="subs-create-name"
          value={name}
          placeholder="Enter a name for the subscription"
          onChange={(event) => setName(event.target.value)}
        />
      </label>
      <label>
        Billing scope
        <input
          className="az-input"
          data-testid="subs-create-scope"
          value={scope}
          onChange={(event) => setScope(event.target.value)}
        />
      </label>
      {provisioningState ? (
        <p className="az-form-status" role="status" data-testid="subs-provisioning">
          Creating subscription… provisioning state: {provisioningState}
        </p>
      ) : null}
      <div className="az-form-actions">
        <button
          type="submit"
          className="az-button-primary"
          data-testid="subs-create-submit"
          disabled={!name.trim() || !scope.trim() || busy}
        >
          Create
        </button>
        <button type="button" className="az-button" onClick={onDismiss} disabled={busy}>
          Cancel
        </button>
      </div>
    </form>
  );
}

// The Subscriptions blade: every subscription in the directory from the real
// Azure Resource Manager subscriptions list, and an Add flow that creates one
// through the Microsoft.Subscription alias API, polling the alias until the
// subscription is provisioned.
export function SubscriptionsPage() {
  const queryClient = useQueryClient();
  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["subscriptions"],
    queryFn: fetchSubscriptions,
  });
  const [adding, setAdding] = useState(false);
  const [provisioningState, setProvisioningState] = useState("");

  const create = useMutation({
    mutationFn: async ({ displayName, billingScope }: { displayName: string; billingScope: string }) => {
      let alias = await createSubscriptionAlias(displayName, billingScope);
      setProvisioningState(alias.provisioningState);
      while (alias.provisioningState !== "Succeeded") {
        if (alias.provisioningState === "Failed") {
          throw new Error(`subscription provisioning failed for alias ${alias.name}`);
        }
        await new Promise((resolve) => setTimeout(resolve, 1000));
        alias = await getSubscriptionAlias(alias.name);
        setProvisioningState(alias.provisioningState);
      }
      return alias;
    },
    onSuccess: () => {
      setAdding(false);
      void queryClient.invalidateQueries({ queryKey: ["subscriptions"] });
    },
    onSettled: () => setProvisioningState(""),
  });

  const rows = data ?? [];

  return (
    <>
      <AzureCommandBar
        commands={[
          { label: "Add", icon: "add", testid: "subs-add", onSelect: () => setAdding(true) },
          { label: "Refresh", icon: "refresh", onSelect: () => void refetch(), disabled: isFetching },
          { label: "Feedback", icon: "feedback" },
        ]}
      />
      <div className="az-main">
        <AzureEssentials
          properties={[
            { label: "Directory", value: "Simulator" },
            { label: "Subscriptions", value: String(rows.length) },
          ]}
        />

        {create.error ? (
          <div className="az-message az-message-error" role="alert" data-testid="subs-error">
            <strong>The subscription could not be created.</strong>{" "}
            {create.error instanceof Error ? create.error.message : "Azure Resource Manager did not respond."}
          </div>
        ) : null}

        {adding ? (
          <SubscriptionCreateForm
            busy={create.isPending}
            provisioningState={provisioningState}
            onCreate={(displayName, billingScope) => create.mutate({ displayName, billingScope })}
            onDismiss={() => setAdding(false)}
          />
        ) : null}

        <table className="az-table" data-testid="subs-table">
          <thead>
            <tr>
              <th>Subscription name</th>
              <th>Subscription ID</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {isError ? (
              <tr>
                <td className="az-table-state" colSpan={3}>
                  <div className="az-message az-message-error" role="alert" data-testid="subs-list-error">
                    <strong>Could not load subscriptions.</strong>{" "}
                    {error instanceof Error ? error.message : "Azure Resource Manager did not respond."}
                  </div>
                </td>
              </tr>
            ) : isLoading ? (
              <tr>
                <td className="az-table-state" colSpan={3}>
                  <div className="az-empty" role="status">Loading subscriptions…</div>
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td className="az-table-state" colSpan={3}>
                  <div className="az-empty">
                    <strong>No subscriptions to display</strong>
                    <p>Subscriptions this directory can reach appear here.</p>
                  </div>
                </td>
              </tr>
            ) : (
              rows.map((row: Subscription) => (
                <tr key={row.subscriptionId} data-testid="subs-row">
                  <td>
                    <Link to={`/ui/subscriptions/${row.subscriptionId}`}>{row.displayName}</Link>
                  </td>
                  <td>
                    <code>{row.subscriptionId}</code>
                  </td>
                  <td>{row.state === "Enabled" ? "Active" : row.state === "Disabled" ? "Disabled" : row.state}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </>
  );
}
