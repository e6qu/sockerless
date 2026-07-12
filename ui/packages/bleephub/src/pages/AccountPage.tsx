import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  addUserEmails,
  blockUser,
  createUserGPGKey,
  createUserSSHKey,
  createUserSSHSigningKey,
  deleteUserEmails,
  deleteUserGPGKey,
  deleteUserSSHKey,
  deleteUserSSHSigningKey,
  createFineGrainedPAT,
  deleteFineGrainedPAT,
  fetchFineGrainedPATDashboard,
  fetchBlockedUsers,
  fetchUserEmails,
  fetchUserGPGKeys,
  fetchUserSSHKeys,
  fetchUserSSHSigningKeys,
  setUserEmailVisibility,
  reviewFineGrainedPATRequest,
  unblockUser,
} from "../api.js";
import type { FineGrainedPATPermissions } from "../api.js";
import type {
  GithubBlockedUser,
  GithubGPGKey,
  GithubSSHKey,
  GithubSSHSigningKey,
  GithubUserEmail,
} from "../types.js";
import { PageTitle, Box, Button, ErrorBanner, FormLabel } from "../components/ui.js";
import { SettingsLayout, type SettingsNavSection } from "../components/SettingsLayout.js";
import { KeyIcon } from "../components/octicons.js";

type AccountTab = "tokens" | "ssh-keys" | "gpg-keys" | "signing-keys" | "emails" | "blocked";

const ACCOUNT_NAV: SettingsNavSection<AccountTab>[] = [
  { items: [{ key: "emails", label: "Emails" }] },
  {
    title: "Access",
    items: [
      { key: "tokens", label: "Personal access tokens" },
      { key: "ssh-keys", label: "SSH keys" },
      { key: "gpg-keys", label: "GPG keys" },
      { key: "signing-keys", label: "Signing keys" },
    ],
  },
  { title: "Moderation", items: [{ key: "blocked", label: "Blocked users" }] },
];

export function AccountPage() {
  const [tab, setTab] = useState<AccountTab>("ssh-keys");
  return (
    <div>
      <PageTitle title="Account" meta="Keys, email addresses, and blocked users on the authenticated account" />
      <SettingsLayout sections={ACCOUNT_NAV} active={tab} onSelect={setTab}>
        {tab === "ssh-keys" && <SSHKeysTab />}
        {tab === "tokens" && <FineGrainedTokensTab />}
        {tab === "gpg-keys" && <GPGKeysTab />}
        {tab === "signing-keys" && <SigningKeysTab />}
        {tab === "emails" && <EmailsTab />}
        {tab === "blocked" && <BlockedUsersTab />}
      </SettingsLayout>
    </div>
  );
}

const REPOSITORY_PERMISSIONS = [
  ["contents", "Contents"], ["issues", "Issues"], ["pull_requests", "Pull requests"],
  ["actions", "Actions"], ["checks", "Checks"], ["deployments", "Deployments"],
] as const;

function FineGrainedTokensTab() {
  const client = useQueryClient();
  const query = useQuery({ queryKey: ["fine-grained-pats"], queryFn: fetchFineGrainedPATDashboard });
  const [name, setName] = useState("");
  const [owner, setOwner] = useState("");
  const [selection, setSelection] = useState<"all" | "subset" | "none">("all");
  const [repositoryIDs, setRepositoryIDs] = useState<number[]>([]);
  const [expires, setExpires] = useState("");
  const [reason, setReason] = useState("");
  const [permissions, setPermissions] = useState<Record<string, string>>({ contents: "read" });
  const [credential, setCredential] = useState<string | null>(null);

  const createMutation = useMutation({
    mutationFn: () => createFineGrainedPAT({
      name, resource_owner: owner || query.data!.resource_owners[0].login,
      repository_selection: selection, repository_ids: selection === "subset" ? repositoryIDs : [],
      permissions: { repository: permissions, organization: { members: "read" } },
      ...(expires ? { expires_at: new Date(`${expires}T23:59:59Z`).toISOString() } : {}),
      ...(reason.trim() ? { reason: reason.trim() } : {}),
    }),
    onSuccess: (created) => {
      setCredential(created.token); setName(""); setReason(""); setRepositoryIDs([]);
      client.invalidateQueries({ queryKey: ["fine-grained-pats"] });
    },
  });
  const deleteMutation = useMutation({ mutationFn: deleteFineGrainedPAT, onSuccess: () => client.invalidateQueries({ queryKey: ["fine-grained-pats"] }) });
  const reviewMutation = useMutation({
    mutationFn: ({ org, id, action }: { org: string; id: number; action: "approve" | "deny" }) => reviewFineGrainedPATRequest(org, id, action),
    onSuccess: () => client.invalidateQueries({ queryKey: ["fine-grained-pats"] }),
  });

  if (query.isLoading) return <Spinner label="loading personal access tokens" />;
  if (query.isError) return <InlineError title="Failed to load personal access tokens" detail={String(query.error)} />;
  const data = query.data!;
  const selectedOwner = owner || data.resource_owners[0]?.login || "";
  const repositories = data.repositories[selectedOwner] ?? [];
  const error = createMutation.error || deleteMutation.error || reviewMutation.error;

  return <div className="flex flex-col gap-4">
    <div style={{ padding: "1.15rem", border: "1px solid color-mix(in srgb, var(--color-brand-purple) 48%, var(--color-border))", borderRadius: 10, background: "linear-gradient(120deg, color-mix(in srgb, var(--color-brand-purple) 18%, var(--color-bg)), color-mix(in srgb, var(--color-brand-cyan) 14%, var(--color-bg-subtle)), color-mix(in srgb, var(--color-brand-pink) 12%, var(--color-bg)))", boxShadow: "var(--shadow-floating)" }}>
      <h2 style={{ fontSize: "1.15rem", fontWeight: 700 }}>Fine-grained personal access tokens</h2>
      <p style={{ color: "var(--color-fg-muted)", marginTop: ".25rem" }}>Limit every credential to one resource owner, selected repositories, explicit permissions, and an expiration date.</p>
    </div>
    {credential && <div role="alert" style={{ padding: "1rem", borderRadius: 8, border: "1px solid var(--color-status-ok)", background: "color-mix(in srgb, var(--color-status-ok) 13%, var(--color-bg))" }}>
      <b>Your new token</b><p style={{ color: "var(--color-fg-muted)", margin: ".25rem 0 .65rem" }}>Copy it now. For your security, it will not be shown again.</p>
      <code style={{ display: "block", overflowWrap: "anywhere", padding: ".7rem", borderRadius: 6, background: "var(--color-bg-subtle)", border: "1px solid var(--color-border)" }}>{credential}</code>
      <Button size="sm" onClick={() => navigator.clipboard.writeText(credential)} style={{ marginTop: ".65rem" }}>Copy token</Button>
    </div>}
    {error && <ErrorBanner>{String(error)}</ErrorBanner>}
    <Box header={<span style={{ fontWeight: 650 }}>Generate new token</span>}>
      <div className="grid gap-4" style={{ padding: "1rem", gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))" }}>
        <div><FormLabel id="pat-name">Token name</FormLabel><input id="pat-name" className="w-full" maxLength={40} value={name} onChange={(e) => setName(e.target.value)} placeholder="Deployment automation" /></div>
        <div><FormLabel id="pat-owner">Resource owner</FormLabel><select id="pat-owner" className="w-full" value={selectedOwner} onChange={(e) => { setOwner(e.target.value); setRepositoryIDs([]); }}>{data.resource_owners.map((item) => <option key={item.login} value={item.login}>{item.login} · {item.type}</option>)}</select></div>
        <div><FormLabel id="pat-expiry">Expiration</FormLabel><input id="pat-expiry" type="date" className="w-full" min={new Date().toISOString().slice(0, 10)} value={expires} onChange={(e) => setExpires(e.target.value)} /></div>
        <div><FormLabel id="pat-reason">Reason for organization access</FormLabel><input id="pat-reason" className="w-full" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Used by the release workflow" /></div>
      </div>
      <div style={{ padding: "0 1rem 1rem" }}><FormLabel id="pat-access">Repository access</FormLabel><div id="pat-access" className="flex flex-wrap gap-3">{([['all','All repositories'],['subset','Only selected repositories'],['none','No repositories']] as const).map(([value,label]) => <label key={value} className="flex items-center gap-2"><input type="radio" name="pat-access" checked={selection === value} onChange={() => setSelection(value)} />{label}</label>)}</div></div>
      {selection === "subset" && <div style={{ padding: "0 1rem 1rem" }}><FormLabel id="pat-repositories">Selected repositories</FormLabel><div id="pat-repositories" className="grid gap-2" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(210px, 1fr))" }}>{repositories.map((repo) => <label key={repo.id} className="flex items-center gap-2"><input type="checkbox" checked={repositoryIDs.includes(repo.id)} onChange={(e) => setRepositoryIDs(e.target.checked ? [...repositoryIDs, repo.id] : repositoryIDs.filter((id) => id !== repo.id))} />{repo.name}{repo.private ? " · Private" : ""}</label>)}</div></div>}
      <div style={{ padding: "0 1rem 1rem" }}><FormLabel id="pat-permissions">Repository permissions</FormLabel><div id="pat-permissions" className="grid gap-2" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(210px, 1fr))" }}>{REPOSITORY_PERMISSIONS.map(([key, label]) => <label key={key} className="flex items-center justify-between gap-3"><span>{label}</span><select aria-label={`${label} permission`} value={permissions[key] ?? "none"} onChange={(e) => { const next = { ...permissions }; if (e.target.value === "none") delete next[key]; else next[key] = e.target.value; setPermissions(next); }}><option value="none">No access</option><option value="read">Read</option><option value="write">Read and write</option></select></label>)}</div></div>
      <div className="flex justify-end" style={{ padding: "0 1rem 1rem" }}><Button variant="primary" disabled={!name.trim() || (selection === "subset" && repositoryIDs.length === 0) || createMutation.isPending} onClick={() => createMutation.mutate()}>Generate token</Button></div>
    </Box>
    {data.pending_requests.length > 0 && <Box header={<span style={{ fontWeight: 650 }}>Organization approval requests</span>}><ul style={{ listStyle: "none", margin: 0, padding: 0 }}>{data.pending_requests.map((request) => <li key={`${request.organization}-${request.id}`} className="flex flex-wrap items-center justify-between gap-3" style={{ padding: ".9rem 1rem", borderBottom: "1px solid var(--color-border)" }}><div><b>{request.token_name}</b><div style={{ color: "var(--color-fg-muted)", fontSize: ".82rem" }}>{request.owner.login} requests {request.organization} · {request.repository_selection} repositories{request.reason ? ` · ${request.reason}` : ""}</div></div><div className="flex gap-2"><Button size="sm" onClick={() => reviewMutation.mutate({ org: request.organization, id: request.id, action: "deny" })}>Deny</Button><Button size="sm" variant="primary" onClick={() => reviewMutation.mutate({ org: request.organization, id: request.id, action: "approve" })}>Approve</Button></div></li>)}</ul></Box>}
    <Box header={<span style={{ fontWeight: 650 }}>Your fine-grained tokens</span>}>{data.tokens.length === 0 ? <div style={{ padding: "1rem", color: "var(--color-fg-muted)" }}>You have not generated any fine-grained tokens.</div> : <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>{data.tokens.map((token) => <li key={token.id} className="flex flex-wrap items-center justify-between gap-3" style={{ padding: ".9rem 1rem", borderBottom: "1px solid var(--color-border)" }}><div><div className="flex items-center gap-2"><b>{token.name}</b><span style={{ padding: ".12rem .45rem", borderRadius: 999, fontSize: ".72rem", fontWeight: 650, color: token.status === "active" ? "var(--color-status-ok)" : "var(--color-status-warn)", background: token.status === "active" ? "var(--color-status-ok-soft)" : "var(--color-status-warn-soft)" }}>{token.status}</span></div><div style={{ color: "var(--color-fg-muted)", fontSize: ".82rem" }}>{token.resource_owner} · {token.repository_selection} repositories · {token.expires_at ? `expires ${new Date(token.expires_at).toLocaleDateString()}` : "no expiration"}</div></div><Button size="sm" variant="danger" disabled={deleteMutation.isPending} onClick={() => confirm(`Delete ${token.name}?`) && deleteMutation.mutate(token.id)}>Delete</Button></li>)}</ul>}</Box>
  </div>;
}

/** Shared add-key form + key list for the three key kinds. */
function KeyManager<T extends { id: number }>({
  kind,
  queryKey,
  list,
  create,
  remove,
  titleOptional,
  renderKey,
}: {
  kind: string;
  queryKey: string;
  list: () => Promise<T[]>;
  create: (title: string, key: string) => Promise<T>;
  remove: (id: number) => Promise<void>;
  titleOptional?: boolean;
  renderKey: (k: T) => React.ReactNode;
}) {
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [title, setTitle] = useState("");
  const [key, setKey] = useState("");

  const query = useQuery({ queryKey: [queryKey], queryFn: list });

  const addMut = useMutation({
    mutationFn: () => create(title.trim(), key.trim()),
    onSuccess: () => {
      setError(null);
      setTitle("");
      setKey("");
      queryClient.invalidateQueries({ queryKey: [queryKey] });
    },
    onError: (err: Error) => setError(err.message),
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => remove(id),
    onSuccess: () => {
      setError(null);
      queryClient.invalidateQueries({ queryKey: [queryKey] });
    },
    onError: (err: Error) => setError(err.message),
  });

  if (query.isLoading) return <Spinner label={`loading ${kind}s`} />;
  if (query.isError)
    return <InlineError title={`Failed to load ${kind}s`} detail={String(query.error)} />;

  const keys = query.data ?? [];

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <Box header={<span style={{ fontWeight: 600 }}>Add {kind}</span>}>
        <div style={{ padding: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
          <FormLabel id={`${queryKey}-title`}>Title{titleOptional ? " (optional)" : ""}</FormLabel>
          <input
            id={`${queryKey}-title`}
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            className="w-full"
          />
          <FormLabel id={`${queryKey}-key`}>Key</FormLabel>
          <textarea
            id={`${queryKey}-key`}
            value={key}
            onChange={(e) => setKey(e.target.value)}
            rows={4}
            className="w-full"
            style={{ fontFamily: "var(--font-mono)", fontSize: "0.8rem" }}
          />
          <div className="flex justify-end">
            <Button
              variant="primary"
              onClick={() => {
                setError(null);
                addMut.mutate();
              }}
              disabled={addMut.isPending || !key.trim() || (!titleOptional && !title.trim())}
            >
              Add {kind}
            </Button>
          </div>
        </div>
      </Box>
      <Box header={<span style={{ fontWeight: 600 }}>{kind[0].toUpperCase() + kind.slice(1)}s</span>}>
        {keys.length === 0 ? (
          <div style={{ padding: "1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
            No {kind}s.
          </div>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {keys.map((k) => (
              <li
                key={k.id}
                className="flex items-center justify-between gap-4"
                style={{ padding: "0.6rem 1rem", borderBottom: "1px solid var(--color-border)" }}
              >
                <div className="flex min-w-0 items-center gap-2">
                  <KeyIcon size={16} style={{ color: "var(--color-fg-muted)", flexShrink: 0 }} />
                  <div style={{ minWidth: 0 }}>{renderKey(k)}</div>
                </div>
                <Button
                  size="sm"
                  variant="danger"
                  onClick={() => {
                    if (confirm(`Delete this ${kind}?`)) deleteMut.mutate(k.id);
                  }}
                  disabled={deleteMut.isPending}
                >
                  delete
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Box>
    </div>
  );
}

const truncatedMono: React.CSSProperties = {
  color: "var(--color-fg-muted)",
  fontSize: "0.78rem",
  fontFamily: "var(--font-mono)",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

function SSHKeysTab() {
  return (
    <KeyManager<GithubSSHKey>
      kind="SSH key"
      queryKey="user-ssh-keys"
      list={fetchUserSSHKeys}
      create={createUserSSHKey}
      remove={deleteUserSSHKey}
      titleOptional
      renderKey={(k) => (
        <>
          <div style={{ fontWeight: 500 }}>{k.title || `Key #${k.id}`}</div>
          <div style={truncatedMono}>{k.key}</div>
          <div style={{ color: "var(--color-fg-muted)", fontSize: "0.72rem" }}>
            {k.verified ? "verified" : "unverified"} · added{" "}
            {new Date(k.created_at).toLocaleDateString()}
          </div>
        </>
      )}
    />
  );
}

function GPGKeysTab() {
  return (
    <KeyManager<GithubGPGKey>
      kind="GPG key"
      queryKey="user-gpg-keys"
      list={fetchUserGPGKeys}
      create={(name, armored) => createUserGPGKey(armored, name || undefined)}
      remove={deleteUserGPGKey}
      titleOptional
      renderKey={(k) => (
        <>
          <div style={{ fontWeight: 500 }}>{k.name || k.key_id || `Key #${k.id}`}</div>
          <div style={truncatedMono}>{k.public_key}</div>
          <div style={{ color: "var(--color-fg-muted)", fontSize: "0.72rem" }}>
            {[
              k.can_sign && "sign",
              k.can_encrypt_commits && "encrypt",
              k.can_certify && "certify",
            ]
              .filter(Boolean)
              .join(" · ")}{" "}
            · added {new Date(k.created_at).toLocaleDateString()}
          </div>
        </>
      )}
    />
  );
}

function SigningKeysTab() {
  return (
    <KeyManager<GithubSSHSigningKey>
      kind="SSH signing key"
      queryKey="user-ssh-signing-keys"
      list={fetchUserSSHSigningKeys}
      create={createUserSSHSigningKey}
      remove={deleteUserSSHSigningKey}
      titleOptional
      renderKey={(k) => (
        <>
          <div style={{ fontWeight: 500 }}>{k.title || `Key #${k.id}`}</div>
          <div style={truncatedMono}>{k.key}</div>
          <div style={{ color: "var(--color-fg-muted)", fontSize: "0.72rem" }}>
            added {new Date(k.created_at).toLocaleDateString()}
          </div>
        </>
      )}
    />
  );
}

function EmailsTab() {
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [newEmail, setNewEmail] = useState("");

  const query = useQuery({ queryKey: ["user-emails"], queryFn: fetchUserEmails });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["user-emails"] });
  const addMut = useMutation({
    mutationFn: () => addUserEmails([newEmail.trim()]),
    onSuccess: () => {
      setError(null);
      setNewEmail("");
      invalidate();
    },
    onError: (err: Error) => setError(err.message),
  });
  const deleteMut = useMutation({
    mutationFn: (email: string) => deleteUserEmails([email]),
    onSuccess: () => {
      setError(null);
      invalidate();
    },
    onError: (err: Error) => setError(err.message),
  });
  const visibilityMut = useMutation({
    mutationFn: (visibility: "public" | "private") => setUserEmailVisibility(visibility),
    onSuccess: () => {
      setError(null);
      invalidate();
    },
    onError: (err: Error) => setError(err.message),
  });

  if (query.isLoading) return <Spinner label="loading emails" />;
  if (query.isError)
    return <InlineError title="Failed to load emails" detail={String(query.error)} />;

  const emails = query.data ?? [];
  const primary = emails.find((e) => e.primary);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <Box header={<span style={{ fontWeight: 600 }}>Add email address</span>}>
        <div style={{ padding: "1rem", display: "flex", gap: "0.75rem", alignItems: "center" }}>
          <input
            type="email"
            aria-label="New email address"
            value={newEmail}
            onChange={(e) => setNewEmail(e.target.value)}
            placeholder="you@example.com"
            className="flex-1"
            style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
          />
          <Button
            variant="primary"
            onClick={() => {
              setError(null);
              addMut.mutate();
            }}
            disabled={addMut.isPending || !newEmail.trim()}
          >
            Add
          </Button>
        </div>
      </Box>
      <Box header={<span style={{ fontWeight: 600 }}>Email addresses</span>}>
        {emails.length === 0 ? (
          <div style={{ padding: "1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
            No email addresses.
          </div>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {emails.map((e: GithubUserEmail) => (
              <li
                key={e.email}
                className="flex items-center justify-between gap-4"
                style={{ padding: "0.6rem 1rem", borderBottom: "1px solid var(--color-border)" }}
              >
                <div>
                  <span style={{ fontWeight: 500 }}>{e.email}</span>
                  <span style={{ marginLeft: "0.5rem", fontSize: "0.75rem", color: "var(--color-fg-muted)" }}>
                    {[
                      e.primary && "primary",
                      e.verified ? "verified" : "unverified",
                      e.visibility ? `visibility: ${e.visibility}` : "visibility unset",
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </span>
                </div>
                {!e.primary && (
                  <Button
                    size="sm"
                    variant="danger"
                    onClick={() => {
                      if (confirm(`Remove ${e.email}?`)) deleteMut.mutate(e.email);
                    }}
                    disabled={deleteMut.isPending}
                  >
                    remove
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
      </Box>
      {primary && (
        <Box header={<span style={{ fontWeight: 600 }}>Primary email visibility</span>}>
          <div style={{ padding: "1rem", display: "flex", alignItems: "center", gap: "1rem" }}>
            <span style={{ fontSize: "0.85rem" }}>
              {primary.email} is {primary.visibility ?? "unset"}
            </span>
            <Button
              size="sm"
              variant="secondary"
              onClick={() =>
                visibilityMut.mutate(primary.visibility === "public" ? "private" : "public")
              }
              disabled={visibilityMut.isPending}
            >
              Make {primary.visibility === "public" ? "private" : "public"}
            </Button>
          </div>
        </Box>
      )}
    </div>
  );
}

function BlockedUsersTab() {
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [username, setUsername] = useState("");

  const query = useQuery({ queryKey: ["user-blocks"], queryFn: fetchBlockedUsers });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["user-blocks"] });
  const blockMut = useMutation({
    mutationFn: () => blockUser(username.trim()),
    onSuccess: () => {
      setError(null);
      setUsername("");
      invalidate();
    },
    onError: (err: Error) => setError(err.message),
  });
  const unblockMut = useMutation({
    mutationFn: (login: string) => unblockUser(login),
    onSuccess: () => {
      setError(null);
      invalidate();
    },
    onError: (err: Error) => setError(err.message),
  });

  if (query.isLoading) return <Spinner label="loading blocked users" />;
  if (query.isError)
    return <InlineError title="Failed to load blocked users" detail={String(query.error)} />;

  const blocked = query.data ?? [];

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <Box header={<span style={{ fontWeight: 600 }}>Block a user</span>}>
        <div style={{ padding: "1rem", display: "flex", gap: "0.75rem", alignItems: "center" }}>
          <input
            type="text"
            aria-label="Username to block"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="username"
            className="flex-1"
            style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
          />
          <Button
            variant="danger"
            onClick={() => {
              setError(null);
              blockMut.mutate();
            }}
            disabled={blockMut.isPending || !username.trim()}
          >
            Block
          </Button>
        </div>
      </Box>
      <Box header={<span style={{ fontWeight: 600 }}>Blocked users</span>}>
        {blocked.length === 0 ? (
          <div style={{ padding: "1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
            No blocked users.
          </div>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {blocked.map((b: GithubBlockedUser) => (
              <li
                key={b.login}
                className="flex items-center justify-between gap-4"
                style={{ padding: "0.6rem 1rem", borderBottom: "1px solid var(--color-border)" }}
              >
                <span style={{ fontWeight: 500 }}>{b.login}</span>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => unblockMut.mutate(b.login)}
                  disabled={unblockMut.isPending}
                >
                  unblock
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Box>
    </div>
  );
}
