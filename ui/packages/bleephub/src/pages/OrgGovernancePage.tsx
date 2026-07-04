import { useState } from "react";
import { useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchOrgInvitations,
  fetchFailedOrgInvitations,
  createOrgInvitation,
  cancelOrgInvitation,
  fetchOutsideCollaborators,
  removeOutsideCollaborator,
  fetchOrgBlocks,
  blockOrgUser,
  unblockOrgUser,
  fetchOrgCustomProperties,
  upsertOrgCustomProperties,
  deleteOrgCustomProperty,
  fetchOrgRepoCustomPropertyValues,
  setOrgRepoCustomPropertyValues,
  fetchOrgIssueTypes,
  createOrgIssueType,
  updateOrgIssueType,
  deleteOrgIssueType,
  fetchOrgRoles,
  fetchOrgRoleTeams,
  fetchOrgRoleUsers,
  assignOrgRoleToTeam,
  revokeOrgRoleFromTeam,
  assignOrgRoleToUser,
  revokeOrgRoleFromUser,
} from "../api.js";
import type {
  GithubAccount,
  GithubCustomProperty,
  GithubCustomPropertyValueType,
  GithubIssueType,
  GithubOrgInvitation,
  GithubOrgRole,
  GithubOrgRepoCustomPropertyValues,
} from "../types.js";
import { OrgHeader } from "../components/Shell.js";
import {
  Box,
  Button,
  ErrorBanner,
  FormLabel,
  Modal,
  DialogActions,
  Tabs,
} from "../components/ui.js";
import { ChevronDownIcon, ChevronRightIcon } from "../components/octicons.js";

type GovernanceTab = "people" | "roles" | "properties" | "issue-types";

export function OrgGovernancePage() {
  const { org = "" } = useParams<{ org: string }>();
  const [tab, setTab] = useState<GovernanceTab>("people");

  return (
    <div>
      <OrgHeader org={org} active="governance" />
      <Tabs
        items={[
          { key: "people" as const, label: "People" },
          { key: "roles" as const, label: "Roles" },
          { key: "properties" as const, label: "Custom properties" },
          { key: "issue-types" as const, label: "Issue types" },
        ]}
        active={tab}
        onChange={setTab}
      />
      {tab === "people" && <PeoplePanel org={org} />}
      {tab === "roles" && <RolesPanel org={org} />}
      {tab === "properties" && <PropertiesPanel org={org} />}
      {tab === "issue-types" && <IssueTypesPanel org={org} />}
    </div>
  );
}

// ─── People: invitations, outside collaborators, blocks ─────────────────

function invitee(inv: GithubOrgInvitation): string {
  return inv.login ? `@${inv.login}` : (inv.email ?? `#${inv.id}`);
}

function PeoplePanel({ org }: { org: string }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState("direct_member");
  const [blockUsername, setBlockUsername] = useState("");

  const invitations = useQuery({
    queryKey: ["org-invitations", org],
    queryFn: () => fetchOrgInvitations(org),
  });
  const failed = useQuery({
    queryKey: ["org-failed-invitations", org],
    queryFn: () => fetchFailedOrgInvitations(org),
  });
  const outside = useQuery({
    queryKey: ["org-outside-collaborators", org],
    queryFn: () => fetchOutsideCollaborators(org),
  });
  const blocks = useQuery({
    queryKey: ["org-blocks", org],
    queryFn: () => fetchOrgBlocks(org),
  });

  const inviteMut = useMutation({
    mutationFn: () => createOrgInvitation(org, { email: inviteEmail.trim(), role: inviteRole }),
    onSuccess: () => {
      setError(null);
      setInviteEmail("");
      qc.invalidateQueries({ queryKey: ["org-invitations", org] });
    },
    onError: (err: Error) => setError(err.message),
  });
  const cancelMut = useMutation({
    mutationFn: (id: number) => cancelOrgInvitation(org, id),
    onSuccess: () => {
      setError(null);
      qc.invalidateQueries({ queryKey: ["org-invitations", org] });
    },
    onError: (err: Error) => setError(err.message),
  });
  const removeOutsideMut = useMutation({
    mutationFn: (username: string) => removeOutsideCollaborator(org, username),
    onSuccess: () => {
      setError(null);
      qc.invalidateQueries({ queryKey: ["org-outside-collaborators", org] });
    },
    onError: (err: Error) => setError(err.message),
  });
  const blockMut = useMutation({
    mutationFn: () => blockOrgUser(org, blockUsername.trim()),
    onSuccess: () => {
      setError(null);
      setBlockUsername("");
      qc.invalidateQueries({ queryKey: ["org-blocks", org] });
    },
    onError: (err: Error) => setError(err.message),
  });
  const unblockMut = useMutation({
    mutationFn: (username: string) => unblockOrgUser(org, username),
    onSuccess: () => {
      setError(null);
      qc.invalidateQueries({ queryKey: ["org-blocks", org] });
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <div className="flex flex-col gap-4">
      {error && <ErrorBanner>{error}</ErrorBanner>}

      <Box header={<span style={{ fontWeight: 600 }}>Pending invitations</span>}>
        <div className="flex gap-2" style={{ padding: "0.75rem 1rem", borderBottom: "1px solid var(--color-border)" }}>
          <input
            aria-label="Invitee email"
            type="email"
            placeholder="person@example.com"
            value={inviteEmail}
            onChange={(e) => setInviteEmail(e.target.value)}
            className="w-full"
          />
          <select aria-label="Invitation role" value={inviteRole} onChange={(e) => setInviteRole(e.target.value)}>
            <option value="direct_member">direct_member</option>
            <option value="admin">admin</option>
            <option value="billing_manager">billing_manager</option>
          </select>
          <Button
            variant="primary"
            size="sm"
            disabled={inviteMut.isPending || !inviteEmail.trim()}
            onClick={() => {
              setError(null);
              inviteMut.mutate();
            }}
          >
            Invite
          </Button>
        </div>
        {invitations.isLoading && <Spinner label="loading invitations" />}
        {invitations.isError && (
          <InlineError title="Failed to load invitations" detail={String(invitations.error)} />
        )}
        {invitations.data &&
          (invitations.data.length === 0 ? (
            <EmptyRow>No pending invitations.</EmptyRow>
          ) : (
            invitations.data.map((inv, i) => (
              <PersonRow key={inv.id} last={i === invitations.data.length - 1}>
                <span style={{ fontWeight: 500 }}>{invitee(inv)}</span>
                <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>
                  {inv.role} · invited by {inv.inviter ? `@${inv.inviter.login}` : "—"} ·{" "}
                  {new Date(inv.created_at).toLocaleDateString()}
                </span>
                <Button
                  size="sm"
                  variant="danger"
                  disabled={cancelMut.isPending}
                  onClick={() => cancelMut.mutate(inv.id)}
                >
                  cancel
                </Button>
              </PersonRow>
            ))
          ))}
      </Box>

      <Box header={<span style={{ fontWeight: 600 }}>Failed invitations</span>}>
        {failed.isLoading && <Spinner label="loading failed invitations" />}
        {failed.isError && (
          <InlineError title="Failed to load failed invitations" detail={String(failed.error)} />
        )}
        {failed.data &&
          (failed.data.length === 0 ? (
            <EmptyRow>No failed invitations.</EmptyRow>
          ) : (
            failed.data.map((inv, i) => (
              <PersonRow key={inv.id} last={i === failed.data.length - 1}>
                <span style={{ fontWeight: 500 }}>{invitee(inv)}</span>
                <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>
                  {inv.failed_reason ?? "failed"} ·{" "}
                  {inv.failed_at ? new Date(inv.failed_at).toLocaleDateString() : "—"}
                </span>
                <span />
              </PersonRow>
            ))
          ))}
      </Box>

      <Box header={<span style={{ fontWeight: 600 }}>Outside collaborators</span>}>
        {outside.isLoading && <Spinner label="loading outside collaborators" />}
        {outside.isError && (
          <InlineError title="Failed to load outside collaborators" detail={String(outside.error)} />
        )}
        {outside.data &&
          (outside.data.length === 0 ? (
            <EmptyRow>No outside collaborators.</EmptyRow>
          ) : (
            outside.data.map((u: GithubAccount, i) => (
              <PersonRow key={u.id} last={i === outside.data.length - 1}>
                <span style={{ fontWeight: 500 }}>@{u.login}</span>
                <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>{u.type}</span>
                <Button
                  size="sm"
                  variant="danger"
                  disabled={removeOutsideMut.isPending}
                  onClick={() => {
                    if (confirm(`Remove ${u.login} as an outside collaborator?`)) {
                      removeOutsideMut.mutate(u.login);
                    }
                  }}
                >
                  remove
                </Button>
              </PersonRow>
            ))
          ))}
      </Box>

      <Box header={<span style={{ fontWeight: 600 }}>Blocked users</span>}>
        <div className="flex gap-2" style={{ padding: "0.75rem 1rem", borderBottom: "1px solid var(--color-border)" }}>
          <input
            aria-label="Username to block"
            placeholder="username"
            value={blockUsername}
            onChange={(e) => setBlockUsername(e.target.value)}
            className="w-full"
          />
          <Button
            variant="danger"
            size="sm"
            disabled={blockMut.isPending || !blockUsername.trim()}
            onClick={() => {
              setError(null);
              blockMut.mutate();
            }}
          >
            Block
          </Button>
        </div>
        {blocks.isLoading && <Spinner label="loading blocked users" />}
        {blocks.isError && <InlineError title="Failed to load blocked users" detail={String(blocks.error)} />}
        {blocks.data &&
          (blocks.data.length === 0 ? (
            <EmptyRow>No blocked users.</EmptyRow>
          ) : (
            blocks.data.map((u: GithubAccount, i) => (
              <PersonRow key={u.id} last={i === blocks.data.length - 1}>
                <span style={{ fontWeight: 500 }}>@{u.login}</span>
                <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>{u.type}</span>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={unblockMut.isPending}
                  onClick={() => unblockMut.mutate(u.login)}
                >
                  unblock
                </Button>
              </PersonRow>
            ))
          ))}
      </Box>
    </div>
  );
}

function PersonRow({ children, last }: { children: React.ReactNode; last: boolean }) {
  return (
    <div
      className="flex flex-wrap items-center justify-between gap-3"
      style={{
        padding: "0.6rem 1rem",
        borderBottom: last ? "none" : "1px solid var(--color-border)",
      }}
    >
      {children}
    </div>
  );
}

function EmptyRow({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ padding: "0.75rem 1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
      {children}
    </div>
  );
}

// ─── Organization roles ─────────────────────────────────────────────────

function RolesPanel({ org }: { org: string }) {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["org-roles", org],
    queryFn: () => fetchOrgRoles(org),
  });

  if (isLoading) return <Spinner label="loading organization roles" />;
  if (isError || !data) {
    return <InlineError title="Failed to load organization roles" detail={String(error)} />;
  }

  return (
    <div className="flex flex-col gap-3">
      <div style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
        {data.totalCount} predefined role{data.totalCount === 1 ? "" : "s"}. Expand a role to see and
        manage its team and user assignments.
      </div>
      {data.items.map((role) => (
        <RoleCard key={role.id} org={org} role={role} />
      ))}
    </div>
  );
}

function RoleCard({ org, role }: { org: string; role: GithubOrgRole }) {
  const [open, setOpen] = useState(false);
  return (
    <Box>
      <button
        type="button"
        className="flex w-full items-center gap-2 text-left"
        onClick={() => setOpen((v) => !v)}
        style={{ padding: "0.7rem 1rem", background: "transparent", border: "none", color: "var(--color-fg)" }}
      >
        {open ? <ChevronDownIcon size={14} /> : <ChevronRightIcon size={14} />}
        <span style={{ fontWeight: 600, fontSize: "0.9rem" }}>{role.name}</span>
        <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>
          base role: {role.base_role}
          {role.permissions.length > 0 && ` · ${role.permissions.join(", ")}`}
        </span>
      </button>
      {open && (
        <div style={{ borderTop: "1px solid var(--color-border)", padding: "0.75rem 1rem" }}>
          <div style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)", marginBottom: "0.75rem" }}>
            {role.description}
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <RoleAssignments org={org} roleId={role.id} kind="teams" />
            <RoleAssignments org={org} roleId={role.id} kind="users" />
          </div>
        </div>
      )}
    </Box>
  );
}

function RoleAssignments({ org, roleId, kind }: { org: string; roleId: number; kind: "teams" | "users" }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState("");

  const key = kind === "teams" ? ["org-role-teams", org, roleId] : ["org-role-users", org, roleId];
  const query = useQuery({
    queryKey: key,
    queryFn: async (): Promise<{ key: string; label: string; assignment: string }[]> => {
      if (kind === "teams") {
        const teams = await fetchOrgRoleTeams(org, roleId);
        return teams.map((t) => ({ key: t.slug, label: `@${t.slug}`, assignment: t.assignment }));
      }
      const users = await fetchOrgRoleUsers(org, roleId);
      return users.map((u) => ({ key: u.login, label: `@${u.login}`, assignment: u.assignment }));
    },
  });

  const assignMut = useMutation({
    mutationFn: () =>
      kind === "teams"
        ? assignOrgRoleToTeam(org, name.trim(), roleId)
        : assignOrgRoleToUser(org, name.trim(), roleId),
    onSuccess: () => {
      setError(null);
      setName("");
      qc.invalidateQueries({ queryKey: key });
    },
    onError: (err: Error) => setError(err.message),
  });
  const revokeMut = useMutation({
    mutationFn: (target: string) =>
      kind === "teams"
        ? revokeOrgRoleFromTeam(org, target, roleId)
        : revokeOrgRoleFromUser(org, target, roleId),
    onSuccess: () => {
      setError(null);
      qc.invalidateQueries({ queryKey: key });
    },
    onError: (err: Error) => setError(err.message),
  });

  if (query.isLoading) return <Spinner label={`loading role ${kind}`} />;
  if (query.isError) {
    return <InlineError title={`Failed to load assigned ${kind}`} detail={String(query.error)} />;
  }

  const entries = query.data;

  return (
    <div>
      <div style={{ fontSize: "0.8rem", fontWeight: 600, marginBottom: "0.4rem" }}>
        {kind === "teams" ? "Teams" : "Users"}
      </div>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      {!entries || entries.length === 0 ? (
        <div style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
          No {kind} assigned.
        </div>
      ) : (
        <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
          {entries.map((e) => (
            <li key={e.key} className="flex items-center justify-between gap-2 py-1" style={{ fontSize: "0.85rem" }}>
              <span>
                {e.label}{" "}
                <span style={{ color: "var(--color-fg-muted)", fontSize: "0.78rem" }}>({e.assignment})</span>
              </span>
              <Button size="sm" variant="danger" disabled={revokeMut.isPending} onClick={() => revokeMut.mutate(e.key)}>
                revoke
              </Button>
            </li>
          ))}
        </ul>
      )}
      <div className="mt-2 flex gap-2">
        <input
          aria-label={kind === "teams" ? `Team slug to assign role ${roleId}` : `Username to assign role ${roleId}`}
          placeholder={kind === "teams" ? "team-slug" : "username"}
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="w-full"
        />
        <Button
          size="sm"
          disabled={assignMut.isPending || !name.trim()}
          onClick={() => {
            setError(null);
            assignMut.mutate();
          }}
        >
          Assign
        </Button>
      </div>
    </div>
  );
}

// ─── Custom properties schema ───────────────────────────────────────────

function PropertiesPanel({ org }: { org: string }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<GithubCustomProperty | null>(null);
  const [creating, setCreating] = useState(false);

  const { data: properties, isLoading, isError, error: loadErr } = useQuery({
    queryKey: ["org-custom-properties", org],
    queryFn: () => fetchOrgCustomProperties(org),
  });

  const deleteMut = useMutation({
    mutationFn: (name: string) => deleteOrgCustomProperty(org, name),
    onSuccess: () => {
      setError(null);
      qc.invalidateQueries({ queryKey: ["org-custom-properties", org] });
    },
    onError: (err: Error) => setError(err.message),
  });

  if (isLoading) return <Spinner label="loading custom properties" />;
  if (isError || !properties) {
    return <InlineError title="Failed to load custom properties" detail={String(loadErr)} />;
  }

  return (
    <div>
      <div className="mb-3 flex items-center justify-between gap-3">
        <span style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
          Typed key/value definitions repositories in this organization can carry.
        </span>
        <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
          New property
        </Button>
      </div>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      {properties.length === 0 ? (
        <EmptyBox>No custom properties defined.</EmptyBox>
      ) : (
        <Box>
          {properties.map((p, i) => (
            <div
              key={p.property_name}
              className="flex flex-wrap items-center gap-3"
              style={{
                padding: "0.7rem 1rem",
                borderBottom: i < properties.length - 1 ? "1px solid var(--color-border)" : "none",
              }}
            >
              <span style={{ fontWeight: 600, fontSize: "0.9rem" }}>{p.property_name}</span>
              <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }} className="min-w-0 flex-1">
                {p.value_type}
                {p.required && " · required"}
                {p.default_value != null && ` · default: ${formatPropertyValue(p.default_value)}`}
                {p.allowed_values && p.allowed_values.length > 0 && ` · [${p.allowed_values.join(", ")}]`}
                {p.description && ` · ${p.description}`}
              </span>
              <Button size="sm" variant="ghost" onClick={() => setEditing(p)}>
                edit
              </Button>
              <Button
                size="sm"
                variant="danger"
                disabled={deleteMut.isPending}
                onClick={() => {
                  if (confirm(`Delete property ${p.property_name}?`)) deleteMut.mutate(p.property_name);
                }}
              >
                delete
              </Button>
            </div>
          ))}
        </Box>
      )}
      {(creating || editing) && (
        <PropertyDialog
          org={org}
          property={editing ?? undefined}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
        />
      )}
      <RepoValuesPanel org={org} properties={properties} />
    </div>
  );
}

/** Renders a custom property value (string or multi_select array) for display. */
function formatPropertyValue(value: unknown): string {
  return Array.isArray(value) ? value.join(", ") : String(value);
}

function PropertyDialog({
  org,
  property,
  onClose,
}: {
  org: string;
  property?: GithubCustomProperty;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState(property?.property_name ?? "");
  const [valueType, setValueType] = useState<GithubCustomPropertyValueType>(
    property?.value_type ?? "string",
  );
  const [description, setDescription] = useState(property?.description ?? "");
  const [allowedValues, setAllowedValues] = useState(property?.allowed_values?.join(", ") ?? "");
  const [required, setRequired] = useState(property?.required ?? false);
  const [defaultValue, setDefaultValue] = useState(
    property?.default_value != null ? formatPropertyValue(property.default_value) : "",
  );
  const [error, setError] = useState<string | null>(null);

  const isSelect = valueType === "single_select" || valueType === "multi_select";
  const parsedAllowed = allowedValues
    .split(",")
    .map((v) => v.trim())
    .filter(Boolean);

  const mutation = useMutation({
    mutationFn: () =>
      upsertOrgCustomProperties(org, [
        {
          property_name: name.trim(),
          value_type: valueType,
          required,
          default_value: defaultValue.trim() === "" ? undefined : defaultValue.trim(),
          description: description || undefined,
          allowed_values: isSelect ? parsedAllowed : undefined,
        },
      ]),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["org-custom-properties", org] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Modal title={property ? `Edit ${property.property_name}` : "New custom property"} onClose={onClose}>
      <FormLabel id="prop-name">Property name</FormLabel>
      <input
        id="prop-name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        disabled={!!property}
        className="mb-3 w-full"
      />
      <FormLabel id="prop-type">Value type</FormLabel>
      <select
        id="prop-type"
        value={valueType}
        onChange={(e) => setValueType(e.target.value as GithubCustomPropertyValueType)}
        className="mb-3 w-full"
      >
        <option value="string">string</option>
        <option value="single_select">single_select</option>
        <option value="multi_select">multi_select</option>
        <option value="true_false">true_false</option>
        <option value="url">url</option>
      </select>
      {isSelect && (
        <>
          <FormLabel id="prop-allowed">Allowed values (comma-separated)</FormLabel>
          <input
            id="prop-allowed"
            value={allowedValues}
            onChange={(e) => setAllowedValues(e.target.value)}
            className="mb-3 w-full"
          />
        </>
      )}
      <FormLabel id="prop-desc">Description (optional)</FormLabel>
      <input
        id="prop-desc"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="mb-3 w-full"
      />
      <label className="mb-3 flex items-center gap-2" style={{ fontSize: "0.85rem" }}>
        <input
          type="checkbox"
          checked={required}
          onChange={(e) => setRequired(e.target.checked)}
        />
        Required (repositories without an explicit value get the default)
      </label>
      <FormLabel id="prop-default">
        {required ? "Default value" : "Default value (optional)"}
      </FormLabel>
      {valueType === "true_false" ? (
        <select
          id="prop-default"
          value={defaultValue}
          onChange={(e) => setDefaultValue(e.target.value)}
          className="mb-4 w-full"
        >
          <option value="">no default</option>
          <option value="true">true</option>
          <option value="false">false</option>
        </select>
      ) : valueType === "single_select" ? (
        <select
          id="prop-default"
          value={defaultValue}
          onChange={(e) => setDefaultValue(e.target.value)}
          className="mb-4 w-full"
        >
          <option value="">no default</option>
          {parsedAllowed.map((v) => (
            <option key={v} value={v}>
              {v}
            </option>
          ))}
        </select>
      ) : (
        <input
          id="prop-default"
          value={defaultValue}
          onChange={(e) => setDefaultValue(e.target.value)}
          className="mb-4 w-full"
        />
      )}
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <DialogActions>
        <Button variant="ghost" size="sm" onClick={onClose} disabled={mutation.isPending}>
          Cancel
        </Button>
        <Button
          variant="primary"
          size="sm"
          disabled={!name.trim() || (required && !defaultValue.trim()) || mutation.isPending}
          onClick={() => {
            setError(null);
            mutation.mutate();
          }}
        >
          {mutation.isPending ? "Saving…" : "Save property"}
        </Button>
      </DialogActions>
    </Modal>
  );
}

function RepoValuesPanel({ org, properties }: { org: string; properties: GithubCustomProperty[] }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [queryInput, setQueryInput] = useState("");
  const [repoQuery, setRepoQuery] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [propName, setPropName] = useState(properties[0]?.property_name ?? "");
  const [valueInput, setValueInput] = useState("");

  const valuesQuery = useQuery({
    queryKey: ["org-property-values", org, repoQuery],
    queryFn: () => fetchOrgRepoCustomPropertyValues(org, repoQuery || undefined),
  });

  const selectedProp = properties.find((p) => p.property_name === propName);

  const setMut = useMutation({
    mutationFn: () => {
      // An empty input unsets the property (the PATCH's null-value contract).
      let value: unknown;
      if (valueInput.trim() === "") {
        value = null;
      } else if (selectedProp?.value_type === "multi_select") {
        value = valueInput
          .split(",")
          .map((v) => v.trim())
          .filter(Boolean);
      } else {
        value = valueInput.trim();
      }
      return setOrgRepoCustomPropertyValues(org, selected, [{ property_name: propName, value }]);
    },
    onSuccess: () => {
      setError(null);
      setSelected([]);
      qc.invalidateQueries({ queryKey: ["org-property-values", org] });
    },
    onError: (err: Error) => setError(err.message),
  });

  const toggleRepo = (name: string) =>
    setSelected((cur) => (cur.includes(name) ? cur.filter((n) => n !== name) : [...cur, name]));

  const rowSummary = (row: GithubOrgRepoCustomPropertyValues) =>
    row.properties.map((p) => `${p.property_name}=${formatPropertyValue(p.value)}`).join(", ");

  return (
    <div className="mt-4">
      <Box header={<span style={{ fontWeight: 600 }}>Repository values</span>}>
        <div style={{ padding: "0.75rem 1rem" }}>
          {error && <ErrorBanner>{error}</ErrorBanner>}
          <div className="mb-3 flex gap-2">
            <input
              aria-label="Repository search query"
              placeholder="filter repositories (repo:owner/name for an exact match)"
              value={queryInput}
              onChange={(e) => setQueryInput(e.target.value)}
              className="w-full"
            />
            <Button size="sm" onClick={() => setRepoQuery(queryInput.trim())}>
              Search
            </Button>
          </div>
          {valuesQuery.isLoading && <Spinner label="loading repository property values" />}
          {valuesQuery.isError && (
            <InlineError
              title="Failed to load repository property values"
              detail={String(valuesQuery.error)}
            />
          )}
          {valuesQuery.data &&
            (valuesQuery.data.length === 0 ? (
              <div style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
                No repositories matched.
              </div>
            ) : (
              <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
                {valuesQuery.data.map((row) => (
                  <li
                    key={row.repository_id}
                    className="flex flex-wrap items-center gap-2"
                    style={{
                      padding: "0.45rem 0",
                      borderBottom: "1px solid var(--color-border)",
                      fontSize: "0.88rem",
                    }}
                  >
                    <label className="flex min-w-0 flex-1 items-center gap-2">
                      <input
                        type="checkbox"
                        aria-label={`Select ${row.repository_full_name}`}
                        checked={selected.includes(row.repository_name)}
                        onChange={() => toggleRepo(row.repository_name)}
                      />
                      <span style={{ fontWeight: 500 }}>{row.repository_full_name}</span>
                      <span style={{ color: "var(--color-fg-muted)", fontSize: "0.8rem" }}>
                        {row.properties.length === 0 ? "no values" : rowSummary(row)}
                      </span>
                    </label>
                  </li>
                ))}
              </ul>
            ))}
          {properties.length === 0 ? (
            <div className="mt-3" style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
              Define a custom property above to set values on repositories.
            </div>
          ) : (
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <select
                aria-label="Property to set"
                value={propName}
                onChange={(e) => {
                  setPropName(e.target.value);
                  setValueInput("");
                }}
              >
                {properties.map((p) => (
                  <option key={p.property_name} value={p.property_name}>
                    {p.property_name}
                  </option>
                ))}
              </select>
              {selectedProp?.value_type === "single_select" ? (
                <select
                  aria-label="Property value"
                  value={valueInput}
                  onChange={(e) => setValueInput(e.target.value)}
                >
                  <option value="">unset</option>
                  {(selectedProp.allowed_values ?? []).map((v) => (
                    <option key={v} value={v}>
                      {v}
                    </option>
                  ))}
                </select>
              ) : selectedProp?.value_type === "true_false" ? (
                <select
                  aria-label="Property value"
                  value={valueInput}
                  onChange={(e) => setValueInput(e.target.value)}
                >
                  <option value="">unset</option>
                  <option value="true">true</option>
                  <option value="false">false</option>
                </select>
              ) : (
                <input
                  aria-label="Property value"
                  placeholder={
                    selectedProp?.value_type === "multi_select"
                      ? "values, comma-separated (empty unsets)"
                      : "value (empty unsets)"
                  }
                  value={valueInput}
                  onChange={(e) => setValueInput(e.target.value)}
                  className="min-w-0 flex-1"
                />
              )}
              <Button
                size="sm"
                variant="primary"
                disabled={
                  setMut.isPending || !propName || selected.length === 0 || selected.length > 30
                }
                onClick={() => {
                  setError(null);
                  setMut.mutate();
                }}
              >
                {valueInput.trim() === "" ? "Unset on selected" : "Set on selected"}
              </Button>
              <span style={{ color: "var(--color-fg-muted)", fontSize: "0.78rem" }}>
                {selected.length} selected (max 30)
              </span>
            </div>
          )}
        </div>
      </Box>
    </div>
  );
}

// ─── Issue types ────────────────────────────────────────────────────────

const ISSUE_TYPE_COLORS = ["gray", "blue", "green", "yellow", "orange", "red", "pink", "purple"];

function IssueTypesPanel({ org }: { org: string }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [color, setColor] = useState("gray");
  const [description, setDescription] = useState("");

  const { data: issueTypes, isLoading, isError, error: loadErr } = useQuery({
    queryKey: ["org-issue-types", org],
    queryFn: () => fetchOrgIssueTypes(org),
  });

  const invalidate = () => {
    setError(null);
    qc.invalidateQueries({ queryKey: ["org-issue-types", org] });
  };
  const createMut = useMutation({
    mutationFn: () =>
      createOrgIssueType(org, {
        name: name.trim(),
        is_enabled: true,
        color,
        description: description || undefined,
      }),
    onSuccess: () => {
      invalidate();
      setName("");
      setDescription("");
    },
    onError: (err: Error) => setError(err.message),
  });
  const toggleMut = useMutation({
    mutationFn: (it: GithubIssueType) =>
      updateOrgIssueType(org, it.id, {
        name: it.name,
        is_enabled: !it.is_enabled,
        color: it.color ?? undefined,
        description: it.description ?? undefined,
      }),
    onSuccess: invalidate,
    onError: (err: Error) => setError(err.message),
  });
  const deleteMut = useMutation({
    mutationFn: (id: number) => deleteOrgIssueType(org, id),
    onSuccess: invalidate,
    onError: (err: Error) => setError(err.message),
  });

  if (isLoading) return <Spinner label="loading issue types" />;
  if (isError || !issueTypes) {
    return <InlineError title="Failed to load issue types" detail={String(loadErr)} />;
  }

  return (
    <div className="flex flex-col gap-4">
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <Box header={<span style={{ fontWeight: 600 }}>New issue type</span>}>
        <div className="flex flex-wrap gap-2" style={{ padding: "0.75rem 1rem" }}>
          <input
            aria-label="Issue type name"
            placeholder="Name (e.g. Bug)"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <select aria-label="Issue type color" value={color} onChange={(e) => setColor(e.target.value)}>
            {ISSUE_TYPE_COLORS.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
          <input
            aria-label="Issue type description"
            placeholder="Description (optional)"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className="min-w-0 flex-1"
          />
          <Button
            variant="primary"
            size="sm"
            disabled={createMut.isPending || !name.trim()}
            onClick={() => {
              setError(null);
              createMut.mutate();
            }}
          >
            Create
          </Button>
        </div>
      </Box>
      {issueTypes.length === 0 ? (
        <EmptyBox>No issue types defined.</EmptyBox>
      ) : (
        <Box>
          {issueTypes.map((it, i) => (
            <div
              key={it.id}
              className="flex flex-wrap items-center gap-3"
              style={{
                padding: "0.7rem 1rem",
                borderBottom: i < issueTypes.length - 1 ? "1px solid var(--color-border)" : "none",
              }}
            >
              <span style={{ fontWeight: 600, fontSize: "0.9rem" }}>{it.name}</span>
              <span className="min-w-0 flex-1" style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>
                {it.color ?? "no color"}
                {it.description && ` · ${it.description}`}
                {!it.is_enabled && " · disabled"}
              </span>
              <Button size="sm" variant="ghost" disabled={toggleMut.isPending} onClick={() => toggleMut.mutate(it)}>
                {it.is_enabled ? "disable" : "enable"}
              </Button>
              <Button
                size="sm"
                variant="danger"
                disabled={deleteMut.isPending}
                onClick={() => {
                  if (confirm(`Delete issue type ${it.name}?`)) deleteMut.mutate(it.id);
                }}
              >
                delete
              </Button>
            </div>
          ))}
        </Box>
      )}
    </div>
  );
}

function EmptyBox({ children }: { children: React.ReactNode }) {
  return (
    <Box>
      <div style={{ padding: "0.9rem 1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
        {children}
      </div>
    </Box>
  );
}
