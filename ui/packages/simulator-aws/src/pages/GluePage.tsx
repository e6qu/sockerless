import { AwsButton, AwsResourceTable, type AwsColumn } from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import { fetchGlueDatabases, fetchGlueJobs, type GlueDatabase, type GlueJob } from "../api.js";

// AWS Glue — Data Catalog databases and ETL jobs, the two resources the real
// Glue console leads with. GetDatabases and GetJobs on the real Glue API
// (X-Amz-Target AWSGlue.<Op>).

const databaseColumns: AwsColumn<GlueDatabase>[] = [
  { id: "name", header: "Name", cell: (row) => row.name, value: (row) => row.name },
  { id: "description", header: "Description", cell: (row) => row.description || "–", value: (row) => row.description },
  { id: "locationUri", header: "Location", cell: (row) => row.locationUri || "–", value: (row) => row.locationUri },
  {
    id: "createTime",
    header: "Created",
    cell: (row) => formatEpoch(row.createTime),
    value: (row) => String(row.createTime),
  },
];

const jobColumns: AwsColumn<GlueJob>[] = [
  { id: "name", header: "Name", cell: (row) => row.name, value: (row) => row.name },
  { id: "role", header: "IAM role", cell: (row) => row.role || "–", value: (row) => row.role },
  { id: "glueVersion", header: "Glue version", cell: (row) => row.glueVersion || "–", value: (row) => row.glueVersion },
  { id: "workerType", header: "Worker type", cell: (row) => row.workerType || "–", value: (row) => row.workerType },
  {
    id: "scriptLocation",
    header: "Script location",
    cell: (row) => row.scriptLocation || "–",
    value: (row) => row.scriptLocation,
  },
  {
    id: "createdOn",
    header: "Created",
    cell: (row) => formatEpoch(row.createdOn),
    value: (row) => String(row.createdOn),
  },
];

export function GluePage() {
  return (
    <>
      <AwsResourceTable<GlueDatabase>
        title="Databases"
        description="AWS Glue Data Catalog databases in this account and Region."
        columns={databaseColumns}
        queryKey={["glue-databases"]}
        queryFn={fetchGlueDatabases}
        filterPlaceholder="Find databases"
        emptyTitle="No databases"
        emptyDescription="No AWS Glue databases exist in this account and Region."
        rowKey={(row) => row.name}
        tableTestId="glue-databases-table"
        errorTestId="glue-databases-error"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
      <AwsResourceTable<GlueJob>
        title="ETL jobs"
        headingVariant="h2"
        description="AWS Glue extract, transform, and load jobs."
        columns={jobColumns}
        queryKey={["glue-jobs"]}
        queryFn={fetchGlueJobs}
        filterPlaceholder="Find jobs"
        emptyTitle="No jobs"
        emptyDescription="No AWS Glue jobs exist in this account and Region."
        rowKey={(row) => row.name}
        tableTestId="glue-jobs-table"
        errorTestId="glue-jobs-error"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
    </>
  );
}
