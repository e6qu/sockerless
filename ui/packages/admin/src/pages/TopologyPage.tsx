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
import { InstanceForm } from "../components/InstanceForm.js";
import { ProjectForm } from "../components/ProjectForm.js";

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
  const [pendingDelete, setPendingDelete] = useState<PendingDelete | null>(
    null,
  );

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
    mutationFn: ({
      project,
      name,
    }: {
      project: string;
      name: string;
    }) => api.topologyInstanceStart(project, name),
    onSuccess: (_d, vars) => {
      invalidate();
      push({ tone: "success", title: `Started ${vars.name}` });
    },
    onError: (err, vars) => reportError(err, `Failed to start ${vars.name}`),
  });

  const stopInstanceMutation = useMutation({
    mutationFn: ({
      project,
      name,
    }: {
      project: string;
      name: string;
    }) => api.topologyInstanceStop(project, name),
    onSuccess: (_d, vars) => {
      invalidate();
      push({ tone: "success", title: `Stopped ${vars.name}` });
    },
    onError: (err, vars) => reportError(err, `Failed to stop ${vars.name}`),
  });

  const rebuildInstanceMutation = useMutation({
    mutationFn: ({
      project,
      name,
    }: {
      project: string;
      name: string;
    }) => api.topologyInstanceRebuild(project, name),
    onSuccess: (_d, vars) => {
      invalidate();
      push({ tone: "success", title: `Rebuilt ${vars.name}` });
    },
    onError: (err, vars) =>
      reportError(err, `Failed to rebuild ${vars.name}`),
  });

  if (isLoading) return <Spinner label="loading topology" />;
  if (isError) return <ErrorPanel message={error?.message} />;
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
        meta={`${projects.length} project${projects.length === 1 ? "" : "s"} · ${totalInstances} instance${totalInstances === 1 ? "" : "s"}`}
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
          </div>
        }
      />

      {projects.length === 0 ? (
        <p
          className="font-mono uppercase tracking-[0.18em] py-6 text-center"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.7rem" }}
        >
          — no projects configured —
        </p>
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
                  name: inst.name,
                })
              }
              onStop={(inst) =>
                stopInstanceMutation.mutate({
                  project: p.name,
                  name: inst.name,
                })
              }
              onRebuild={(inst) =>
                rebuildInstanceMutation.mutate({
                  project: p.name,
                  name: inst.name,
                })
              }
              busy={{
                start: startInstanceMutation,
                stop: stopInstanceMutation,
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
  start: { isPending: boolean; variables?: { name: string } };
  stop: { isPending: boolean; variables?: { name: string } };
  rebuild: { isPending: boolean; variables?: { name: string } };
}

function ProjectCard({
  project,
  onAddInstance,
  onEditInstance,
  onDeleteInstance,
  onDeleteProject,
  onStart,
  onStop,
  onRebuild,
  busy,
}: {
  project: TopologyProject;
  onAddInstance: () => void;
  onEditInstance: (inst: TopologyInstance) => void;
  onDeleteInstance: (inst: TopologyInstance) => void;
  onDeleteProject: () => void;
  onStart: (inst: TopologyInstance) => void;
  onStop: (inst: TopologyInstance) => void;
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
              onDelete={() => onDeleteInstance(inst)}
              onStart={() => onStart(inst)}
              onStop={() => onStop(inst)}
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
  onDelete,
  onStart,
  onStop,
  onRebuild,
  busy,
}: {
  project: string;
  instance: TopologyInstance;
  onEdit: () => void;
  onDelete: () => void;
  onStart: () => void;
  onStop: () => void;
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
    m.isPending && m.variables?.name === instance.name;

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
          <Button variant="ghost" size="sm" onClick={onEdit}>
            edit
          </Button>
          <Button variant="ghost" size="sm" onClick={onDelete}>
            delete
          </Button>
        </div>
      </div>
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
