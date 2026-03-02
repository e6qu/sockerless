import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { AdminApiClient, type CloudType, type BackendType, type CreateProjectRequest } from "../api.js";

const api = new AdminApiClient();

const clouds: { value: CloudType; label: string; description: string }[] = [
  { value: "aws", label: "AWS", description: "ECS / Lambda + AWS Simulator" },
  { value: "gcp", label: "GCP", description: "Cloud Run / GCF + GCP Simulator" },
  { value: "azure", label: "Azure", description: "ACA / AZF + Azure Simulator" },
];

const backendsByCloud: Record<CloudType, { value: BackendType; label: string; description: string }[]> = {
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

  const create = useMutation({
    mutationFn: (req: CreateProjectRequest) => api.projectCreate(req),
    onSuccess: (data) => navigate(`/ui/projects/${data.name}`),
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

  const handleSubmit = () => {
    if (!cloud || !backend || !name) return;
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
    <div className="space-y-6">
      <h2 className="text-xl font-semibold">New Project</h2>

      {/* Step indicator */}
      <div className="flex gap-2 text-sm">
        <span className={step === "cloud" ? "font-bold text-blue-600 dark:text-blue-400" : "text-gray-500 dark:text-gray-400"}>
          1. Cloud
        </span>
        <span className="text-gray-400">/</span>
        <span className={step === "backend" ? "font-bold text-blue-600 dark:text-blue-400" : "text-gray-500 dark:text-gray-400"}>
          2. Backend
        </span>
        <span className="text-gray-400">/</span>
        <span className={step === "config" ? "font-bold text-blue-600 dark:text-blue-400" : "text-gray-500 dark:text-gray-400"}>
          3. Configure
        </span>
      </div>

      {/* Step 1: Cloud selection */}
      {step === "cloud" && (
        <div className="grid gap-4 sm:grid-cols-3">
          {clouds.map((c) => (
            <button
              key={c.value}
              onClick={() => selectCloud(c.value)}
              className={`rounded-lg border-2 p-6 text-left transition-colors ${
                cloud === c.value
                  ? "border-blue-500 bg-blue-50 dark:bg-blue-900/30"
                  : "border-gray-200 hover:border-blue-300 dark:border-gray-700 dark:hover:border-blue-600"
              }`}
            >
              <div className="text-lg font-semibold">{c.label}</div>
              <div className="mt-1 text-sm text-gray-500 dark:text-gray-400">{c.description}</div>
            </button>
          ))}
        </div>
      )}

      {/* Step 2: Backend selection */}
      {step === "backend" && cloud && (
        <div>
          <button
            onClick={() => setStep("cloud")}
            className="mb-4 text-sm text-blue-600 hover:underline dark:text-blue-400"
          >
            Back to cloud selection
          </button>
          <div className="grid gap-4 sm:grid-cols-2">
            {backendsByCloud[cloud].map((b) => (
              <button
                key={b.value}
                onClick={() => selectBackend(b.value)}
                className={`rounded-lg border-2 p-6 text-left transition-colors ${
                  backend === b.value
                    ? "border-blue-500 bg-blue-50 dark:bg-blue-900/30"
                    : "border-gray-200 hover:border-blue-300 dark:border-gray-700 dark:hover:border-blue-600"
                }`}
              >
                <div className="text-lg font-semibold">{b.label}</div>
                <div className="mt-1 text-sm text-gray-500 dark:text-gray-400">{b.description}</div>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Step 3: Configuration */}
      {step === "config" && cloud && backend && (
        <div>
          <button
            onClick={() => setStep("backend")}
            className="mb-4 text-sm text-blue-600 hover:underline dark:text-blue-400"
          >
            Back to backend selection
          </button>

          <div className="max-w-md space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
                Project Name
              </label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="my-project"
                className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700 dark:text-white"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
                Log Level
              </label>
              <select
                value={logLevel}
                onChange={(e) => setLogLevel(e.target.value)}
                className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700 dark:text-white"
              >
                <option value="debug">Debug</option>
                <option value="info">Info</option>
                <option value="warn">Warn</option>
                <option value="error">Error</option>
              </select>
            </div>

            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="auto-assign"
                checked={autoAssign}
                onChange={(e) => setAutoAssign(e.target.checked)}
                className="rounded border-gray-300"
              />
              <label htmlFor="auto-assign" className="text-sm text-gray-700 dark:text-gray-300">
                Auto-assign all ports
              </label>
            </div>

            {!autoAssign && (
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
                    Simulator Port
                  </label>
                  <input
                    type="number"
                    value={simPort}
                    onChange={(e) => setSimPort(Number(e.target.value))}
                    className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
                    Backend Port
                  </label>
                  <input
                    type="number"
                    value={backendPort}
                    onChange={(e) => setBackendPort(Number(e.target.value))}
                    className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
                    Frontend Port
                  </label>
                  <input
                    type="number"
                    value={frontendPort}
                    onChange={(e) => setFrontendPort(Number(e.target.value))}
                    className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
                    Frontend Mgmt Port
                  </label>
                  <input
                    type="number"
                    value={frontendMgmtPort}
                    onChange={(e) => setFrontendMgmtPort(Number(e.target.value))}
                    className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                  />
                </div>
              </div>
            )}

            {create.isError && (
              <p className="text-sm text-red-600 dark:text-red-400">
                {create.error instanceof Error ? create.error.message : "Failed to create project"}
              </p>
            )}

            <button
              onClick={handleSubmit}
              disabled={!name || create.isPending}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {create.isPending ? "Creating..." : "Create Project"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
