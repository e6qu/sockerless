import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AwsButton, AwsErrorAlert, AwsModal, AwsResourceTable, AwsStatus, type AwsColumn } from "../console/index.js";
import {
  deleteEventBridgeRule,
  fetchEventBridgeRules,
  fetchEventBuses,
  setEventBridgeRuleState,
  type EventBridgeRule,
  type EventBus,
} from "../api.js";

// Amazon EventBridge — Rules and Event buses. ListRules, EnableRule,
// DisableRule, DeleteRule, and ListEventBuses on the real EventBridge API
// (X-Amz-Target AWSEvents.<Op>).

const ruleColumns: AwsColumn<EventBridgeRule>[] = [
  { id: "name", header: "Name", cell: (row) => row.name, value: (row) => row.name },
  { id: "state", header: "Status", cell: (row) => <AwsStatus status={row.state} />, value: (row) => row.state },
  { id: "eventBusName", header: "Event bus", cell: (row) => row.eventBusName, value: (row) => row.eventBusName },
  {
    id: "scheduleExpression",
    header: "Schedule",
    cell: (row) => row.scheduleExpression || "–",
    value: (row) => row.scheduleExpression,
  },
  { id: "description", header: "Description", cell: (row) => row.description || "–", value: (row) => row.description },
];

const busColumns: AwsColumn<EventBus>[] = [
  { id: "name", header: "Name", cell: (row) => row.name, value: (row) => row.name },
  { id: "arn", header: "ARN", cell: (row) => row.arn, value: (row) => row.arn },
  { id: "description", header: "Description", cell: (row) => row.description || "–", value: (row) => row.description },
];

function DeleteRulesModal({
  rules,
  onClose,
  clearSelection,
}: {
  rules: EventBridgeRule[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: async () => {
      for (const rule of rules) {
        await deleteEventBridgeRule(rule.name);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["eventbridge-rules"] }),
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  return (
    <AwsModal
      title={rules.length === 1 ? `Delete ${rules[0].name}?` : `Delete ${rules.length} rules?`}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="eventbridge-delete-rule-confirm"
            disabled={remove.isPending}
            onClick={() => remove.mutate()}
          >
            {remove.isPending ? "Deleting…" : "Delete"}
          </AwsButton>
        </>
      }
    >
      <p>A rule with targets still attached must have them removed before EventBridge will delete it.</p>
      <ul>
        {rules.map((rule) => (
          <li key={rule.name}>
            <code>{rule.name}</code>
          </li>
        ))}
      </ul>
      {remove.isError && (
        <AwsErrorAlert>
          <strong>Could not delete.</strong>{" "}
          {remove.error instanceof Error ? remove.error.message : "The request failed."}
        </AwsErrorAlert>
      )}
    </AwsModal>
  );
}

export function EventBridgePage() {
  const queryClient = useQueryClient();
  const [deleting, setDeleting] = useState<{ rules: EventBridgeRule[]; clearSelection: () => void } | null>(null);
  const setState = useMutation({
    mutationFn: async ({ rules, enabled }: { rules: EventBridgeRule[]; enabled: boolean }) => {
      for (const rule of rules) {
        await setEventBridgeRuleState(rule.name, enabled);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["eventbridge-rules"] }),
  });
  return (
    <>
      <AwsResourceTable<EventBridgeRule>
        title="Rules"
        description="EventBridge rules in this account and Region."
        columns={ruleColumns}
        queryKey={["eventbridge-rules"]}
        queryFn={fetchEventBridgeRules}
        filterPlaceholder="Find rules"
        emptyTitle="No rules"
        emptyDescription="No EventBridge rules exist in this account and Region."
        rowKey={(row) => row.arn || row.name}
        tableTestId="eventbridge-rules-table"
        errorTestId="eventbridge-rules-error"
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="eventbridge-enable-rule"
              disabled={selected.length === 0 || setState.isPending}
              onClick={() => setState.mutate({ rules: selected, enabled: true })}
            >
              Enable
            </AwsButton>
            <AwsButton
              data-testid="eventbridge-disable-rule"
              disabled={selected.length === 0 || setState.isPending}
              onClick={() => setState.mutate({ rules: selected, enabled: false })}
            >
              Disable
            </AwsButton>
            <AwsButton
              data-testid="eventbridge-delete-rule"
              disabled={selected.length === 0}
              onClick={() => setDeleting({ rules: selected, clearSelection })}
            >
              Delete
            </AwsButton>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
          </>
        )}
      />
      {setState.isError && (
        <AwsErrorAlert testId="eventbridge-rule-state-error">
          <strong>Could not change the rule state.</strong>{" "}
          {setState.error instanceof Error ? setState.error.message : "The request failed."}
        </AwsErrorAlert>
      )}
      <AwsResourceTable<EventBus>
        title="Event buses"
        headingVariant="h2"
        description="The event buses rules match events on."
        columns={busColumns}
        queryKey={["eventbridge-buses"]}
        queryFn={fetchEventBuses}
        filterPlaceholder="Find event buses"
        emptyTitle="No event buses"
        emptyDescription="No EventBridge event buses exist in this account and Region."
        rowKey={(row) => row.arn || row.name}
        tableTestId="eventbridge-buses-table"
        errorTestId="eventbridge-buses-error"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
      {deleting && (
        <DeleteRulesModal rules={deleting.rules} clearSelection={deleting.clearSelection} onClose={() => setDeleting(null)} />
      )}
    </>
  );
}
