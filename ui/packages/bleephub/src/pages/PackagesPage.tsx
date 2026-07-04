import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "react-router";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import {
  deletePackage,
  deletePackageVersion,
  fetchCurrentUser,
  fetchPackageFiles,
  fetchPackages,
  fetchPackageVersions,
  restorePackageVersion,
  uploadPackageVersion,
  type PackageScope,
} from "../api.js";
import type {
  GithubPackage,
  GithubPackageVersion,
  GithubPackageType,
} from "../types.js";
import {
  Box,
  Button,
  DialogActions,
  ErrorBanner,
  FormLabel,
  Modal,
  PageTitle,
  Tabs,
} from "../components/ui.js";
import { PackageIcon, TrashIcon } from "../components/octicons.js";

const PACKAGE_TYPES: GithubPackageType[] = [
  "npm",
  "maven",
  "rubygems",
  "nuget",
  "docker",
  "container",
];

const pkgCol = createColumnHelper<GithubPackage>();
const verCol = createColumnHelper<GithubPackageVersion>();

export function PackagesPage() {
  const params = useParams<{ org?: string; owner?: string; repo?: string }>();
  const [tab, setTab] = useState<GithubPackageType>("container");

  const {
    data: currentUser,
    isLoading: userLoading,
    isError: userError,
    error: userErrorObj,
  } = useQuery({
    queryKey: ["current-user"],
    queryFn: fetchCurrentUser,
    enabled: !params.org && !(params.owner && params.repo),
  });

  const scope: PackageScope | null = params.org
    ? { kind: "org", org: params.org }
    : params.owner && params.repo
      ? { kind: "repo", owner: params.owner, repo: params.repo }
      : currentUser
        ? { kind: "user", username: currentUser.login }
        : null;

  if (userLoading) {
    return <Spinner label="loading user" />;
  }
  if (userError) {
    return (
      <InlineError title="Failed to load current user" detail={String(userErrorObj)} />
    );
  }
  if (!scope) {
    return <InlineError title="Unable to determine package scope" />;
  }

  return (
    <div>
      <PageTitle
        icon={<PackageIcon size={20} />}
        title="Packages"
        meta="Manage packages and versions."
      />

      <Tabs<GithubPackageType>
        items={PACKAGE_TYPES.map((t) => ({ key: t, label: t }))}
        active={tab}
        onChange={setTab}
      />

      <PackagesList scope={scope} packageType={tab} />
    </div>
  );
}

function PackagesList({
  scope,
  packageType,
}: {
  scope: PackageScope;
  packageType: GithubPackageType;
}) {
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<GithubPackage | null>(null);
  const [showUpload, setShowUpload] = useState(false);

  const listKey = ["packages", scope, packageType];
  const {
    data,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: listKey,
    queryFn: () => fetchPackages(scope),
  });

  const deletePkgMut = useMutation({
    mutationFn: (pkg: GithubPackage) =>
      deletePackage(scope, pkg.package_type, pkg.name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: listKey }),
  });

  const filtered = useMemo(
    () => (data ?? []).filter((p) => p.package_type === packageType),
    [data, packageType],
  );

  const columns = useMemo(
    () => [
      pkgCol.accessor("name", {
        header: "Name",
        cell: (info) => (
          <button
            type="button"
            onClick={() => setSelected(info.row.original)}
            className="font-medium"
            style={{
              background: "transparent",
              border: "none",
              padding: 0,
              color: "var(--color-accent)",
              cursor: "pointer",
            }}
          >
            {info.getValue()}
          </button>
        ),
      }),
      pkgCol.accessor("visibility", { header: "Visibility" }),
      pkgCol.accessor("version_count", {
        header: "Versions",
        cell: (info) => (
          <span className="tabular-nums">{info.getValue()}</span>
        ),
      }),
      pkgCol.accessor("updated_at", {
        header: "Updated",
        cell: (info) => new Date(info.getValue<string>()).toLocaleString(),
      }),
      pkgCol.display({
        id: "actions",
        header: "Actions",
        cell: (info) => {
          const pkg = info.row.original;
          return (
            <div className="flex flex-wrap items-center gap-2">
              <Button
                size="sm"
                variant="secondary"
                onClick={() => setSelected(pkg)}
              >
                Versions
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  if (confirm(`Delete package ${pkg.name}?`)) {
                    deletePkgMut.mutate(pkg);
                  }
                }}
                disabled={deletePkgMut.isPending}
              >
                <TrashIcon size={14} /> Delete
              </Button>
            </div>
          );
        },
      }),
    ],
    [deletePkgMut, scope],
  );

  if (isError) {
    return (
      <InlineError title="Failed to load packages" detail={String(error)} />
    );
  }
  if (isLoading || !data) {
    return <Spinner label="loading packages" />;
  }

  return (
    <>
      <div className="mb-3 flex items-center justify-between">
        <div style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
          {filtered.length} package{filtered.length === 1 ? "" : "s"}
        </div>
        <Button size="sm" variant="primary" onClick={() => setShowUpload(true)}>
          Upload version
        </Button>
      </div>
      <DataTable
        data={filtered}
        columns={columns}
        emptyMessage="No packages yet."
      />
      {selected && (
        <PackageDetailDialog
          scope={scope}
          pkg={selected}
          onClose={() => setSelected(null)}
        />
      )}
      {showUpload && (
        <UploadPackageVersionDialog
          scope={scope}
          defaultType={packageType}
          onClose={() => setShowUpload(false)}
        />
      )}
    </>
  );
}

function PackageDetailDialog({
  scope,
  pkg,
  onClose,
}: {
  scope: PackageScope;
  pkg: GithubPackage;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);

  const versionsKey = ["package-versions", scope, pkg.package_type, pkg.name];
  const {
    data: versions,
    isLoading,
    isError,
  } = useQuery({
    queryKey: versionsKey,
    queryFn: () => fetchPackageVersions(scope, pkg.package_type, pkg.name),
  });

  const deleteMut = useMutation({
    mutationFn: (v: GithubPackageVersion) =>
      deletePackageVersion(scope, pkg.package_type, pkg.name, v.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: versionsKey });
      queryClient.invalidateQueries({ queryKey: ["packages", scope, pkg.package_type] });
    },
    onError: (err: Error) => setError(err.message),
  });

  const restoreMut = useMutation({
    mutationFn: (v: GithubPackageVersion) =>
      restorePackageVersion(scope, pkg.package_type, pkg.name, v.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: versionsKey });
      queryClient.invalidateQueries({ queryKey: ["packages", scope, pkg.package_type] });
    },
    onError: (err: Error) => setError(err.message),
  });

  const columns = useMemo(
    () => [
      verCol.accessor("name", { header: "Version" }),
      verCol.accessor("description", {
        header: "Description",
        cell: (info) => info.getValue() || "—",
      }),
      verCol.display({
        id: "files",
        header: "Files",
        cell: (info) => <VersionFiles scope={scope} pkg={pkg} version={info.row.original} />,
      }),
      verCol.display({
        id: "actions",
        header: "Actions",
        cell: (info) => {
          const v = info.row.original;
          const deleted = !!v.deleted_at;
          return (
            <div className="flex flex-wrap items-center gap-2">
              {deleted ? (
                // GitHub has no repository-scoped package-version restore endpoint —
                // restore is only available for user- and organization-scoped packages.
                scope.kind !== "repo" && (
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => restoreMut.mutate(v)}
                    disabled={restoreMut.isPending}
                  >
                    Restore
                  </Button>
                )
              ) : (
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    if (confirm(`Delete version ${v.name}?`)) {
                      deleteMut.mutate(v);
                    }
                  }}
                  disabled={deleteMut.isPending}
                >
                  <TrashIcon size={14} /> Delete
                </Button>
              )}
            </div>
          );
        },
      }),
    ],
    [deleteMut, restoreMut, scope, pkg],
  );

  return (
    <Modal title={`${pkg.name} versions`} onClose={onClose}>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      {isError ? (
        <InlineError title="Failed to load versions" />
      ) : isLoading || !versions ? (
        <Spinner label="loading versions" />
      ) : (
        <DataTable
          data={versions}
          columns={columns}
          emptyMessage="No versions."
        />
      )}
      <DialogActions>
        <Button onClick={onClose} variant="ghost">
          Close
        </Button>
      </DialogActions>
    </Modal>
  );
}

function VersionFiles({
  scope,
  pkg,
  version,
}: {
  scope: PackageScope;
  pkg: GithubPackage;
  version: GithubPackageVersion;
}) {
  const { data, isLoading } = useQuery({
    queryKey: ["package-files", scope, pkg.package_type, pkg.name, version.id],
    queryFn: () => fetchPackageFiles(scope, pkg.package_type, pkg.name, version.id),
  });

  if (isLoading || !data) return <span style={{ fontSize: "0.78rem" }}>…</span>;
  return (
    <span style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
      {data.length} file{data.length === 1 ? "" : "s"}
    </span>
  );
}

function UploadPackageVersionDialog({
  scope,
  defaultType,
  onClose,
}: {
  scope: PackageScope;
  defaultType: GithubPackageType;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [pkgName, setPkgName] = useState("");
  const [pkgType, setPkgType] = useState<GithubPackageType>(defaultType);
  const [version, setVersion] = useState("");
  const [fileName, setFileName] = useState("artifact.tgz");
  const [fileContent, setFileContent] = useState("hello");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: async () => {
      let ownerType: "user" | "org" | "repository";
      let owner: string;
      switch (scope.kind) {
        case "user":
          ownerType = "user";
          owner = scope.username;
          break;
        case "org":
          ownerType = "org";
          owner = scope.org;
          break;
        case "repo":
          ownerType = "repository";
          owner = `${scope.owner}/${scope.repo}`;
          break;
        default:
          ownerType = "user";
          owner = "";
      }
      return uploadPackageVersion(ownerType, owner, pkgType, pkgName, {
        version,
        files: [
          {
            name: fileName,
            content_type: "application/gzip",
            content_base64: btoa(fileContent),
          },
        ],
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["packages", scope, pkgType] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  const valid =
    pkgName.trim() !== "" && version.trim() !== "" && fileName.trim() !== "";

  return (
    <Modal title="Upload package version" onClose={onClose}>
      <div className="mb-4 grid grid-cols-2 gap-3">
        <div>
          <FormLabel id="pkg-type">Package type</FormLabel>
          <select
            id="pkg-type"
            value={pkgType}
            onChange={(e) => setPkgType(e.target.value as GithubPackageType)}
            className="w-full"
          >
            {PACKAGE_TYPES.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </div>
        <div>
          <FormLabel id="pkg-name">Package name</FormLabel>
          <input
            id="pkg-name"
            value={pkgName}
            onChange={(e) => setPkgName(e.target.value)}
            placeholder="my-package"
            className="w-full"
          />
        </div>
        <div>
          <FormLabel id="pkg-version">Version</FormLabel>
          <input
            id="pkg-version"
            value={version}
            onChange={(e) => setVersion(e.target.value)}
            placeholder="1.0.0"
            className="w-full"
          />
        </div>
        <div>
          <FormLabel id="file-name">File name</FormLabel>
          <input
            id="file-name"
            value={fileName}
            onChange={(e) => setFileName(e.target.value)}
            className="w-full"
          />
        </div>
      </div>
      <Box header="File content">
        <textarea
          value={fileContent}
          onChange={(e) => setFileContent(e.target.value)}
          rows={4}
          className="w-full"
          style={{ border: "none" }}
        />
      </Box>

      {error && <ErrorBanner>{error}</ErrorBanner>}

      <DialogActions>
        <Button onClick={onClose} disabled={mutation.isPending} variant="ghost">
          Cancel
        </Button>
        <Button
          onClick={() => {
            setError(null);
            mutation.mutate();
          }}
          disabled={mutation.isPending || !valid}
          variant="primary"
        >
          {mutation.isPending ? "Uploading…" : "Upload"}
        </Button>
      </DialogActions>
    </Modal>
  );
}
