import { useMemo, useState, type CSSProperties } from "react";
import { Link } from "react-router";
import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  Button,
  Modal,
  PageHeading,
  Spinner,
  StatusBadge,
  useReportError,
  useToast,
} from "@sockerless/ui-core/components";
import {
  AdminApiClient,
  type InstanceKind,
  type Topology,
  type TopologyInstance,
  type TopologyProject,
} from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";
import { ConfigEditModal } from "../components/ConfigEditModal.js";
import { InstanceForm } from "../components/InstanceForm.js";
import { ProjectForm } from "../components/ProjectForm.js";
import {
  UnhealthyDiagnosticPanel,
  shouldRender as shouldRenderDiagnostics,
} from "../components/UnhealthyDiagnosticPanel.js";

const api = new AdminApiClient();

const cardStyle: CSSProperties = {
  background: "var(--color-surface)",
  border: "1px solid var(--color-border)",
  borderRadius: "var(--radius-sm)",
};

interface PendingDelete {
  kind: "project" | "instance";
  project: string;
  instance?: string;
}

interface InstanceEditTarget {
  project: TopologyProject;
  instance?: TopologyInstance;
}

interface ConfigEditTarget {
  project: string;
  instance: TopologyInstance;
}

interface ActionError {
  kicker: string;
  message: string;
  commands: string[];
}

interface StackPreset {
  label: string;
  command: string;
  project: TopologyProject;
}

interface InstanceActionVars {
  project: string;
  inst: TopologyInstance;
  projectCfg?: TopologyProject;
}

const stackPresets: StackPreset[] = [
  {
    label: "Azure ACA",
    command: "make stack-azure-aca",
    project: {
      name: "local-azure-aca",
      instances: [
        { name: "sim-azure", kind: "sim", cloud: "azure", port: 4568 },
        {
          name: "aca",
          kind: "backend",
          cloud: "azure",
          backend: "aca",
          port: 3375,
          sim: "sim-azure",
          config: {
            SOCKERLESS_ACA_SUBSCRIPTION_ID:
              "00000000-0000-0000-0000-000000000000",
            SOCKERLESS_ACA_RESOURCE_GROUP: "sim-rg",
            SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE: "default",
            SOCKERLESS_CALLBACK_URL: "ws://localhost:3375/v1/aca/reverse",
          },
        },
      ],
    },
  },
  {
    label: "AWS ECS",
    command: "make stack-aws-ecs",
    project: {
      name: "local-aws-ecs",
      instances: [
        { name: "sim-aws", kind: "sim", cloud: "aws", port: 4566 },
        {
          name: "ecs",
          kind: "backend",
          cloud: "aws",
          backend: "ecs",
          port: 3375,
          sim: "sim-aws",
          config: {
            AWS_REGION: "us-east-1",
          },
        },
      ],
    },
  },
  {
    label: "GCP Cloud Run",
    command: "make stack-gcp-cloudrun",
    project: {
      name: "local-gcp-cloudrun",
      instances: [
        { name: "sim-gcp", kind: "sim", cloud: "gcp", port: 4567 },
        {
          name: "cloudrun",
          kind: "backend",
          cloud: "gcp",
          backend: "cloudrun",
          port: 3375,
          sim: "sim-gcp",
          config: {
            SOCKERLESS_GCR_PROJECT: "sim-project",
            SOCKERLESS_LOG_TIMEOUT: "2s",
          },
        },
      ],
    },
  },
];

function startCommand(inst: TopologyInstance, project?: TopologyProject): string {
  const args = [
    "make start-component",
    `KIND=${inst.kind}`,
    `NAME=${inst.name}`,
    `PORT=${inst.port}`,
  ];
  if (inst.cloud) args.push(`CLOUD=${inst.cloud}`);
  if (inst.backend) args.push(`BACKEND=${inst.backend}`);
  if (inst.kind === "backend" && inst.sim) {
    const sim = project?.instances?.find(
      (candidate) => candidate.name === inst.sim,
    );
    args.push(`SIM_PORT=${sim?.port ?? "<sim port>"}`);
  }
  return args.join(" ");
}

function stopCommand(inst: TopologyInstance): string {
  return `make stop-component NAME=${inst.name}`;
}

function rebuildCommand(inst: TopologyInstance): string {
  const args = ["make rebuild-component", `KIND=${inst.kind}`];
  if (inst.cloud) args.push(`CLOUD=${inst.cloud}`);
  if (inst.backend) args.push(`BACKEND=${inst.backend}`);
  return args.join(" ");
}

/**
 * TopologyPage — single-screen view of `sockerless.yaml`. Each project
 * expands into its instance list with per-row Start/Stop/Rebuild +
 * edit/delete + live status indicator. New projects + new instances are
 * added via the per-form modals (ProjectForm, InstanceForm).
 *
 * Backend wiring lives in the `/api/v1/topology/*` surface (Phase 79
 * step 7). This page is pure UI on top — no business logic in the
 * client beyond querying / posting / surfacing errors.
 */
export function TopologyPage() {
  const queryClient = useQueryClient();
  const { push } = useToast();
  const reportError = useReportError();

  const {
    data: topology,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["topology"],
    queryFn: () => api.topology(),
    refetchInterval: 5000,
  });

  const [projectFormOpen, setProjectFormOpen] = useState(false);
  const [instanceForm, setInstanceForm] = useState<InstanceEditTarget | null>(
    null,
  );
  const [configEdit, setConfigEdit] = useState<ConfigEditTarget | null>(null);
  const [pendingDelete, setPendingDelete] = useState<PendingDelete | null>(
    null,
  );
  const [actionError, setActionError] = useState<ActionError | null>(null);

  const { data: configMetadata } = useQuery({
    queryKey: ["config-metadata"],
    queryFn: () => api.topologyConfigMetadata(),
    staleTime: 60_000,
  });

  const { data: topologyFile } = useQuery({
    queryKey: ["topology-file"],
    queryFn: () => api.topologyFile(),
    staleTime: 60_000,
  });

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["topology"] });
    queryClient.invalidateQueries({ queryKey: ["topology-status"] });
  };

  const addProjectMutation = useMutation({
    mutationFn: (p: TopologyProject) => api.topologyAddProject(p),
    onSuccess: (p) => {
      invalidate();
      push({ tone: "success", title: `Project ${p.name} created` });
    },
    onError: (err, p) =>
      setActionError({
        kicker: "create project failed",
        message: err instanceof Error ? err.message : String(err),
        commands: [`edit ${topologyFile?.path ?? "sockerless.yaml"}`],
      }),
  });

  const removeProjectMutation = useMutation({
    mutationFn: (name: string) => api.topologyRemoveProject(name),
    onSuccess: (_d, name) => {
      invalidate();
      push({ tone: "success", title: `Project ${name} removed` });
    },
    onError: (err, name) => reportError(err, `Failed to remove ${name}`),
  });

  const addInstanceMutation = useMutation({
    mutationFn: ({
      project,
      inst,
    }: {
      project: string;
      inst: TopologyInstance;
    }) => api.topologyAddInstance(project, inst),
    onSuccess: (ref) => {
      invalidate();
      push({
        tone: "success",
        title: `Added ${ref.instance.name} to ${ref.project}`,
      });
    },
  });

  const updateInstanceMutation = useMutation({
    mutationFn: ({
      project,
      inst,
    }: {
      project: string;
      inst: TopologyInstance;
    }) => api.topologyUpdateInstance(project, inst),
    onSuccess: (ref) => {
      invalidate();
      push({ tone: "success", title: `Updated ${ref.instance.name}` });
    },
  });

  const removeInstanceMutation = useMutation({
    mutationFn: ({
      project,
      name,
    }: {
      project: string;
      name: string;
    }) => api.topologyRemoveInstance(project, name),
    onSuccess: (_d, vars) => {
      invalidate();
      push({ tone: "success", title: `Removed ${vars.name}` });
    },
    onError: (err, vars) =>
      reportError(err, `Failed to remove ${vars.name}`),
  });

  const startInstanceMutation = useMutation({
    mutationFn: ({ project, inst }: InstanceActionVars) =>
      api.topologyInstanceStart(project, inst.name),
    onSuccess: (_d, vars) => {
      invalidate();
      push({ tone: "success", title: `Started ${vars.inst.name}` });
    },
    onError: (err, vars) => {
      reportError(err, `Failed to start ${vars.inst.name}`);
      setActionError({
        kicker: "start failed",
        message: err instanceof Error ? err.message : String(err),
        commands: [
          "make stack-status",
          startCommand(vars.inst, vars.projectCfg),
        ],
      });
    },
  });

  const stopInstanceMutation = useMutation({
    mutationFn: ({ project, inst }: InstanceActionVars) =>
      api.topologyInstanceStop(project, inst.name),
    onSuccess: (_d, vars) => {
      invalidate();
      push({ tone: "success", title: `Stopped ${vars.inst.name}` });
    },
    onError: (err, vars) => {
      reportError(err, `Failed to stop ${vars.inst.name}`);
      setActionError({
        kicker: "stop failed",
        message: err instanceof Error ? err.message : String(err),
        commands: [stopCommand(vars.inst), "make stack-status"],
      });
    },
  });

  const restartInstanceMutation = useMutation({
    mutationFn: ({ project, inst }: InstanceActionVars) =>
      api.topologyInstanceRestart(project, inst.name),
    onSuccess: (_d, vars) => {
      invalidate();
      push({ tone: "success", title: `Restarted ${vars.inst.name}` });
    },
    onError: (err, vars) =>
      setActionError({
        kicker: "restart failed",
        message: err instanceof Error ? err.message : String(err),
        commands: [
          stopCommand(vars.inst),
          startCommand(vars.inst, vars.projectCfg),
        ],
      }),
  });

  const stopStackMutation = useMutation({
    mutationFn: () => api.topologyStopAll(),
    onSuccess: (resp) => {
      invalidate();
      push({
        tone: "success",
        title:
          resp.status === "stopping"
            ? "Stack shutdown scheduled"
            : `Stopped ${resp.stopped ?? 0} instance(s)`,
      });
    },
    onError: (err) =>
      setActionError({
        kicker: "stop stack failed",
        message: err instanceof Error ? err.message : String(err),
        commands: ["make stop-components", "make stack-down"],
      }),
  });

  const rebuildInstanceMutation = useMutation({
    mutationFn: ({ project, inst }: InstanceActionVars) =>
      api.topologyInstanceRebuild(project, inst.name),
    onSuccess: (_d, vars) => {
      invalidate();
      push({ tone: "success", title: `Rebuilt ${vars.inst.name}` });
    },
    onError: (err, vars) => {
      reportError(err, `Failed to rebuild ${vars.inst.name}`);
      setActionError({
        kicker: "rebuild failed",
        message: err instanceof Error ? err.message : String(err),
        commands: [rebuildCommand(vars.inst)],
      });
    },
  });

  const reloadInstanceMutation = useMutation({
    mutationFn: ({
      project,
      name,
    }: {
      project: string;
      name: string;
    }) => api.topologyInstanceReload(project, name),
    onSuccess: (_d, vars) => {
      invalidate();
      push({ tone: "success", title: `Reloaded ${vars.name}` });
    },
    onError: (err, vars) => reportError(err, `Failed to reload ${vars.name}`),
  });

  if (isLoading) return <Spinner label="loading topology" />;
  if (isError) {
    return (
      <ErrorPanel
        message={error?.message}
        commands={[
          "make cmd/sockerless-admin/run",
          "make stack-azure-aca",
          "make stack-status",
        ]}
      />
    );
  }
  if (!topology) return <Spinner label="loading topology" />;

  const projects = topology.projects ?? [];
  const totalInstances = projects.reduce(
    (acc, p) => acc + (p.instances?.length ?? 0),
    0,
  );

  return (
    <div className="flex flex-col gap-6">
      <PageHeading
        kicker="admin · topology"
        title={<>Topology</>}
        meta={`${projects.length} project${projects.length === 1 ? "" : "s"} · ${totalInstances} instance${totalInstances === 1 ? "" : "s"} · ${topologyFile?.path ?? "sockerless.yaml"}`}
        actions={
          <div className="inline-flex items-center gap-2">
            <Link
              to="/ui/topology/resources"
              style={{
                fontSize: "0.7rem",
                fontFamily: "var(--font-mono)",
                padding: "0.3rem 0.7rem",
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-xs)",
                color: "var(--color-fg-muted)",
                textDecoration: "none",
                letterSpacing: "0.05em",
              }}
            >
              cloud resources
            </Link>
            <Button
              variant="primary"
              size="sm"
              onClick={() => setProjectFormOpen(true)}
            >
              + project
            </Button>
            <Button
              variant="danger"
              size="sm"
              onClick={() => stopStackMutation.mutate()}
              disabled={stopStackMutation.isPending || totalInstances === 0}
            >
              stop stack
            </Button>
          </div>
        }
      />

      {actionError && (
        <ErrorPanel
          kicker={actionError.kicker}
          message={actionError.message}
          commands={actionError.commands}
        />
      )}

      <TopologyFileCard path={topologyFile?.path ?? "sockerless.yaml"} />

      {projects.length === 0 ? (
        <QuickStartCard
          presets={stackPresets}
          onCreate={(preset) => addProjectMutation.mutate(preset.project)}
        />
      ) : (
        <div className="flex flex-col gap-4">
          {projects.map((p) => (
            <ProjectCard
              key={p.name}
              project={p}
              onAddInstance={() =>
                setInstanceForm({ project: p, instance: undefined })
              }
              onEditInstance={(inst) =>
                setInstanceForm({ project: p, instance: inst })
              }
              onConfigEdit={(inst) =>
                setConfigEdit({ project: p.name, instance: inst })
              }
              onDeleteInstance={(inst) =>
                setPendingDelete({
                  kind: "instance",
                  project: p.name,
                  instance: inst.name,
                })
              }
              onDeleteProject={() =>
                setPendingDelete({ kind: "project", project: p.name })
              }
              onStart={(inst) =>
                startInstanceMutation.mutate({
                  project: p.name,
                  inst,
                  projectCfg: p,
                })
              }
              onStop={(inst) =>
                stopInstanceMutation.mutate({
                  project: p.name,
                  inst,
                  projectCfg: p,
                })
              }
              onRebuild={(inst) =>
                rebuildInstanceMutation.mutate({
                  project: p.name,
                  inst,
                  projectCfg: p,
                })
              }
              onRestart={(inst) =>
                restartInstanceMutation.mutate({
                  project: p.name,
                  inst,
                  projectCfg: p,
                })
              }
              busy={{
                start: startInstanceMutation,
                stop: stopInstanceMutation,
                restart: restartInstanceMutation,
                rebuild: rebuildInstanceMutation,
              }}
            />
          ))}
        </div>
      )}

      <PortRegistryCard topology={topology} />

      <ProjectForm
        open={projectFormOpen}
        onClose={() => setProjectFormOpen(false)}
        onSubmit={async (p) => {
          await addProjectMutation.mutateAsync(p);
        }}
      />

      {instanceForm && (
        <InstanceForm
          open={!!instanceForm}
          onClose={() => setInstanceForm(null)}
          project={instanceForm.project}
          editing={instanceForm.instance}
          onAllocatePort={async (kind: InstanceKind) => {
            const result = await api.topologyAllocatePort(kind);
            return result.port;
          }}
          onSubmit={async (inst) => {
            if (instanceForm.instance) {
              await updateInstanceMutation.mutateAsync({
                project: instanceForm.project.name,
                inst,
              });
            } else {
              await addInstanceMutation.mutateAsync({
                project: instanceForm.project.name,
                inst,
              });
            }
          }}
        />
      )}

      {configEdit && (
        <ConfigEditModal
          open={!!configEdit}
          onClose={() => setConfigEdit(null)}
          project={configEdit.project}
          instance={configEdit.instance}
          metadata={configMetadata?.keys ?? []}
          onReload={async (project, name) => {
            await reloadInstanceMutation.mutateAsync({ project, name });
          }}
          onRestart={async (project, name) => {
            const projectCfg = topology.projects?.find((p) => p.name === project);
            await restartInstanceMutation.mutateAsync({
              project,
              inst: configEdit.instance,
              projectCfg,
            });
          }}
        />
      )}

      <ConfirmDeleteModal
        pending={pendingDelete}
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => {
          if (!pendingDelete) return;
          if (pendingDelete.kind === "project") {
            removeProjectMutation.mutate(pendingDelete.project);
          } else if (pendingDelete.instance) {
            removeInstanceMutation.mutate({
              project: pendingDelete.project,
              name: pendingDelete.instance,
            });
          }
          setPendingDelete(null);
        }}
      />
    </div>
  );
}

interface BusyMutations {
  start: { isPending: boolean; variables?: InstanceActionVars };
  stop: { isPending: boolean; variables?: InstanceActionVars };
  restart: { isPending: boolean; variables?: InstanceActionVars };
  rebuild: { isPending: boolean; variables?: InstanceActionVars };
}

function TopologyFileCard({ path }: { path: string }) {
  return (
    <section className="grid gap-4 md:grid-cols-[minmax(0,1fr)_auto]" style={cardStyle}>
      <div className="px-4 py-3">
        <div
          className="font-mono uppercase tracking-[0.18em]"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.62rem" }}
        >
          topology file
        </div>
        <div
          className="mt-1 font-mono"
          style={{ color: "var(--color-fg)", fontSize: "0.86rem" }}
        >
          {path}
        </div>
      </div>
      <div
        className="flex flex-col gap-1 px-4 py-3"
        style={{ borderLeft: "1px solid var(--color-border)" }}
      >
        {["make stack-status", "make stack-down"].map((cmd) => (
          <code
            key={cmd}
            style={{
              background: "var(--color-bg)",
              border: "1px solid var(--color-border)",
              borderRadius: "var(--radius-xs)",
              color: "var(--color-fg-muted)",
              fontSize: "0.72rem",
              padding: "0.25rem 0.45rem",
            }}
          >
            {cmd}
          </code>
        ))}
      </div>
    </section>
  );
}

function QuickStartCard({
  presets,
  onCreate,
}: {
  presets: StackPreset[];
  onCreate: (preset: StackPreset) => void;
}) {
  return (
    <section style={cardStyle}>
      <header
        className="px-4 py-3"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <div
          className="font-display"
          style={{
            fontStyle: "italic",
            fontWeight: 600,
            fontSize: "1rem",
            letterSpacing: "-0.02em",
          }}
        >
          No projects configured
        </div>
        <div
          className="mt-0.5 font-mono uppercase tracking-[0.18em]"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.62rem" }}
        >
          create topology entries or run the equivalent stack command
        </div>
      </header>
      <div className="grid gap-3 p-4 md:grid-cols-3">
        {presets.map((preset) => (
          <div
            key={preset.label}
            className="flex flex-col gap-3 p-3"
            style={{
              border: "1px solid var(--color-border)",
              borderRadius: "var(--radius-sm)",
            }}
          >
            <div
              className="font-mono uppercase tracking-[0.16em]"
              style={{ color: "var(--color-fg)", fontSize: "0.7rem" }}
            >
              {preset.label}
            </div>
            <code
              style={{
                background: "var(--color-bg)",
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-xs)",
                color: "var(--color-fg-muted)",
                fontSize: "0.72rem",
                padding: "0.35rem 0.5rem",
                whiteSpace: "pre-wrap",
              }}
            >
              {preset.command}
            </code>
            <Button variant="primary" size="sm" onClick={() => onCreate(preset)}>
              create
            </Button>
          </div>
        ))}
      </div>
    </section>
  );
}

function ProjectCard({
  project,
  onAddInstance,
  onEditInstance,
  onConfigEdit,
  onDeleteInstance,
  onDeleteProject,
  onStart,
  onStop,
  onRestart,
  onRebuild,
  busy,
}: {
  project: TopologyProject;
  onAddInstance: () => void;
  onEditInstance: (inst: TopologyInstance) => void;
  onConfigEdit: (inst: TopologyInstance) => void;
  onDeleteInstance: (inst: TopologyInstance) => void;
  onDeleteProject: () => void;
  onStart: (inst: TopologyInstance) => void;
  onStop: (inst: TopologyInstance) => void;
  onRestart: (inst: TopologyInstance) => void;
  onRebuild: (inst: TopologyInstance) => void;
  busy: BusyMutations;
}) {
  const instances = project.instances ?? [];
  return (
    <section style={cardStyle}>
      <header
        className="flex items-center justify-between px-4 py-3"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <div>
          <div
            className="font-display"
            style={{
              fontStyle: "italic",
              fontWeight: 600,
              fontSize: "1.1rem",
              letterSpacing: "-0.02em",
            }}
          >
            {project.name}
          </div>
          <div
            className="mt-0.5 font-mono uppercase tracking-[0.18em]"
            style={{ color: "var(--color-fg-subtle)", fontSize: "0.65rem" }}
          >
            {instances.length} instance{instances.length === 1 ? "" : "s"}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Link
            to={`/ui/topology/${encodeURIComponent(project.name)}/console`}
            style={{
              fontSize: "0.7rem",
              fontFamily: "var(--font-mono)",
              padding: "0.25rem 0.6rem",
              border: "1px solid var(--color-border)",
              borderRadius: "var(--radius-xs)",
              color: "var(--color-fg-muted)",
              textDecoration: "none",
              letterSpacing: "0.05em",
            }}
          >
            console
          </Link>
          <Button variant="secondary" size="sm" onClick={onAddInstance}>
            + instance
          </Button>
          <Button variant="danger" size="sm" onClick={onDeleteProject}>
            delete project
          </Button>
        </div>
      </header>

      {instances.length === 0 ? (
        <div
          className="px-4 py-4 font-mono uppercase tracking-[0.18em] text-center"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.65rem" }}
        >
          — no instances —
        </div>
      ) : (
        <ul className="divide-y" style={{ borderColor: "var(--color-border)" }}>
          {instances.map((inst) => (
            <InstanceRow
              key={inst.name}
              project={project.name}
              instance={inst}
              onEdit={() => onEditInstance(inst)}
              onConfigEdit={() => onConfigEdit(inst)}
              onDelete={() => onDeleteInstance(inst)}
              onStart={() => onStart(inst)}
              onStop={() => onStop(inst)}
              onRestart={() => onRestart(inst)}
              onRebuild={() => onRebuild(inst)}
              busy={busy}
            />
          ))}
        </ul>
      )}
    </section>
  );
}

function InstanceRow({
  project,
  instance,
  onEdit,
  onConfigEdit,
  onDelete,
  onStart,
  onStop,
  onRestart,
  onRebuild,
  busy,
}: {
  project: string;
  instance: TopologyInstance;
  onEdit: () => void;
  onConfigEdit: () => void;
  onDelete: () => void;
  onStart: () => void;
  onStop: () => void;
  onRestart: () => void;
  onRebuild: () => void;
  busy: BusyMutations;
}) {
  // Per-instance status — polled while the row is mounted. 2s feels live
  // without hammering the admin process; admins can override by closing
  // the page.
  const { data: status } = useQuery({
    queryKey: ["topology-status", project, instance.name],
    queryFn: () => api.topologyInstanceStatus(project, instance.name),
    refetchInterval: 2000,
  });

  const running = status?.running ?? false;
  const healthLabel = status
    ? status.running
      ? status.health
      : "stopped"
    : "unknown";

  const isBusy = (m: BusyMutations[keyof BusyMutations]) =>
    m.isPending && m.variables?.inst.name === instance.name;

  return (
    <li className="px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-1 flex-wrap items-center gap-3">
          <span
            className="font-mono"
            style={{ color: "var(--color-fg)", fontSize: "0.85rem" }}
          >
            {instance.name}
          </span>
          <span
            className="font-mono uppercase tracking-[0.16em]"
            style={{
              color: "var(--color-fg-subtle)",
              fontSize: "0.62rem",
            }}
          >
            {instance.kind}
            {instance.cloud ? ` · ${instance.cloud}` : ""}
            {instance.backend ? ` · ${instance.backend}` : ""}
            {" · :"}
            {instance.port}
            {instance.sim ? ` · sim → ${instance.sim}` : ""}
          </span>
          <StatusBadge status={healthLabel} />
          {status?.health_detail && (
            <span
              className="font-mono"
              style={{
                color: "var(--color-status-error)",
                fontSize: "0.7rem",
              }}
            >
              {status.health_detail}
            </span>
          )}
        </div>
        <div className="flex items-center gap-1.5">
          {running ? (
            <Button
              variant="danger"
              size="sm"
              onClick={onStop}
              disabled={isBusy(busy.stop)}
            >
              stop
            </Button>
          ) : (
            <Button
              variant="primary"
              size="sm"
              onClick={onStart}
              disabled={isBusy(busy.start)}
            >
              start
            </Button>
          )}
          <Button
            variant="secondary"
            size="sm"
            onClick={onRestart}
            disabled={isBusy(busy.restart)}
          >
            restart
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onClick={onRebuild}
            disabled={isBusy(busy.rebuild)}
          >
            rebuild
          </Button>
          <Link
            to={`/ui/topology/${encodeURIComponent(project)}/${encodeURIComponent(instance.name)}/logs`}
            style={{
              fontSize: "0.7rem",
              fontFamily: "var(--font-mono)",
              padding: "0.25rem 0.6rem",
              border: "1px solid var(--color-border)",
              borderRadius: "var(--radius-xs)",
              color: "var(--color-fg-muted)",
              textDecoration: "none",
              letterSpacing: "0.05em",
            }}
          >
            logs
          </Link>
          <a
            href={`http://localhost:${instance.port}/ui/`}
            target="_blank"
            rel="noreferrer"
            style={{
              fontSize: "0.7rem",
              fontFamily: "var(--font-mono)",
              padding: "0.25rem 0.6rem",
              border: "1px solid var(--color-border)",
              borderRadius: "var(--radius-xs)",
              color: "var(--color-fg-muted)",
              textDecoration: "none",
              letterSpacing: "0.05em",
            }}
          >
            UI
          </a>
          <Button variant="ghost" size="sm" onClick={onConfigEdit}>
            config
          </Button>
          <Button variant="ghost" size="sm" onClick={onEdit}>
            edit
          </Button>
          <Button variant="ghost" size="sm" onClick={onDelete}>
            delete
          </Button>
        </div>
      </div>
      {shouldRenderDiagnostics(status) && (
        <div className="mt-3 -mx-4">
          <UnhealthyDiagnosticPanel
            project={project}
            instanceName={instance.name}
            status={status!}
          />
        </div>
      )}
    </li>
  );
}

function PortRegistryCard({ topology }: { topology: Topology }) {
  const ranges = topology.ports?.ranges ?? {};
  const claimed = useMemo(() => {
    const out: { project: string; instance: string; port: number; kind: InstanceKind }[] = [];
    for (const p of topology.projects ?? []) {
      for (const inst of p.instances ?? []) {
        out.push({
          project: p.name,
          instance: inst.name,
          port: inst.port,
          kind: inst.kind,
        });
      }
    }
    out.sort((a, b) => a.port - b.port);
    return out;
  }, [topology]);

  return (
    <section style={cardStyle}>
      <header
        className="px-4 py-3"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <div
          className="font-display"
          style={{
            fontStyle: "italic",
            fontWeight: 600,
            fontSize: "1rem",
            letterSpacing: "-0.02em",
          }}
        >
          Port registry
        </div>
        <div
          className="mt-0.5 font-mono uppercase tracking-[0.18em]"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.62rem" }}
        >
          configured ranges + claimed ports
        </div>
      </header>

      <div className="grid gap-4 p-4 md:grid-cols-2">
        <div>
          <div
            className="font-mono uppercase tracking-[0.18em] mb-2"
            style={{ color: "var(--color-fg-subtle)", fontSize: "0.62rem" }}
          >
            ranges
          </div>
          {Object.keys(ranges).length === 0 ? (
            <div
              className="font-mono"
              style={{
                color: "var(--color-fg-subtle)",
                fontSize: "0.78rem",
              }}
            >
              — none configured —
            </div>
          ) : (
            <ul className="font-mono text-[0.82rem]">
              {Object.entries(ranges).map(([kind, r]) => (
                <li key={kind}>
                  <span style={{ color: "var(--color-fg-subtle)" }}>
                    {kind}
                  </span>
                  <span className="ml-2" style={{ color: "var(--color-fg)" }}>
                    {r.from}–{r.to}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div>
          <div
            className="font-mono uppercase tracking-[0.18em] mb-2"
            style={{ color: "var(--color-fg-subtle)", fontSize: "0.62rem" }}
          >
            claimed
          </div>
          {claimed.length === 0 ? (
            <div
              className="font-mono"
              style={{
                color: "var(--color-fg-subtle)",
                fontSize: "0.78rem",
              }}
            >
              — none —
            </div>
          ) : (
            <ul className="font-mono text-[0.78rem]">
              {claimed.map((c) => (
                <li key={`${c.project}/${c.instance}`}>
                  <span style={{ color: "var(--color-fg)" }}>:{c.port}</span>
                  <span
                    className="ml-2"
                    style={{ color: "var(--color-fg-subtle)" }}
                  >
                    {c.project}/{c.instance} ({c.kind})
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </section>
  );
}

function ConfirmDeleteModal({
  pending,
  onCancel,
  onConfirm,
}: {
  pending: PendingDelete | null;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  if (!pending) {
    return (
      <Modal open={false} onClose={onCancel} title="">
        {null}
      </Modal>
    );
  }
  const target =
    pending.kind === "project"
      ? `project ${pending.project}`
      : `instance ${pending.project}/${pending.instance}`;
  return (
    <Modal
      open
      onClose={onCancel}
      kicker="confirm"
      title={`Delete ${target}`}
      size="sm"
      footer={
        <>
          <Button variant="ghost" size="sm" onClick={onCancel}>
            Cancel
          </Button>
          <Button variant="danger" size="sm" onClick={onConfirm}>
            Delete
          </Button>
        </>
      }
    >
      <p
        className="font-mono"
        style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}
      >
        This removes the entry from <code>sockerless.yaml</code>. Running
        processes are not stopped — stop the instance first if you want a
        clean teardown.
      </p>
    </Modal>
  );
}
