import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { AwsButton, AwsContainer, AwsCopyButton, AwsPageHeader, AwsStatus } from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import { fetchOrgAccount } from "../api.js";
import { AccountActionModal, orgAccountStatusKind } from "./OrganizationsPage.js";

// An AWS Organizations member account, read with DescribeAccount — the detail
// the real console opens from the accounts list, with the same two actions:
// Remove from organization and Close.

export function OrgAccountDetailPage() {
  const { accountId = "" } = useParams();
  const navigate = useNavigate();
  const account = useQuery({
    queryKey: ["org-account", accountId],
    queryFn: () => fetchOrgAccount(accountId),
  });
  const [action, setAction] = useState<"remove" | "close" | null>(null);

  return (
    <>
      <AwsPageHeader
        title={account.data?.name ?? accountId}
        description="Member account in AWS Organizations."
        actions={
          <>
            <AwsButton
              data-testid="org-remove-account"
              disabled={!account.isSuccess}
              onClick={() => setAction("remove")}
            >
              Remove from organization
            </AwsButton>
            <AwsButton data-testid="org-close-account" disabled={!account.isSuccess} onClick={() => setAction("close")}>
              Close account
            </AwsButton>
          </>
        }
      />
      <AwsContainer>
        {account.isError ? (
          <div className="aws-flash aws-flash-error" role="alert" data-testid="org-error">
            <strong>Could not load the account.</strong>{" "}
            {account.error instanceof Error ? account.error.message : "The request failed."}
          </div>
        ) : account.isLoading ? (
          <div className="aws-empty" role="status">
            Loading account…
          </div>
        ) : account.data ? (
          <dl className="aws-key-value" data-testid="org-account-detail">
            <dt>Account ID</dt>
            <dd>
              <code>{account.data.id}</code>
              <AwsCopyButton value={account.data.id} label="Copy account ID" />
            </dd>
            <dt>ARN</dt>
            <dd>
              <code>{account.data.arn}</code>
              <AwsCopyButton value={account.data.arn} label="Copy account ARN" />
            </dd>
            <dt>Account name</dt>
            <dd>{account.data.name}</dd>
            <dt>Email</dt>
            <dd>{account.data.email}</dd>
            <dt>Status</dt>
            <dd>
              <AwsStatus status={account.data.status} kind={orgAccountStatusKind(account.data.status)} />
            </dd>
            <dt>Joined method</dt>
            <dd>{account.data.joinedMethod}</dd>
            <dt>Joined</dt>
            <dd>{formatEpoch(account.data.joinedTimestamp)}</dd>
          </dl>
        ) : null}
      </AwsContainer>
      {action && account.data && (
        <AccountActionModal
          action={action}
          accounts={[{ id: account.data.id, name: account.data.name }]}
          clearSelection={() => navigate("/ui/organizations")}
          onClose={() => setAction(null)}
        />
      )}
    </>
  );
}
