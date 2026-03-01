import { StatusBadge } from "./StatusBadge.js";
import type { StatusResponse } from "../api/types.js";

export interface BackendInfoCardProps {
  status: StatusResponse;
}

export function BackendInfoCard({ status }: BackendInfoCardProps) {
  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
      <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-3">Backend Details</h3>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
        <dt className="text-gray-500 dark:text-gray-400">Type</dt>
        <dd><StatusBadge status={status.backend_type} /></dd>
        <dt className="text-gray-500 dark:text-gray-400">Instance</dt>
        <dd className="font-mono text-xs">{status.instance_id}</dd>
        {status.context && (
          <>
            <dt className="text-gray-500 dark:text-gray-400">Context</dt>
            <dd>{status.context}</dd>
          </>
        )}
      </dl>
    </div>
  );
}
