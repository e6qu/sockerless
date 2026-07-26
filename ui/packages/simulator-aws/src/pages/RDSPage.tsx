import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AwsButton, AwsErrorAlert, AwsModal, AwsResourceTable, AwsStatus, type AwsColumn } from "../console/index.js";
import { deleteRDSInstance, fetchRDSClusters, fetchRDSInstances, type RDSCluster, type RDSInstance } from "../api.js";

// Amazon Relational Database Service (RDS) — Databases. DescribeDBInstances and
// DescribeDBClusters for the tables, DeleteDBInstance for the delete action,
// all on the real RDS Query API (Version 2014-10-31).

const instanceColumns: AwsColumn<RDSInstance>[] = [
  {
    id: "identifier",
    header: "DB identifier",
    cell: (row) => row.dbInstanceIdentifier,
    value: (row) => row.dbInstanceIdentifier,
  },
  { id: "status", header: "Status", cell: (row) => <AwsStatus status={row.status} />, value: (row) => row.status },
  { id: "engine", header: "Engine", cell: (row) => row.engine, value: (row) => row.engine },
  { id: "engineVersion", header: "Engine version", cell: (row) => row.engineVersion, value: (row) => row.engineVersion },
  { id: "class", header: "Size", cell: (row) => row.dbInstanceClass, value: (row) => row.dbInstanceClass },
  {
    id: "endpoint",
    header: "Endpoint",
    cell: (row) => (row.endpointAddress ? `${row.endpointAddress}:${row.endpointPort}` : "–"),
    value: (row) => row.endpointAddress,
  },
  {
    id: "storage",
    header: "Storage",
    cell: (row) => `${row.allocatedStorage} GiB`,
    value: (row) => String(row.allocatedStorage),
  },
];

const clusterColumns: AwsColumn<RDSCluster>[] = [
  {
    id: "identifier",
    header: "DB identifier",
    cell: (row) => row.dbClusterIdentifier,
    value: (row) => row.dbClusterIdentifier,
  },
  { id: "status", header: "Status", cell: (row) => <AwsStatus status={row.status} />, value: (row) => row.status },
  { id: "engine", header: "Engine", cell: (row) => row.engine, value: (row) => row.engine },
  { id: "engineVersion", header: "Engine version", cell: (row) => row.engineVersion, value: (row) => row.engineVersion },
  { id: "endpoint", header: "Endpoint", cell: (row) => row.endpoint || "–", value: (row) => row.endpoint },
];

function DeleteInstancesModal({
  instances,
  onClose,
  clearSelection,
}: {
  instances: RDSInstance[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: async () => {
      for (const instance of instances) {
        await deleteRDSInstance(instance.dbInstanceIdentifier);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["rds-instances"] }),
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  return (
    <AwsModal
      title={instances.length === 1 ? `Delete ${instances[0].dbInstanceIdentifier}?` : `Delete ${instances.length} databases?`}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="rds-delete-instance-confirm"
            disabled={remove.isPending}
            onClick={() => remove.mutate()}
          >
            {remove.isPending ? "Deleting…" : "Delete"}
          </AwsButton>
        </>
      }
    >
      <p>The delete request skips the final snapshot, so the database and its data are gone for good.</p>
      <ul>
        {instances.map((instance) => (
          <li key={instance.dbInstanceIdentifier}>
            <code>{instance.dbInstanceIdentifier}</code>
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

export function RDSPage() {
  const [deleting, setDeleting] = useState<{ instances: RDSInstance[]; clearSelection: () => void } | null>(null);
  return (
    <>
      <AwsResourceTable<RDSInstance>
        title="DB instances"
        description="Amazon RDS database instances in this account and Region."
        columns={instanceColumns}
        queryKey={["rds-instances"]}
        queryFn={fetchRDSInstances}
        filterPlaceholder="Find databases"
        emptyTitle="No DB instances"
        emptyDescription="No Amazon RDS database instances exist in this account and Region."
        rowKey={(row) => row.dbInstanceIdentifier}
        tableTestId="rds-instances-table"
        errorTestId="rds-instances-error"
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="rds-delete-instance"
              disabled={selected.length === 0}
              onClick={() => setDeleting({ instances: selected, clearSelection })}
            >
              Delete
            </AwsButton>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
          </>
        )}
      />
      <AwsResourceTable<RDSCluster>
        title="DB clusters"
        headingVariant="h2"
        description="Amazon Aurora and Multi-AZ DB clusters in this account and Region."
        columns={clusterColumns}
        queryKey={["rds-clusters"]}
        queryFn={fetchRDSClusters}
        filterPlaceholder="Find clusters"
        emptyTitle="No DB clusters"
        emptyDescription="No Amazon RDS database clusters exist in this account and Region."
        rowKey={(row) => row.dbClusterIdentifier}
        tableTestId="rds-clusters-table"
        errorTestId="rds-clusters-error"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
      {deleting && (
        <DeleteInstancesModal
          instances={deleting.instances}
          clearSelection={deleting.clearSelection}
          onClose={() => setDeleting(null)}
        />
      )}
    </>
  );
}
