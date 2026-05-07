import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DataTable, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { useState } from "react";
import {
  createApp,
  fetchApps,
  fetchInstallations,
} from "../api.js";
import type { BleephubApp, BleephubInstallation } from "../types.js";

type Tab = "apps" | "installations";

export function AppsPage() {
  const [tab, setTab] = useState<Tab>("apps");
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Apps</h2>
        <button
          type="button"
          onClick={() => setShowCreate(true)}
          className="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700"
        >
          Create App
        </button>
      </div>
      <div className="flex gap-2 border-b border-gray-200">
        <TabButton active={tab === "apps"} onClick={() => setTab("apps")}>
          Apps
        </TabButton>
        <TabButton active={tab === "installations"} onClick={() => setTab("installations")}>
          Installations
        </TabButton>
      </div>
      {tab === "apps" ? <AppsTab /> : <InstallationsTab />}
      {showCreate && <CreateAppDialog onClose={() => setShowCreate(false)} />}
    </div>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  const cls = active
    ? "px-4 py-2 border-b-2 border-blue-600 text-blue-600 font-medium"
    : "px-4 py-2 border-b-2 border-transparent text-gray-600 hover:text-gray-900";
  return (
    <button type="button" onClick={onClick} className={cls}>
      {children}
    </button>
  );
}

const appsCol = createColumnHelper<BleephubApp>();

function AppsTab() {
  const { data, isLoading } = useQuery({
    queryKey: ["apps"],
    queryFn: fetchApps,
    refetchInterval: 5000,
  });
  if (isLoading || !data) return <Spinner />;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const columns: any[] = [
    appsCol.accessor("id", { header: "ID" }),
    appsCol.accessor("slug", { header: "Slug" }),
    appsCol.accessor("name", { header: "Name" }),
    appsCol.accessor("description", { header: "Description" }),
    appsCol.accessor("createdAt", {
      header: "Created",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
  ];

  return (
    <>
      <div className="text-sm text-gray-600">{data.length} app{data.length === 1 ? "" : "s"}</div>
      <DataTable data={data} columns={columns} filterPlaceholder="Filter apps..." />
    </>
  );
}

const installsCol = createColumnHelper<BleephubInstallation>();

function InstallationsTab() {
  const { data, isLoading } = useQuery({
    queryKey: ["installations"],
    queryFn: fetchInstallations,
    refetchInterval: 5000,
  });
  if (isLoading || !data) return <Spinner />;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const columns: any[] = [
    installsCol.accessor("id", { header: "ID" }),
    installsCol.accessor("appSlug", { header: "App" }),
    installsCol.accessor("targetLogin", { header: "Target" }),
    installsCol.accessor("targetType", { header: "Type" }),
    installsCol.accessor("repositorySelection", { header: "Repo selection" }),
    installsCol.accessor("createdAt", {
      header: "Created",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
  ];

  return (
    <>
      <div className="text-sm text-gray-600">
        {data.length} installation{data.length === 1 ? "" : "s"}
      </div>
      <DataTable data={data} columns={columns} filterPlaceholder="Filter installations..." />
    </>
  );
}

function CreateAppDialog({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => createApp({ name, description }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["apps"] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl p-6 max-w-md w-full space-y-4">
        <h3 className="text-lg font-semibold">Create App</h3>
        <div>
          <label htmlFor="app-name" className="block text-sm font-medium text-gray-700">Name</label>
          <input
            id="app-name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md text-sm"
          />
        </div>
        <div>
          <label htmlFor="app-desc" className="block text-sm font-medium text-gray-700">Description</label>
          <textarea
            id="app-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
            className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md text-sm"
          />
        </div>
        {error && <div className="text-sm text-red-600">{error}</div>}
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 border border-gray-300 rounded-md text-sm font-medium text-gray-700 hover:bg-gray-50"
            disabled={mutation.isPending}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => {
              setError(null);
              mutation.mutate();
            }}
            disabled={mutation.isPending || !name.trim()}
            className="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700 disabled:opacity-50"
          >
            {mutation.isPending ? "Creating…" : "Create"}
          </button>
        </div>
      </div>
    </div>
  );
}
