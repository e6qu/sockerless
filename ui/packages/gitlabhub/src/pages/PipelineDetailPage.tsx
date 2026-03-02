import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { StatusBadge, Spinner, LogViewer } from "@sockerless/ui-core/components";
import { fetchPipelineDetail, fetchPipelineLogs } from "../api.js";
import type { GitlabhubPipelineJob } from "../types.js";

export function PipelineDetailPage() {
  const { id } = useParams<{ id: string }>();
  const pipelineId = Number(id);
  const { data: pl, isLoading } = useQuery({
    queryKey: ["pipeline", pipelineId],
    queryFn: () => fetchPipelineDetail(pipelineId),
    enabled: !!id,
    refetchInterval: 3000,
  });
  const { data: logs } = useQuery({
    queryKey: ["pipeline-logs", pipelineId],
    queryFn: () => fetchPipelineLogs(pipelineId),
    enabled: !!id,
    refetchInterval: 5000,
  });

  if (isLoading || !pl) return <Spinner />;

  const allJobs = Object.values(pl.jobs);

  // Group jobs by stage, ordered by pipeline.stages
  const stages = pl.stages ?? [];
  const jobsByStage: Record<string, GitlabhubPipelineJob[]> = {};
  for (const stage of stages) {
    jobsByStage[stage] = [];
  }
  for (const job of allJobs) {
    if (!jobsByStage[job.stage]) {
      jobsByStage[job.stage] = [];
    }
    jobsByStage[job.stage].push(job);
  }
  // Sort jobs within each stage by name
  for (const jobs of Object.values(jobsByStage)) {
    jobs.sort((a, b) => a.name.localeCompare(b.name));
  }

  // Flat ordered list for logs section
  const orderedJobs = stages.flatMap((stage) => jobsByStage[stage] ?? []);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">Pipeline #{pl.id}</h2>
        <div className="mt-2 flex flex-wrap items-center gap-3 text-sm text-gray-500 dark:text-gray-400">
          <span>{pl.project_name}</span>
          <StatusBadge status={pl.status} />
          {pl.result && <StatusBadge status={pl.result} />}
          <span>Ref: {pl.ref}</span>
          {pl.sha && <span className="font-mono">{pl.sha.slice(0, 8)}</span>}
          <span>{new Date(pl.created_at).toLocaleString()}</span>
        </div>
      </div>

      <h3 className="text-lg font-medium">Stages</h3>
      <div className="flex gap-4 overflow-x-auto pb-2">
        {stages.map((stage) => (
          <div key={stage} className="min-w-[180px] rounded-lg border border-gray-200 dark:border-gray-700">
            <div className="border-b border-gray-200 bg-gray-50 px-3 py-2 text-sm font-medium dark:border-gray-700 dark:bg-gray-800">
              {stage}
            </div>
            <div className="space-y-1 p-2">
              {(jobsByStage[stage] ?? []).map((job) => (
                <div key={job.name} className="flex items-center gap-2 rounded px-2 py-1 text-sm hover:bg-gray-50 dark:hover:bg-gray-800">
                  <StatusBadge status={job.status} />
                  <span>{job.name}</span>
                  {job.allow_failure && (
                    <span className="rounded bg-yellow-100 px-1 text-xs text-yellow-700 dark:bg-yellow-900 dark:text-yellow-300">
                      allow failure
                    </span>
                  )}
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>

      {logs && Object.keys(logs).length > 0 && (
        <div className="space-y-4">
          <h3 className="text-lg font-medium">Logs</h3>
          {orderedJobs.map((job) => {
            const jobLogs = logs[job.name];
            if (!jobLogs || jobLogs.length === 0) return null;
            return (
              <div key={job.name}>
                <p className="mb-1 text-sm font-medium">{job.name}</p>
                <LogViewer lines={jobLogs} />
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
