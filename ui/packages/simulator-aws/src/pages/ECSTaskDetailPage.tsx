import { Fragment, useMemo, useState } from "react";
import { useParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AwsButton, AwsContainer, AwsPageHeader, AwsStatus, AwsTabs, type AwsTab } from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import { fetchECSTaskDefinition, fetchECSTaskDetail, type ECSTask } from "../api.js";
import { isStoppable, StopTasksModal } from "./ECSTasksPage.js";

// Amazon Elastic Container Service — Task detail. Reads the real ECS
// awsjson1.1 API (DescribeTasks for the task, DescribeTaskDefinition for the
// task definition it runs) with the operator's federated credentials, and
// reuses the Tasks list page's real StopTask action.

function shortArn(arn: string): string {
  const slash = arn.lastIndexOf("/");
  return slash === -1 ? arn : arn.slice(slash + 1);
}

export function ECSTaskDetailPage() {
  const { taskArn = "" } = useParams();
  const queryClient = useQueryClient();
  const [stopping, setStopping] = useState(false);

  const task = useQuery({ queryKey: ["ecs-task", taskArn], queryFn: () => fetchECSTaskDetail(taskArn) });
  const taskDefinition = useQuery({
    queryKey: ["ecs-task-definition", task.data?.taskDefinitionArn],
    queryFn: () => fetchECSTaskDefinition(task.data!.taskDefinitionArn),
    enabled: Boolean(task.data?.taskDefinitionArn),
  });

  const containersWithImage = useMemo(() => {
    const images = new Map((taskDefinition.data?.containerDefinitions ?? []).map((c) => [c.name, c.image]));
    return (task.data?.containers ?? []).map((container) => ({
      ...container,
      image: images.get(container.name) || container.image,
    }));
  }, [task.data, taskDefinition.data]);

  const asECSTask: ECSTask | null = task.data
    ? {
        taskArn: task.data.taskArn,
        status: task.data.status,
        clusterArn: task.data.clusterArn,
        launchType: task.data.launchType,
        cpu: task.data.cpu,
        memory: task.data.memory,
      }
    : null;

  return (
    <>
      <AwsPageHeader
        title={taskArn}
        description="Task in Amazon Elastic Container Service."
        actions={
          <AwsButton
            data-testid="ecs-task-stop"
            disabled={!asECSTask || !isStoppable(asECSTask)}
            onClick={() => setStopping(true)}
          >
            Stop
          </AwsButton>
        }
      />
      <AwsContainer>
        {task.isError ? (
          <div className="aws-flash aws-flash-error" role="alert" data-testid="ecs-task-error">
            <strong>Could not load the task.</strong>{" "}
            {task.error instanceof Error ? task.error.message : "The request failed."}
          </div>
        ) : task.isLoading ? (
          <div className="aws-empty" role="status">
            Loading task…
          </div>
        ) : task.data ? (
          <>
            <dl className="aws-key-value" data-testid="ecs-task-summary">
              <dt>Last status</dt>
              <dd>
                <AwsStatus status={task.data.status} />
              </dd>
              <dt>Desired status</dt>
              <dd>{task.data.desiredStatus || "–"}</dd>
              <dt>Cluster</dt>
              <dd>{shortArn(task.data.clusterArn) || "–"}</dd>
              <dt>Launch type</dt>
              <dd>{task.data.launchType || "–"}</dd>
              <dt>Connectivity</dt>
              <dd>{task.data.connectivity || "–"}</dd>
              <dt>CPU</dt>
              <dd>{task.data.cpu || "–"}</dd>
              <dt>Memory</dt>
              <dd>{task.data.memory || "–"}</dd>
              <dt>Task definition</dt>
              <dd>{taskDefinition.data ? `${taskDefinition.data.family}:${taskDefinition.data.revision}` : "–"}</dd>
              <dt>Created</dt>
              <dd>{task.data.createdAt ? formatEpoch(task.data.createdAt) : "–"}</dd>
              <dt>Started</dt>
              <dd>{task.data.startedAt ? formatEpoch(task.data.startedAt) : "–"}</dd>
              {task.data.stoppedAt ? (
                <>
                  <dt>Stopped</dt>
                  <dd>{formatEpoch(task.data.stoppedAt)}</dd>
                  <dt>Stop reason</dt>
                  <dd>{task.data.stoppedReason || task.data.stopCode || "–"}</dd>
                </>
              ) : null}
            </dl>
            <AwsTabs
              ariaLabel="Task detail"
              tabs={[
                {
                  id: "containers",
                  label: "Containers",
                  content: (
                    <table className="aws-table" aria-label="Containers" data-testid="ecs-task-containers">
                      <thead>
                        <tr>
                          <th scope="col">Container name</th>
                          <th scope="col">Image</th>
                          <th scope="col">Status</th>
                          <th scope="col">Exit code</th>
                          <th scope="col">Private IP</th>
                        </tr>
                      </thead>
                      <tbody>
                        {containersWithImage.length === 0 ? (
                          <tr>
                            <td className="aws-table-state" colSpan={5}>
                              <div className="aws-empty">
                                <strong>No containers</strong>
                                <p>This task reports no containers.</p>
                              </div>
                            </td>
                          </tr>
                        ) : (
                          containersWithImage.map((container) => (
                            <tr key={container.name}>
                              <td>{container.name}</td>
                              <td>{container.image || "–"}</td>
                              <td>
                                <AwsStatus status={container.lastStatus} />
                              </td>
                              <td>{container.exitCode ?? "–"}</td>
                              <td>{container.privateIpv4Address ?? "–"}</td>
                            </tr>
                          ))
                        )}
                      </tbody>
                    </table>
                  ),
                },
                {
                  id: "network",
                  label: "Network",
                  content: (
                    <div data-testid="ecs-task-network">
                      {task.data.networkConfiguration && (
                        <dl className="aws-key-value">
                          <dt>Subnets</dt>
                          <dd>{task.data.networkConfiguration.subnets.join(", ") || "–"}</dd>
                          <dt>Security groups</dt>
                          <dd>{task.data.networkConfiguration.securityGroups.join(", ") || "–"}</dd>
                          <dt>Assign public IP</dt>
                          <dd>{task.data.networkConfiguration.assignPublicIp || "–"}</dd>
                        </dl>
                      )}
                      {task.data.attachments.length === 0 ? (
                        <div className="aws-empty">
                          <strong>No network attachments</strong>
                          <p>This task has no elastic network interface attachments.</p>
                        </div>
                      ) : (
                        task.data.attachments.map((attachment) => (
                          <div key={attachment.id} className="aws-detail-section">
                            <h3 className="aws-subheading">{attachment.type || "Attachment"}</h3>
                            <dl className="aws-key-value">
                              <dt>Status</dt>
                              <dd>
                                <AwsStatus status={attachment.status} />
                              </dd>
                              {attachment.details.map((detail) => (
                                <Fragment key={detail.name}>
                                  <dt>{detail.name}</dt>
                                  <dd>{detail.value}</dd>
                                </Fragment>
                              ))}
                            </dl>
                          </div>
                        ))
                      )}
                    </div>
                  ),
                },
                {
                  id: "task-definition",
                  label: "Task definition",
                  content: taskDefinition.isError ? (
                    <div className="aws-flash aws-flash-error" role="alert">
                      <strong>Could not load the task definition.</strong>{" "}
                      {taskDefinition.error instanceof Error ? taskDefinition.error.message : "The request failed."}
                    </div>
                  ) : taskDefinition.isLoading || !taskDefinition.data ? (
                    <div className="aws-empty" role="status">
                      Loading task definition…
                    </div>
                  ) : (
                    <div data-testid="ecs-task-definition">
                      <dl className="aws-key-value">
                        <dt>Family : revision</dt>
                        <dd>
                          {taskDefinition.data.family}:{taskDefinition.data.revision}
                        </dd>
                        <dt>Network mode</dt>
                        <dd>{taskDefinition.data.networkMode || "–"}</dd>
                        <dt>Requires compatibilities</dt>
                        <dd>{taskDefinition.data.requiresCompatibilities.join(", ") || "–"}</dd>
                        <dt>Task CPU</dt>
                        <dd>{taskDefinition.data.cpu || "–"}</dd>
                        <dt>Task memory</dt>
                        <dd>{taskDefinition.data.memory || "–"}</dd>
                        <dt>Task role</dt>
                        <dd>{taskDefinition.data.taskRoleArn || "–"}</dd>
                        <dt>Execution role</dt>
                        <dd>{taskDefinition.data.executionRoleArn || "–"}</dd>
                      </dl>
                      {taskDefinition.data.containerDefinitions.map((container) => (
                        <div key={container.name} className="aws-detail-section">
                          <h3 className="aws-subheading">{container.name}</h3>
                          <dl className="aws-key-value">
                            <dt>Image</dt>
                            <dd>{container.image}</dd>
                            <dt>Essential</dt>
                            <dd>{container.essential ? "Yes" : "No"}</dd>
                            {container.cpu !== undefined && (
                              <>
                                <dt>CPU units</dt>
                                <dd>{container.cpu}</dd>
                              </>
                            )}
                            {container.memory !== undefined && (
                              <>
                                <dt>Memory hard limit</dt>
                                <dd>{container.memory} MiB</dd>
                              </>
                            )}
                            {container.memoryReservation !== undefined && (
                              <>
                                <dt>Memory soft limit</dt>
                                <dd>{container.memoryReservation} MiB</dd>
                              </>
                            )}
                            {container.entryPoint.length > 0 && (
                              <>
                                <dt>Entry point</dt>
                                <dd>
                                  <code>{container.entryPoint.join(" ")}</code>
                                </dd>
                              </>
                            )}
                            {container.command.length > 0 && (
                              <>
                                <dt>Command</dt>
                                <dd>
                                  <code>{container.command.join(" ")}</code>
                                </dd>
                              </>
                            )}
                            {container.portMappings.length > 0 && (
                              <>
                                <dt>Port mappings</dt>
                                <dd>
                                  {container.portMappings
                                    .map((mapping) =>
                                      mapping.hostPort
                                        ? `${mapping.hostPort}:${mapping.containerPort}/${mapping.protocol ?? "tcp"}`
                                        : `${mapping.containerPort}/${mapping.protocol ?? "tcp"}`,
                                    )
                                    .join(", ")}
                                </dd>
                              </>
                            )}
                            {container.logDriver && (
                              <>
                                <dt>Log driver</dt>
                                <dd>{container.logDriver}</dd>
                              </>
                            )}
                            {container.environment.length > 0 && (
                              <>
                                <dt>Environment variables</dt>
                                <dd>
                                  <ul>
                                    {container.environment.map((entry) => (
                                      <li key={entry.name}>
                                        <code>
                                          {entry.name}={entry.value}
                                        </code>
                                      </li>
                                    ))}
                                  </ul>
                                </dd>
                              </>
                            )}
                          </dl>
                        </div>
                      ))}
                    </div>
                  ),
                },
              ]}
            />
          </>
        ) : null}
      </AwsContainer>
      {stopping && task.data && asECSTask && (
        <StopTasksModal
          tasks={[asECSTask]}
          clearSelection={() => {}}
          onClose={() => {
            setStopping(false);
            void queryClient.invalidateQueries({ queryKey: ["ecs-task", taskArn] });
          }}
        />
      )}
    </>
  );
}
