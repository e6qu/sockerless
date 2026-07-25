import { useEffect, useState } from "react";
import { NavLink } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AwsButton,
  AwsContainer,
  AwsModal,
  AwsPageHeader,
  AwsResourceTable,
  AwsStatus,
  type AwsColumn,
  type AwsStatusKind,
} from "../console/index.js";
import { AwsApiError } from "../console/federation.js";
import { formatEpoch } from "../console/format.js";
import {
  closeOrgAccount,
  createOrgAccount,
  createOrganization,
  fetchOrgAccounts,
  fetchOrgCreateAccountStatus,
  fetchOrgCreateAccountStatuses,
  removeOrgAccount,
  type OrgAccount,
  type OrgCreateAccountStatus,
} from "../api.js";

// AWS Organizations — the AWS accounts page. Everything on it is the real
// Organizations wire (awsjson1.1, X-Amz-Target AWSOrganizationsV20161128.*)
// driven with the operator's federated credentials: ListAccounts for the
// table, the async CreateAccount request tracked through
// DescribeCreateAccountStatus, DescribeAccount for the detail page, and the
// two account actions the real console offers — Remove from organization and
// Close. When the caller's account is not in an organization the service
// answers AWSOrganizationsNotInUseException, and the page offers Create an
// organization, exactly as the real console does.

/** Organization account statuses carry fixed meanings, so the glyph is chosen
 * from the documented value rather than inferred from wording. */
export function orgAccountStatusKind(status: string): AwsStatusKind {
  switch (status) {
    case "ACTIVE":
      return "success";
    case "SUSPENDED":
      return "error";
    case "PENDING_CLOSURE":
      return "warning";
    default:
      return "inactive";
  }
}

const columns: AwsColumn<OrgAccount>[] = [
  {
    id: "name",
    header: "Account name",
    cell: (row) => <NavLink to={`/ui/organizations/accounts/${encodeURIComponent(row.id)}`}>{row.name}</NavLink>,
    value: (row) => row.name,
  },
  { id: "id", header: "Account ID", cell: (row) => <code>{row.id}</code>, value: (row) => row.id },
  { id: "email", header: "Email", cell: (row) => row.email, value: (row) => row.email },
  {
    id: "status",
    header: "Status",
    cell: (row) => <AwsStatus status={row.status} kind={orgAccountStatusKind(row.status)} />,
    value: (row) => row.status,
  },
  {
    id: "joined",
    header: "Joined",
    cell: (row) => formatEpoch(row.joinedTimestamp),
    value: (row) => String(row.joinedTimestamp),
  },
];

/**
 * The state of a CreateAccount request, rendered the way the real console's
 * request list renders it: the request id, the account it is creating, and a
 * status that resolves to the new account's id on success or the service's
 * failure reason on failure.
 */
export function CreateAccountStatusPanel({ status }: { status: OrgCreateAccountStatus }) {
  const kind: AwsStatusKind =
    status.state === "SUCCEEDED" ? "success" : status.state === "FAILED" ? "error" : "warning";
  return (
    <div data-testid="org-create-status">
      <dl className="aws-key-value">
        <dt>Request ID</dt>
        <dd>
          <code>{status.id}</code>
        </dd>
        <dt>Account name</dt>
        <dd>{status.accountName}</dd>
        <dt>Status</dt>
        <dd>
          <AwsStatus status={status.state} kind={kind} />
        </dd>
        {status.state === "SUCCEEDED" && (
          <>
            <dt>Account ID</dt>
            <dd>
              <code data-testid="org-created-account-id">{status.accountId}</code>
            </dd>
          </>
        )}
        <dt>Requested</dt>
        <dd>{formatEpoch(status.requestedTimestamp)}</dd>
        {status.completedTimestamp > 0 && (
          <>
            <dt>Completed</dt>
            <dd>{formatEpoch(status.completedTimestamp)}</dd>
          </>
        )}
      </dl>
      {status.state === "IN_PROGRESS" && (
        <p role="status">AWS Organizations is creating the account. This page updates when the request completes.</p>
      )}
      {status.state === "FAILED" && (
        <div className="aws-flash aws-flash-error" role="alert" data-testid="org-error">
          <strong>The account could not be created.</strong> {status.failureReason || "The request failed."}
        </div>
      )}
    </div>
  );
}

/**
 * What the real Organizations console shows an account that is not in an
 * organization: an explanation and Create an organization as the primary
 * action. Presentational — the caller owns the CreateOrganization mutation.
 */
export function OrganizationNotInUsePanel({
  onCreate,
  isPending,
  errorMessage,
}: {
  onCreate: () => void;
  isPending: boolean;
  errorMessage?: string;
}) {
  return (
    <>
      <AwsPageHeader
        title="AWS Organizations"
        description="Your account is not a member of an organization. Create an organization to centrally manage multiple AWS accounts."
        actions={
          <AwsButton variant="primary" data-testid="org-create-organization" disabled={isPending} onClick={onCreate}>
            {isPending ? "Creating…" : "Create an organization"}
          </AwsButton>
        }
      />
      <AwsContainer>
        <div className="aws-empty">
          <strong>No organization</strong>
          <p>
            An organization lets you create member accounts, group them into organizational units, and govern them with
            policies. Your account becomes the organization's management account.
          </p>
        </div>
        {errorMessage && (
          <div className="aws-flash aws-flash-error" role="alert" data-testid="org-error">
            <strong>Could not create the organization.</strong> {errorMessage}
          </div>
        )}
      </AwsContainer>
    </>
  );
}

// The constraints the real service enforces on CreateAccount: a 1–50 character
// printable account name, and the owner email address. Validating them inline
// is what the real console does before it lets the request go out.
const ACCOUNT_NAME_PATTERN = /^[ -~]{1,50}$/;
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function AddAccountModal({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const create = useMutation({
    mutationFn: () => createOrgAccount(name.trim(), email.trim()),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["org-create-statuses"] }),
  });

  // CreateAccount is asynchronous on the real service: the response is a
  // CreateAccountStatus, and the console polls DescribeCreateAccountStatus
  // until the request settles at SUCCEEDED or FAILED.
  const requestId = create.data?.id;
  const polled = useQuery({
    queryKey: ["org-create-status", requestId],
    queryFn: () => fetchOrgCreateAccountStatus(requestId!),
    enabled: Boolean(requestId),
    refetchInterval: (query) => {
      const state = query.state.data?.state;
      return state === "SUCCEEDED" || state === "FAILED" ? false : 1000;
    },
  });
  const status = polled.data ?? create.data;

  useEffect(() => {
    if (status?.state === "SUCCEEDED") {
      void queryClient.invalidateQueries({ queryKey: ["org-accounts"] });
    }
  }, [status?.state, queryClient]);

  const valid = ACCOUNT_NAME_PATTERN.test(name.trim()) && EMAIL_PATTERN.test(email.trim());

  if (create.isSuccess && status) {
    return (
      <AwsModal
        title="Add an AWS account"
        onDismiss={onClose}
        footer={
          <AwsButton variant="primary" data-testid="org-add-account-done" onClick={onClose}>
            Done
          </AwsButton>
        }
      >
        <CreateAccountStatusPanel status={status} />
      </AwsModal>
    );
  }

  return (
    <AwsModal
      title="Add an AWS account"
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="org-add-account-submit"
            disabled={!valid || create.isPending}
            onClick={() => create.mutate()}
          >
            {create.isPending ? "Creating…" : "Create AWS account"}
          </AwsButton>
        </>
      }
    >
      <p>
        AWS Organizations creates the account and places it in your organization's root. The request is asynchronous —
        its status is shown here until it succeeds or fails.
      </p>
      <label htmlFor="org-account-name">
        <strong>AWS account name</strong>
      </label>
      <input
        id="org-account-name"
        data-testid="org-account-name-input"
        className="aws-input"
        value={name}
        autoFocus
        onChange={(event) => setName(event.target.value)}
        aria-describedby="org-account-name-constraint"
      />
      <p id="org-account-name-constraint" className="aws-page-description">
        Up to 50 printable characters.
      </p>
      <label htmlFor="org-account-email">
        <strong>Email address of the account's owner</strong>
      </label>
      <input
        id="org-account-email"
        data-testid="org-account-email-input"
        className="aws-input"
        type="email"
        value={email}
        onChange={(event) => setEmail(event.target.value)}
      />
      {create.isError && (
        <div className="aws-flash aws-flash-error" role="alert" data-testid="org-error">
          <strong>Could not create the account.</strong>{" "}
          {create.error instanceof Error ? create.error.message : "The request failed."}
        </div>
      )}
    </AwsModal>
  );
}

/**
 * The two account actions the real console's Actions menu offers, as one
 * confirmation dialog each way: Remove from organization deletes the
 * membership; Close suspends the account (the service's own semantics).
 */
export function AccountActionModal({
  action,
  accounts,
  onClose,
  clearSelection,
}: {
  action: "remove" | "close";
  accounts: { id: string; name: string }[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const perform = action === "remove" ? removeOrgAccount : closeOrgAccount;
  const mutate = useMutation({
    // The action is per-account on the wire; a failure part-way (removing the
    // management account, say) surfaces as the real API error, with the
    // already-processed accounts reflected in the refreshed list.
    mutationFn: async () => {
      for (const account of accounts) {
        await perform(account.id);
      }
    },
    onSettled: async () => {
      await queryClient.invalidateQueries({ queryKey: ["org-accounts"] });
      await queryClient.invalidateQueries({ queryKey: ["org-account"] });
    },
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  const verb = action === "remove" ? "Remove" : "Close";
  const title =
    accounts.length === 1
      ? `${verb} ${accounts[0].name}?`
      : `${verb} ${accounts.length} accounts?`;
  return (
    <AwsModal
      title={title}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid={action === "remove" ? "org-remove-account-confirm" : "org-close-account-confirm"}
            disabled={mutate.isPending}
            onClick={() => mutate.mutate()}
          >
            {mutate.isPending ? `${verb === "Remove" ? "Removing" : "Closing"}…` : verb}
          </AwsButton>
        </>
      }
    >
      {action === "remove" ? (
        <p>
          Removing an account from the organization makes it a standalone account. The organization's policies no
          longer apply to it. The management account cannot be removed.
        </p>
      ) : (
        <p>
          Closing an account suspends it. A closed account remains visible in the organization with the SUSPENDED
          status until AWS permanently deletes it. The management account cannot be closed.
        </p>
      )}
      <ul>
        {accounts.map((account) => (
          <li key={account.id}>
            <code>{account.id}</code> {account.name}
          </li>
        ))}
      </ul>
      {mutate.isError && (
        <div className="aws-flash aws-flash-error" role="alert" data-testid="org-error">
          <strong>Could not {action} the account.</strong>{" "}
          {mutate.error instanceof Error ? mutate.error.message : "The request failed."}
        </div>
      )}
    </AwsModal>
  );
}

/**
 * The real accounts page surfaces account creation requests that are still in
 * progress or that failed; settled successful requests live in the accounts
 * table itself. The read is gated on the accounts list having loaded so an
 * unauthorized or organization-less state reports one error, not two.
 */
function CreationRequests({ enabled }: { enabled: boolean }) {
  const requests = useQuery({
    queryKey: ["org-create-statuses"],
    queryFn: fetchOrgCreateAccountStatuses,
    enabled,
  });
  const open = (requests.data ?? []).filter((request) => request.state !== "SUCCEEDED");
  if (requests.isError) {
    return (
      <AwsContainer>
        <div className="aws-flash aws-flash-error" role="alert">
          <strong>Could not load account creation requests.</strong>{" "}
          {requests.error instanceof Error ? requests.error.message : "The request failed."}
        </div>
      </AwsContainer>
    );
  }
  if (open.length === 0) return null;
  return (
    <AwsContainer>
      <h2>Account creation requests</h2>
      {open.map((request) => (
        <CreateAccountStatusPanel key={request.id} status={request} />
      ))}
    </AwsContainer>
  );
}

export function OrganizationsPage() {
  const queryClient = useQueryClient();
  const accounts = useQuery({ queryKey: ["org-accounts"], queryFn: fetchOrgAccounts });
  const [adding, setAdding] = useState(false);
  const [action, setAction] = useState<{
    kind: "remove" | "close";
    accounts: OrgAccount[];
    clearSelection: () => void;
  } | null>(null);
  const createOrg = useMutation({
    mutationFn: createOrganization,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["org-accounts"] }),
  });

  const notInUse =
    accounts.error instanceof AwsApiError && accounts.error.code === "AWSOrganizationsNotInUseException";
  if (notInUse) {
    return (
      <OrganizationNotInUsePanel
        onCreate={() => createOrg.mutate()}
        isPending={createOrg.isPending}
        errorMessage={
          createOrg.isError
            ? createOrg.error instanceof Error
              ? createOrg.error.message
              : "The request failed."
            : undefined
        }
      />
    );
  }

  return (
    <>
      <AwsResourceTable<OrgAccount>
        title="AWS accounts"
        description="The AWS accounts in your organization. Add an account to create a new member account in the organization's root."
        columns={columns}
        queryKey={["org-accounts"]}
        queryFn={fetchOrgAccounts}
        filterPlaceholder="Find AWS accounts"
        emptyTitle="No AWS accounts"
        emptyDescription="The organization has no member accounts. Add an AWS account to create one."
        rowKey={(row) => row.id}
        tableTestId="org-accounts-table"
        rowTestId={(row) => `org-account-row-${row.id}`}
        errorTestId="org-error"
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="org-remove-account"
              disabled={selected.length === 0}
              onClick={() => setAction({ kind: "remove", accounts: selected, clearSelection })}
            >
              Remove from organization
            </AwsButton>
            <AwsButton
              data-testid="org-close-account"
              disabled={selected.length === 0}
              onClick={() => setAction({ kind: "close", accounts: selected, clearSelection })}
            >
              Close
            </AwsButton>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
            <AwsButton variant="primary" data-testid="org-add-account" onClick={() => setAdding(true)}>
              Add an AWS account
            </AwsButton>
          </>
        )}
      />
      <CreationRequests enabled={accounts.isSuccess} />
      {adding && <AddAccountModal onClose={() => setAdding(false)} />}
      {action && (
        <AccountActionModal
          action={action.kind}
          accounts={action.accounts}
          clearSelection={action.clearSelection}
          onClose={() => setAction(null)}
        />
      )}
    </>
  );
}
