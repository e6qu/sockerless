import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { Button, PageHeading } from "@sockerless/ui-core/components";
import {
  AdminApiClient,
  type CloudType,
  type BackendType,
  type CreateProjectRequest,
} from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

const api = new AdminApiClient();

const clouds: { value: CloudType; label: string; description: string }[] = [
  { value: "aws", label: "AWS", description: "ECS / Lambda + AWS simulator" },
  { value: "gcp", label: "GCP", description: "Cloud Run / GCF + GCP simulator" },
  { value: "azure", label: "Azure", description: "ACA / AZF + Azure simulator" },
];

const backendsByCloud: Record<
  CloudType,
  { value: BackendType; label: string; description: string }[]
> = {
  aws: [
    { value: "ecs", label: "ECS", description: "Container-based (Elastic Container Service)" },
    { value: "lambda", label: "Lambda", description: "Function-based (AWS Lambda)" },
  ],
  gcp: [
    { value: "cloudrun", label: "Cloud Run", description: "Container-based (Cloud Run)" },
    { value: "gcf", label: "GCF", description: "Function-based (Google Cloud Functions)" },
  ],
  azure: [
    { value: "aca", label: "ACA", description: "Container-based (Azure Container Apps)" },
    { value: "azf", label: "AZF", description: "Function-based (Azure Functions)" },
  ],
};

const validNameRE = /^[a-z0-9][a-z0-9_-]*$/;

type Step = "cloud" | "backend" | "config";

export function ProjectCreatePage() {
  const navigate = useNavigate();
  const [step, setStep] = useState<Step>("cloud");
  const [cloud, setCloud] = useState<CloudType | null>(null);
  const [backend, setBackend] = useState<BackendType | null>(null);
  const [name, setName] = useState("");
  const [logLevel, setLogLevel] = useState("info");
  const [simPort, setSimPort] = useState(0);
  const [backendPort, setBackendPort] = useState(0);
  const [frontendPort, setFrontendPort] = useState(0);
  const [frontendMgmtPort, setFrontendMgmtPort] = useState(0);
  const [autoAssign, setAutoAssign] = useState(true);

  const queryClient = useQueryClient();

  const create = useMutation({
    mutationFn: (req: CreateProjectRequest) => api.projectCreate(req),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      navigate(`/ui/projects/${encodeURIComponent(data.name)}`);
    },
  });

  const selectCloud = (c: CloudType) => {
    setCloud(c);
    setBackend(null);
    setStep("backend");
  };

  const selectBackend = (b: BackendType) => {
    setBackend(b);
    if (!name) setName(`${cloud}-${b}`);
    setStep("config");
  };

  const nameError =
    name.length > 0 && !validNameRE.test(name)
      ? "must start with a-z/0-9; lowercase letters, digits, hyphens, underscores only"
      : "";

  const handleSubmit = (e?: { preventDefault(): void }) => {
    e?.preventDefault();
    if (create.isPending) return;
    if (!cloud || !backend || !name || !!nameError) return;
    create.mutate({
      name,
      cloud,
      backend,
      log_level: logLevel,
      sim_port: autoAssign ? 0 : simPort,
      backend_port: autoAssign ? 0 : backendPort,
      frontend_port: autoAssign ? 0 : frontendPort,
      frontend_mgmt_port: autoAssign ? 0 : frontendMgmtPort,
    });
  };

  return (
    <div>
      <PageHeading
        kicker="admin · new project"
        title={<>Create project</>}
        meta="3-step wizard: pick cloud → pick backend → configure ports + name."
      />

      <Stepper step={step} />

      {step === "cloud" && (
        <div className="grid gap-3 sm:grid-cols-3">
          {clouds.map((c) => (
            <ChoiceCard
              key={c.value}
              selected={cloud === c.value}
              onClick={() => selectCloud(c.value)}
              title={c.label}
              description={c.description}
            />
          ))}
        </div>
      )}

      {step === "backend" && cloud && (
        <div>
          <button
            type="button"
            onClick={() => setStep("cloud")}
            className="mb-4 font-mono uppercase tracking-[0.12em]"
            style={{
              fontSize: "0.7rem",
              color: "var(--color-fg-muted)",
              background: "transparent",
              border: 0,
            }}
          >
            ← back to cloud
          </button>
          <div className="grid gap-3 sm:grid-cols-2">
            {backendsByCloud[cloud].map((b) => (
              <ChoiceCard
                key={b.value}
                selected={backend === b.value}
                onClick={() => selectBackend(b.value)}
                title={b.label}
                description={b.description}
              />
            ))}
          </div>
        </div>
      )}

      {step === "config" && cloud && backend && (
        <div>
          <button
            type="button"
            onClick={() => setStep("backend")}
            className="mb-4 font-mono uppercase tracking-[0.12em]"
            style={{
              fontSize: "0.7rem",
              color: "var(--color-fg-muted)",
              background: "transparent",
              border: 0,
            }}
          >
            ← back to backend
          </button>

          <form onSubmit={handleSubmit} className="max-w-md space-y-5">
            <Field
              label="Project name"
              error={nameError}
              input={
                <input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="my-project"
                  maxLength={64}
                  className="w-full"
                />
              }
            />

            <Field
              label="Log level"
              input={
                <select
                  value={logLevel}
                  onChange={(e) => setLogLevel(e.target.value)}
                  className="w-full"
                >
                  <option value="debug">debug</option>
                  <option value="info">info</option>
                  <option value="warn">warn</option>
                  <option value="error">error</option>
                </select>
              }
            />

            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                id="auto-assign"
                checked={autoAssign}
                onChange={(e) => setAutoAssign(e.target.checked)}
              />
              <span
                className="font-mono"
                style={{ fontSize: "0.78rem", color: "var(--color-fg)" }}
              >
                Auto-assign all ports
              </span>
            </label>

            {!autoAssign && (
              <div className="grid grid-cols-2 gap-3">
                <Field
                  label="Simulator port"
                  input={
                    <input
                      type="number"
                      value={simPort}
                      onChange={(e) => setSimPort(Number(e.target.value))}
                      className="w-full"
                    />
                  }
                />
                <Field
                  label="Backend port"
                  input={
                    <input
                      type="number"
                      value={backendPort}
                      onChange={(e) => setBackendPort(Number(e.target.value))}
                      className="w-full"
                    />
                  }
                />
                <Field
                  label="Frontend port"
                  input={
                    <input
                      type="number"
                      value={frontendPort}
                      onChange={(e) => setFrontendPort(Number(e.target.value))}
                      className="w-full"
                    />
                  }
                />
                <Field
                  label="Frontend mgmt port"
                  input={
                    <input
                      type="number"
                      value={frontendMgmtPort}
                      onChange={(e) =>
                        setFrontendMgmtPort(Number(e.target.value))
                      }
                      className="w-full"
                    />
                  }
                />
              </div>
            )}

            {create.isError && (
              <ErrorPanel
                kicker="create failed"
                message={
                  create.error instanceof Error
                    ? create.error.message
                    : "request failed"
                }
              />
            )}

            <Button
              type="submit"
              variant="primary"
              size="md"
              disabled={!name || !!nameError || create.isPending}
            >
              {create.isPending ? "Creating…" : "Create project →"}
            </Button>
          </form>
        </div>
      )}
    </div>
  );
}

function Stepper({ step }: { step: Step }) {
  const steps: { key: Step; n: string; label: string }[] = [
    { key: "cloud", n: "01", label: "Cloud" },
    { key: "backend", n: "02", label: "Backend" },
    { key: "config", n: "03", label: "Configure" },
  ];
  return (
    <div className="mb-6 flex items-center gap-3">
      {steps.map((s, i) => {
        const active = step === s.key;
        const done =
          (step === "backend" && s.key === "cloud") ||
          (step === "config" && (s.key === "cloud" || s.key === "backend"));
        return (
          <div key={s.key} className="flex items-center gap-3">
            <div
              className="flex items-center gap-2 px-3 py-1.5 font-mono uppercase tracking-[0.12em]"
              style={{
                fontSize: "0.7rem",
                background: active ? "var(--color-accent)" : "transparent",
                color: active
                  ? "var(--color-accent-fg)"
                  : done
                    ? "var(--color-fg)"
                    : "var(--color-fg-subtle)",
                border: `1px solid ${active ? "var(--color-accent)" : "var(--color-border)"}`,
              }}
            >
              <span style={{ opacity: 0.7 }}>{s.n}</span>
              <span>{s.label}</span>
            </div>
            {i < steps.length - 1 && (
              <span style={{ color: "var(--color-fg-subtle)" }}>→</span>
            )}
          </div>
        );
      })}
    </div>
  );
}

function ChoiceCard({
  title,
  description,
  selected,
  onClick,
}: {
  title: string;
  description: string;
  selected: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="text-left p-5"
      style={{
        background: "var(--color-surface)",
        border: `1px solid ${selected ? "var(--color-accent)" : "var(--color-border)"}`,
        borderLeft: `3px solid ${selected ? "var(--color-accent)" : "var(--color-border)"}`,
        borderRadius: "var(--radius-sm)",
        cursor: "pointer",
        transition: "border-color 0.12s var(--ease-out-quint)",
      }}
    >
      <div
        className="font-display"
        style={{
          fontStyle: "italic",
          fontWeight: 600,
          fontSize: "1.4rem",
          letterSpacing: "-0.02em",
          lineHeight: 1.05,
          color: "var(--color-fg)",
        }}
      >
        {title}
      </div>
      <div
        className="mt-2 font-mono text-[12px]"
        style={{ color: "var(--color-fg-muted)" }}
      >
        {description}
      </div>
    </button>
  );
}

function Field({
  label,
  input,
  error,
}: {
  label: string;
  input: React.ReactNode;
  error?: string;
}) {
  return (
    <div>
      <label
        className="mb-1 block text-[10px] uppercase tracking-[0.18em]"
        style={{ color: "var(--color-fg-subtle)" }}
      >
        {label}
      </label>
      {input}
      {error && (
        <p
          className="mt-1 font-mono"
          style={{ color: "var(--color-status-error)", fontSize: "0.7rem" }}
        >
          {error}
        </p>
      )}
    </div>
  );
}
