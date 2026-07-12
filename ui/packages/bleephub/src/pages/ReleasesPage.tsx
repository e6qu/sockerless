import { useEffect, useState, type FormEvent } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { InlineError, Spinner } from "@sockerless/ui-core/components";
import {
  createRelease,
  deleteRelease,
  deleteReleaseAsset,
  downloadReleaseAsset,
  fetchRelease,
  fetchReleases,
  updateRelease,
  uploadReleaseAsset,
  type ReleasePayload,
} from "../api.js";
import type { GithubRelease, GithubReleaseAsset } from "../types.js";
import { RepoHeader } from "../components/Shell.js";
import { Blankslate, Box, Button, ErrorBanner, FormLabel, PageTitle } from "../components/ui.js";
import { DownloadIcon, PlusIcon, TagIcon, TrashIcon } from "../components/octicons.js";

const inputStyle = {
  width: "100%",
  border: "1px solid var(--color-border)",
  borderRadius: "var(--radius-md)",
  background: "var(--color-surface)",
  color: "var(--color-fg)",
  padding: "0.45rem 0.6rem",
} as const;

export function ReleasesPage() {
  const { owner = "", repo = "", releaseId } = useParams<{
    owner: string;
    repo: string;
    releaseId?: string;
  }>();
  const location = useLocation();
  const creating = location.pathname.endsWith("/releases/new");
  const id = releaseId ? Number(releaseId) : 0;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="code" />
      {creating ? (
        <ReleaseEditor owner={owner} repo={repo} />
      ) : id > 0 ? (
        <ReleaseDetail owner={owner} repo={repo} releaseId={id} />
      ) : (
        <ReleaseList owner={owner} repo={repo} />
      )}
    </div>
  );
}

function ReleaseList({ owner, repo }: { owner: string; repo: string }) {
  const releases = useQuery({
    queryKey: ["releases", owner, repo],
    queryFn: () => fetchReleases(owner, repo),
  });
  if (releases.isLoading) return <Spinner label="loading releases" />;
  if (releases.isError) return <InlineError title="Failed to load releases" detail={String(releases.error)} />;

  return (
    <>
      <PageTitle
        icon={<TagIcon size={22} />}
        title="Releases"
        meta={`${releases.data?.length ?? 0} releases`}
        actions={
          <Link to={`/ui/repos/${owner}/${repo}/releases/new`} style={{ textDecoration: "none" }}>
            <Button variant="primary"><PlusIcon size={14} /> New release</Button>
          </Link>
        }
      />
      {(releases.data?.length ?? 0) === 0 ? (
        <Blankslate icon={<TagIcon size={28} />} title="No releases published">
          Create a release from a real repository tag and attach distributable files.
        </Blankslate>
      ) : (
        <Box>
          {releases.data!.map((release, index) => (
            <ReleaseRow
              key={release.id}
              owner={owner}
              repo={repo}
              release={release}
              last={index === releases.data!.length - 1}
            />
          ))}
        </Box>
      )}
    </>
  );
}

function ReleaseRow({ owner, repo, release, last }: {
  owner: string;
  repo: string;
  release: GithubRelease;
  last: boolean;
}) {
  return (
    <Link
      to={`/ui/repos/${owner}/${repo}/releases/${release.id}`}
      className="flex items-start gap-3"
      style={{ padding: "0.85rem 1rem", borderBottom: last ? "none" : "1px solid var(--color-border)", textDecoration: "none", color: "inherit" }}
    >
      <TagIcon size={17} style={{ color: "var(--color-fg-muted)", marginTop: 2 }} />
      <div className="min-w-0 flex-1">
        <div style={{ fontWeight: 600 }}>{release.name || release.tag_name}</div>
        <div className="mt-1 flex flex-wrap gap-2" style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
          <span className="font-mono">{release.tag_name}</span>
          {release.draft && <span>Draft</span>}
          {release.prerelease && <span>Pre-release</span>}
          {!release.draft && release.published_at && <span>Published {new Date(release.published_at).toLocaleDateString()}</span>}
          <span>{release.assets.length} assets</span>
        </div>
      </div>
    </Link>
  );
}

function ReleaseEditor({ owner, repo, release }: { owner: string; repo: string; release?: GithubRelease }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [tagName, setTagName] = useState(release?.tag_name ?? "");
  const [target, setTarget] = useState(release?.target_commitish ?? "");
  const [name, setName] = useState(release?.name ?? "");
  const [body, setBody] = useState(release?.body ?? "");
  const [draft, setDraft] = useState(release?.draft ?? false);
  const [prerelease, setPrerelease] = useState(release?.prerelease ?? false);

  useEffect(() => {
    if (!release) return;
    setTagName(release.tag_name);
    setTarget(release.target_commitish);
    setName(release.name);
    setBody(release.body);
    setDraft(release.draft);
    setPrerelease(release.prerelease);
  }, [release]);

  const save = useMutation({
    mutationFn: async () => {
      const payload: ReleasePayload = {
        tag_name: tagName.trim(),
        target_commitish: target.trim() || undefined,
        name: name.trim(),
        body,
        draft,
        prerelease,
      };
      return release
        ? updateRelease(owner, repo, release.id, payload)
        : createRelease(owner, repo, payload);
    },
    onSuccess: async (saved) => {
      await queryClient.invalidateQueries({ queryKey: ["releases", owner, repo] });
      navigate(`/ui/repos/${owner}/${repo}/releases/${saved.id}`);
    },
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (tagName.trim()) save.mutate();
  };

  return (
    <>
      <PageTitle title={release ? `Edit ${release.name || release.tag_name}` : "Create a new release"} />
      {save.isError && <ErrorBanner>{String(save.error)}</ErrorBanner>}
      <form onSubmit={submit} className="flex flex-col gap-4">
        <div className="grid gap-4 md:grid-cols-2">
          <label><FormLabel>Tag</FormLabel><input aria-label="Tag" required value={tagName} onChange={(e) => setTagName(e.target.value)} style={inputStyle} placeholder="v1.0.0" /></label>
          <label><FormLabel>Target branch or commit</FormLabel><input aria-label="Target branch or commit" value={target} onChange={(e) => setTarget(e.target.value)} style={inputStyle} placeholder="main" /></label>
        </div>
        <label><FormLabel>Release title</FormLabel><input aria-label="Release title" value={name} onChange={(e) => setName(e.target.value)} style={inputStyle} /></label>
        <label><FormLabel>Release notes</FormLabel><textarea aria-label="Release notes" value={body} onChange={(e) => setBody(e.target.value)} rows={10} style={{ ...inputStyle, resize: "vertical" }} /></label>
        <div className="flex flex-wrap gap-5">
          <label className="inline-flex items-center gap-2"><input type="checkbox" checked={draft} onChange={(e) => setDraft(e.target.checked)} /> Save as draft</label>
          <label className="inline-flex items-center gap-2"><input type="checkbox" checked={prerelease} onChange={(e) => setPrerelease(e.target.checked)} /> Mark as pre-release</label>
        </div>
        <div className="flex gap-2">
          <Button type="submit" variant="primary" disabled={!tagName.trim() || save.isPending}>{release ? "Save changes" : "Create release"}</Button>
          <Button type="button" onClick={() => navigate(release ? `/ui/repos/${owner}/${repo}/releases/${release.id}` : `/ui/repos/${owner}/${repo}/releases`)}>Cancel</Button>
        </div>
      </form>
    </>
  );
}

function ReleaseDetail({ owner, repo, releaseId }: { owner: string; repo: string; releaseId: number }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const release = useQuery({ queryKey: ["release", owner, repo, releaseId], queryFn: () => fetchRelease(owner, repo, releaseId) });
  const remove = useMutation({
    mutationFn: () => deleteRelease(owner, repo, releaseId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["releases", owner, repo] });
      navigate(`/ui/repos/${owner}/${repo}/releases`);
    },
  });
  if (release.isLoading) return <Spinner label="loading release" />;
  if (release.isError || !release.data) return <InlineError title="Failed to load release" detail={String(release.error)} />;
  if (editing) return <ReleaseEditor owner={owner} repo={repo} release={release.data} />;

  return (
    <>
      <PageTitle
        icon={<TagIcon size={22} />}
        title={release.data.name || release.data.tag_name}
        meta={<><span className="font-mono">{release.data.tag_name}</span>{release.data.draft ? " · Draft" : release.data.prerelease ? " · Pre-release" : " · Published"}</>}
        actions={<><Button onClick={() => setEditing(true)}>Edit</Button><Button variant="danger" onClick={() => { if (confirm("Delete this release and all of its assets?")) remove.mutate(); }}><TrashIcon size={14} /> Delete</Button></>}
      />
      {remove.isError && <ErrorBanner>{String(remove.error)}</ErrorBanner>}
      {release.data.body && <div className="mb-5 whitespace-pre-wrap" style={{ lineHeight: 1.6 }}>{release.data.body}</div>}
      <ReleaseAssets owner={owner} repo={repo} release={release.data} />
    </>
  );
}

function ReleaseAssets({ owner, repo, release }: { owner: string; repo: string; release: GithubRelease }) {
  const queryClient = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  const [label, setLabel] = useState("");
  const upload = useMutation({
    mutationFn: () => uploadReleaseAsset(owner, repo, release.id, file!, label.trim()),
    onSuccess: async () => {
      setFile(null);
      setLabel("");
      await queryClient.invalidateQueries({ queryKey: ["release", owner, repo, release.id] });
    },
  });
  const remove = useMutation({
    mutationFn: (assetId: number) => deleteReleaseAsset(owner, repo, assetId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["release", owner, repo, release.id] }),
  });
  const download = async (asset: GithubReleaseAsset) => {
    const blob = await downloadReleaseAsset(owner, repo, asset.id);
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = asset.name;
    anchor.click();
    URL.revokeObjectURL(url);
  };

  return (
    <section aria-labelledby="release-assets-heading">
      <h2 id="release-assets-heading" className="mb-3" style={{ fontSize: "1rem", fontWeight: 600 }}>Assets</h2>
      {(upload.isError || remove.isError) && <ErrorBanner>{String(upload.error ?? remove.error)}</ErrorBanner>}
      {release.assets.length > 0 && (
        <Box className="mb-4">
          {release.assets.map((asset, index) => (
            <div key={asset.id} className="flex flex-wrap items-center gap-3" style={{ padding: "0.65rem 1rem", borderBottom: index === release.assets.length - 1 ? "none" : "1px solid var(--color-border)" }}>
              <div className="min-w-0 flex-1"><div style={{ fontWeight: 500 }}>{asset.label || asset.name}</div><div style={{ fontSize: "0.76rem", color: "var(--color-fg-muted)" }}>{asset.name} · {asset.size.toLocaleString()} bytes · {asset.download_count} downloads</div></div>
              <Button size="sm" aria-label={`Download ${asset.name}`} onClick={() => void download(asset)}><DownloadIcon size={14} /></Button>
              <Button size="sm" variant="danger" aria-label={`Delete ${asset.name}`} onClick={() => remove.mutate(asset.id)}><TrashIcon size={14} /></Button>
            </div>
          ))}
        </Box>
      )}
      <div className="flex flex-wrap items-end gap-3">
        <label><FormLabel>Asset file</FormLabel><input aria-label="Asset file" type="file" onChange={(e) => setFile(e.target.files?.[0] ?? null)} /></label>
        <label><FormLabel>Label</FormLabel><input aria-label="Asset label" value={label} onChange={(e) => setLabel(e.target.value)} style={{ ...inputStyle, minWidth: 220 }} /></label>
        <Button variant="primary" disabled={!file || upload.isPending} onClick={() => upload.mutate()}><PlusIcon size={14} /> Upload asset</Button>
      </div>
    </section>
  );
}
