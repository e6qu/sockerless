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
  fetchBlockedUsers,
  fetchUserEmails,
  fetchUserGPGKeys,
  fetchUserSSHKeys,
  fetchUserSSHSigningKeys,
  setUserEmailVisibility,
  unblockUser,
} from "../api.js";
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

type AccountTab = "ssh-keys" | "gpg-keys" | "signing-keys" | "emails" | "blocked";

const ACCOUNT_NAV: SettingsNavSection<AccountTab>[] = [
  { items: [{ key: "emails", label: "Emails" }] },
  {
    title: "Access",
    items: [
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
        {tab === "gpg-keys" && <GPGKeysTab />}
        {tab === "signing-keys" && <SigningKeysTab />}
        {tab === "emails" && <EmailsTab />}
        {tab === "blocked" && <BlockedUsersTab />}
      </SettingsLayout>
    </div>
  );
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
