import { useState } from "react";
import { useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchCopilotBilling,
  fetchCopilotSeats,
  addCopilotSeats,
  cancelCopilotSeats,
  fetchCopilotSpaces,
  createCopilotSpace,
  updateCopilotSpace,
  deleteCopilotSpace,
  fetchCopilotSpaceCollaborators,
  addCopilotSpaceCollaborator,
  updateCopilotSpaceCollaborator,
  removeCopilotSpaceCollaborator,
  fetchCopilotSpaceResources,
  addCopilotSpaceResource,
  updateCopilotSpaceResource,
  removeCopilotSpaceResource,
} from "../api.js";
import type {
  GithubCopilotSpace,
  GithubCopilotSpaceCollaborator,
  GithubCopilotSpaceResource,
} from "../types.js";
import { OrgHeader } from "../components/Shell.js";
import {
  Box,
  Button,
  DialogActions,
  ErrorBanner,
  FormLabel,
  Modal,
  SectionLabel,
  StatCard,
} from "../components/ui.js";
import { ChevronDownIcon, ChevronRightIcon, CommentIcon } from "../components/octicons.js";
import { Blankslate } from "../components/ui.js";

export function CopilotPage() {
  const { org = "" } = useParams<{ org: string }>();

  return (
    <div>
      <OrgHeader org={org} active="copilot" />
      <div className="flex flex-col gap-6">
        <BillingSection org={org} />
        <SeatsSection org={org} />
        <SpacesSection org={org} />
      </div>
    </div>
  );
}

function BillingSection({ org }: { org: string }) {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["copilot-billing", org],
    queryFn: () => fetchCopilotBilling(org),
  });

  return (
    <section>
      <SectionLabel>Billing</SectionLabel>
      {isLoading && <Spinner label="loading Copilot billing" />}
      {isError && <InlineError title="Failed to load Copilot billing" detail={String(error)} />}
      {data && (
        <>
          <div className="mb-3 grid gap-3 sm:grid-cols-4">
            <StatCard title="Total seats" value={data.seat_breakdown.total} emphasized />
            <StatCard title="Added this cycle" value={data.seat_breakdown.added_this_cycle} />
            <StatCard title="Pending invitation" value={data.seat_breakdown.pending_invitation} />
            <StatCard title="Pending cancellation" value={data.seat_breakdown.pending_cancellation} />
          </div>
          <Box>
            <div className="flex flex-wrap gap-x-6 gap-y-1" style={{ padding: "0.75rem 1rem", fontSize: "0.85rem" }}>
              <span>
                Plan: <strong>{data.plan_type}</strong>
              </span>
              <span>
                Seat management: <strong>{data.seat_management_setting}</strong>
              </span>
              <span>
                Public code suggestions: <strong>{data.public_code_suggestions}</strong>
              </span>
              <span>
                IDE chat: <strong>{data.ide_chat}</strong>
              </span>
              <span>
                Platform chat: <strong>{data.platform_chat}</strong>
              </span>
              <span>
                CLI: <strong>{data.cli}</strong>
              </span>
            </div>
          </Box>
        </>
      )}
    </section>
  );
}

function SeatsSection({ org }: { org: string }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [usernames, setUsernames] = useState("");

  const { data, isLoading, isError, error: loadErr } = useQuery({
    queryKey: ["copilot-seats", org],
    queryFn: () => fetchCopilotSeats(org),
  });

  const invalidate = () => {
    setError(null);
    qc.invalidateQueries({ queryKey: ["copilot-seats", org] });
    qc.invalidateQueries({ queryKey: ["copilot-billing", org] });
  };
  const addMut = useMutation({
    mutationFn: () =>
      addCopilotSeats(
        org,
        usernames
          .split(",")
          .map((u) => u.trim())
          .filter(Boolean),
      ),
    onSuccess: () => {
      invalidate();
      setUsernames("");
    },
    onError: (err: Error) => setError(err.message),
  });
  const removeMut = useMutation({
    mutationFn: (login: string) => cancelCopilotSeats(org, [login]),
    onSuccess: invalidate,
    onError: (err: Error) => setError(err.message),
  });

  return (
    <section>
      <SectionLabel>Seats</SectionLabel>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <Box>
        <div className="flex gap-2" style={{ padding: "0.75rem 1rem", borderBottom: "1px solid var(--color-border)" }}>
          <input
            aria-label="Usernames to assign Copilot seats"
            placeholder="usernames, comma-separated"
            value={usernames}
            onChange={(e) => setUsernames(e.target.value)}
            className="w-full"
          />
          <Button
            variant="primary"
            size="sm"
            disabled={addMut.isPending || !usernames.trim()}
            onClick={() => {
              setError(null);
              addMut.mutate();
            }}
          >
            Assign seats
          </Button>
        </div>
        {isLoading && <Spinner label="loading Copilot seats" />}
        {isError && <InlineError title="Failed to load Copilot seats" detail={String(loadErr)} />}
        {data &&
          (data.seats.length === 0 ? (
            <div style={{ padding: "0.9rem 1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
              No Copilot seats assigned.
            </div>
          ) : (
            data.seats.map((seat, i) => {
              const login = seat.assignee?.login;
              return (
              <div
                key={login ?? i}
                className="flex flex-wrap items-center justify-between gap-3"
                style={{
                  padding: "0.6rem 1rem",
                  borderBottom: i < data.seats.length - 1 ? "1px solid var(--color-border)" : "none",
                }}
              >
                <span style={{ fontWeight: 500 }}>
                  {login ? `@${login}` : "(unresolved assignee)"}
                </span>
                <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>
                  {seat.plan_type} · since {new Date(seat.created_at).toLocaleDateString()}
                  {seat.assigning_team && ` · via team @${seat.assigning_team.slug}`}
                  {seat.pending_cancellation_date && ` · cancels ${seat.pending_cancellation_date}`}
                </span>
                {login && (
                  <Button
                    size="sm"
                    variant="danger"
                    disabled={removeMut.isPending}
                    onClick={() => {
                      if (confirm(`Cancel ${login}'s Copilot seat?`)) {
                        removeMut.mutate(login);
                      }
                    }}
                  >
                    cancel seat
                  </Button>
                )}
              </div>
              );
            })
          ))}
      </Box>
    </section>
  );
}

function SpacesSection({ org }: { org: string }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<GithubCopilotSpace | null>(null);

  const { data, isLoading, isError, error: loadErr } = useQuery({
    queryKey: ["copilot-spaces", org],
    queryFn: () => fetchCopilotSpaces(org),
  });

  const deleteMut = useMutation({
    mutationFn: (spaceNumber: number) => deleteCopilotSpace(org, spaceNumber),
    onSuccess: () => {
      setError(null);
      qc.invalidateQueries({ queryKey: ["copilot-spaces", org] });
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <section>
      <div className="flex items-center justify-between gap-3">
        <SectionLabel>Copilot Spaces</SectionLabel>
        <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
          New space
        </Button>
      </div>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      {isLoading && <Spinner label="loading Copilot Spaces" />}
      {isError && <InlineError title="Failed to load Copilot Spaces" detail={String(loadErr)} />}
      {data &&
        (data.length === 0 ? (
          <Blankslate icon={<CommentIcon size={26} />} title="No Copilot Spaces">
            Spaces bundle repositories, files, and instructions into a shared Copilot context.
          </Blankslate>
        ) : (
          <div className="flex flex-col gap-2">
            {data.map((space) => (
              <SpaceCard
                key={space.id}
                org={org}
                space={space}
                onEdit={() => setEditing(space)}
                onDelete={() => {
                  if (confirm(`Delete Copilot Space ${space.name}?`)) deleteMut.mutate(space.number);
                }}
                deleting={deleteMut.isPending}
              />
            ))}
          </div>
        ))}
      {(creating || editing) && (
        <SpaceDialog
          org={org}
          space={editing ?? undefined}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
        />
      )}
    </section>
  );
}

function SpaceCard({
  org,
  space,
  onEdit,
  onDelete,
  deleting,
}: {
  org: string;
  space: GithubCopilotSpace;
  onEdit: () => void;
  onDelete: () => void;
  deleting: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Box>
      <div className="flex flex-wrap items-center gap-3" style={{ padding: "0.6rem 1rem" }}>
        <button
          type="button"
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
          onClick={() => setOpen((v) => !v)}
          style={{ background: "transparent", border: "none", color: "var(--color-fg)", padding: 0 }}
        >
          {open ? <ChevronDownIcon size={14} /> : <ChevronRightIcon size={14} />}
          <span style={{ fontWeight: 600, fontSize: "0.9rem" }}>
            {space.name}
            <span style={{ color: "var(--color-fg-muted)", fontWeight: 400 }}> #{space.number}</span>
          </span>
          <span className="min-w-0 flex-1" style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>
            {space.description ?? "No description"}
          </span>
          <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>
            base role: {space.base_role}
            {space.creator && ` · created by @${space.creator.login}`} ·{" "}
            {new Date(space.updated_at).toLocaleDateString()}
          </span>
        </button>
        <Button size="sm" variant="ghost" onClick={onEdit}>
          edit
        </Button>
        <Button size="sm" variant="danger" disabled={deleting} onClick={onDelete}>
          delete
        </Button>
      </div>
      {open && (
        <div
          className="flex flex-col gap-4"
          style={{ borderTop: "1px solid var(--color-border)", padding: "0.75rem 1rem" }}
        >
          {space.general_instructions && (
            <div style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
              {space.general_instructions}
            </div>
          )}
          <SpaceCollaboratorsPanel org={org} spaceNumber={space.number} />
          <SpaceResourcesPanel org={org} spaceNumber={space.number} />
        </div>
      )}
    </Box>
  );
}

const COPILOT_SPACE_ROLES = ["reader", "writer", "admin"];

function SpaceDialog({
  org,
  space,
  onClose,
}: {
  org: string;
  space?: GithubCopilotSpace;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState(space?.name ?? "");
  const [description, setDescription] = useState(space?.description ?? "");
  const [instructions, setInstructions] = useState(space?.general_instructions ?? "");
  const [baseRole, setBaseRole] = useState(space?.base_role ?? "no_access");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      const payload = {
        name: name.trim(),
        description,
        general_instructions: instructions,
        base_role: baseRole,
      };
      return space ? updateCopilotSpace(org, space.number, payload) : createCopilotSpace(org, payload);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["copilot-spaces", org] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Modal title={space ? `Edit ${space.name}` : "New Copilot Space"} onClose={onClose}>
      <FormLabel id="space-name">Name</FormLabel>
      <input
        id="space-name"
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
        className="mb-3 w-full"
      />
      <FormLabel id="space-desc">Description (optional)</FormLabel>
      <input
        id="space-desc"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="mb-3 w-full"
      />
      <FormLabel id="space-instructions">General instructions (optional)</FormLabel>
      <textarea
        id="space-instructions"
        value={instructions}
        onChange={(e) => setInstructions(e.target.value)}
        rows={3}
        className="mb-3 w-full"
      />
      <FormLabel id="space-base-role">Base role for organization members</FormLabel>
      <select
        id="space-base-role"
        value={baseRole}
        onChange={(e) => setBaseRole(e.target.value)}
        className="mb-4 w-full"
      >
        <option value="no_access">no_access</option>
        {COPILOT_SPACE_ROLES.map((r) => (
          <option key={r} value={r}>
            {r}
          </option>
        ))}
      </select>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <DialogActions>
        <Button variant="ghost" size="sm" onClick={onClose} disabled={mutation.isPending}>
          Cancel
        </Button>
        <Button
          variant="primary"
          size="sm"
          disabled={!name.trim() || mutation.isPending}
          onClick={() => {
            setError(null);
            mutation.mutate();
          }}
        >
          {mutation.isPending ? "Saving…" : space ? "Save" : "Create space"}
        </Button>
      </DialogActions>
    </Modal>
  );
}

/** The path identifier the collaborator endpoints key on: login for users, slug for teams. */
function collaboratorIdentifier(c: GithubCopilotSpaceCollaborator): string {
  return (c.actor_type === "Team" ? c.slug : c.login) ?? String(c.id);
}

function SpaceCollaboratorsPanel({ org, spaceNumber }: { org: string; spaceNumber: number }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [actorType, setActorType] = useState<"User" | "Team">("User");
  const [identifier, setIdentifier] = useState("");
  const [role, setRole] = useState("reader");

  const key = ["copilot-space-collaborators", org, spaceNumber];
  const { data, isLoading, isError, error: loadErr } = useQuery({
    queryKey: key,
    queryFn: () => fetchCopilotSpaceCollaborators(org, spaceNumber),
  });

  const invalidate = () => {
    setError(null);
    qc.invalidateQueries({ queryKey: key });
  };
  const addMut = useMutation({
    mutationFn: () =>
      addCopilotSpaceCollaborator(org, spaceNumber, {
        actor_type: actorType,
        actor_identifier: identifier.trim(),
        role,
      }),
    onSuccess: () => {
      invalidate();
      setIdentifier("");
    },
    onError: (err: Error) => setError(err.message),
  });
  const roleMut = useMutation({
    mutationFn: ({ c, newRole }: { c: GithubCopilotSpaceCollaborator; newRole: string }) =>
      updateCopilotSpaceCollaborator(org, spaceNumber, c.actor_type, collaboratorIdentifier(c), newRole),
    onSuccess: invalidate,
    onError: (err: Error) => setError(err.message),
  });
  const removeMut = useMutation({
    mutationFn: (c: GithubCopilotSpaceCollaborator) =>
      removeCopilotSpaceCollaborator(org, spaceNumber, c.actor_type, collaboratorIdentifier(c)),
    onSuccess: invalidate,
    onError: (err: Error) => setError(err.message),
  });

  return (
    <div>
      <div style={{ fontSize: "0.8rem", fontWeight: 600, marginBottom: "0.4rem" }}>Collaborators</div>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      {isLoading && <Spinner label="loading space collaborators" />}
      {isError && <InlineError title="Failed to load collaborators" detail={String(loadErr)} />}
      {data &&
        (data.length === 0 ? (
          <div style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
            No collaborators granted beyond the base role.
          </div>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {data.map((c) => {
              const label = c.actor_type === "Team" ? `@${c.slug} (team)` : `@${c.login}`;
              return (
                <li
                  key={`${c.actor_type}-${c.id}`}
                  className="flex flex-wrap items-center justify-between gap-2 py-1"
                  style={{ fontSize: "0.85rem" }}
                >
                  <span style={{ fontWeight: 500 }}>{label}</span>
                  <span className="flex items-center gap-2">
                    <select
                      aria-label={`Role for ${label}`}
                      value={c.role}
                      disabled={roleMut.isPending}
                      onChange={(e) => roleMut.mutate({ c, newRole: e.target.value })}
                    >
                      {COPILOT_SPACE_ROLES.map((r) => (
                        <option key={r} value={r}>
                          {r}
                        </option>
                      ))}
                    </select>
                    <Button
                      size="sm"
                      variant="danger"
                      disabled={removeMut.isPending}
                      onClick={() => removeMut.mutate(c)}
                    >
                      remove
                    </Button>
                  </span>
                </li>
              );
            })}
          </ul>
        ))}
      <div className="mt-2 flex flex-wrap gap-2">
        <select
          aria-label="Collaborator actor type"
          value={actorType}
          onChange={(e) => setActorType(e.target.value as "User" | "Team")}
        >
          <option value="User">User</option>
          <option value="Team">Team</option>
        </select>
        <input
          aria-label="Collaborator username or team slug"
          placeholder={actorType === "Team" ? "team-slug" : "username"}
          value={identifier}
          onChange={(e) => setIdentifier(e.target.value)}
          className="min-w-0 flex-1"
        />
        <select aria-label="Collaborator role" value={role} onChange={(e) => setRole(e.target.value)}>
          {COPILOT_SPACE_ROLES.map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </select>
        <Button
          size="sm"
          disabled={addMut.isPending || !identifier.trim()}
          onClick={() => {
            setError(null);
            addMut.mutate();
          }}
        >
          Add
        </Button>
      </div>
    </div>
  );
}

const SPACE_RESOURCE_TYPES = [
  "repository",
  "github_file",
  "github_issue",
  "github_pull_request",
  "free_text",
] as const;

type SpaceResourceType = (typeof SPACE_RESOURCE_TYPES)[number];

function resourceSummary(res: GithubCopilotSpaceResource): string {
  const m = res.metadata;
  switch (res.resource_type) {
    case "repository":
      return `repository #${String(m.repository_id)}`;
    case "github_file":
      return `file ${String(m.file_path)} in repository #${String(m.repository_id)}`;
    case "github_issue":
      return `issue #${String(m.number)} in repository #${String(m.repository_id)}`;
    case "github_pull_request":
      return `pull request #${String(m.number)} in repository #${String(m.repository_id)}`;
    case "free_text":
      return String(m.text);
    default:
      return JSON.stringify(m);
  }
}

function SpaceResourcesPanel({ org, spaceNumber }: { org: string; spaceNumber: number }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<GithubCopilotSpaceResource | null>(null);

  const key = ["copilot-space-resources", org, spaceNumber];
  const { data, isLoading, isError, error: loadErr } = useQuery({
    queryKey: key,
    queryFn: () => fetchCopilotSpaceResources(org, spaceNumber),
  });

  const removeMut = useMutation({
    mutationFn: (resourceId: number) => removeCopilotSpaceResource(org, spaceNumber, resourceId),
    onSuccess: () => {
      setError(null);
      qc.invalidateQueries({ queryKey: key });
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <div>
      <div style={{ fontSize: "0.8rem", fontWeight: 600, marginBottom: "0.4rem" }}>Resources</div>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      {isLoading && <Spinner label="loading space resources" />}
      {isError && <InlineError title="Failed to load resources" detail={String(loadErr)} />}
      {data &&
        (data.length === 0 ? (
          <div style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
            No resources attached.
          </div>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {data.map((res) => (
              <li
                key={res.id}
                className="flex flex-wrap items-center justify-between gap-2 py-1"
                style={{ fontSize: "0.85rem" }}
              >
                <span className="min-w-0 flex-1">
                  <span style={{ fontWeight: 500 }}>{res.resource_type}</span>{" "}
                  <span style={{ color: "var(--color-fg-muted)" }}>{resourceSummary(res)}</span>
                </span>
                <Button size="sm" variant="ghost" onClick={() => setEditing(res)}>
                  edit
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  disabled={removeMut.isPending}
                  onClick={() => removeMut.mutate(res.id)}
                >
                  remove
                </Button>
              </li>
            ))}
          </ul>
        ))}
      {/* Keyed on the edited resource so switching rows remounts with fresh fields. */}
      <SpaceResourceForm
        key={editing?.id ?? "new"}
        org={org}
        spaceNumber={spaceNumber}
        resource={editing ?? undefined}
        onDone={() => setEditing(null)}
      />
    </div>
  );
}

function SpaceResourceForm({
  org,
  spaceNumber,
  resource,
  onDone,
}: {
  org: string;
  spaceNumber: number;
  /** When set the form edits this resource's metadata (the type is fixed). */
  resource?: GithubCopilotSpaceResource;
  onDone: () => void;
}) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [resourceType, setResourceType] = useState<SpaceResourceType>("repository");
  const [repositoryId, setRepositoryId] = useState(
    resource?.metadata.repository_id != null ? String(resource.metadata.repository_id) : "",
  );
  const [filePath, setFilePath] = useState(
    typeof resource?.metadata.file_path === "string" ? resource.metadata.file_path : "",
  );
  const [itemNumber, setItemNumber] = useState(
    resource?.metadata.number != null ? String(resource.metadata.number) : "",
  );
  const [text, setText] = useState(
    typeof resource?.metadata.text === "string" ? resource.metadata.text : "",
  );

  const effectiveType = resource ? (resource.resource_type as SpaceResourceType) : resourceType;
  const needsRepo = effectiveType !== "free_text";
  const needsPath = effectiveType === "github_file";
  const needsNumber = effectiveType === "github_issue" || effectiveType === "github_pull_request";
  const needsText = effectiveType === "free_text";

  const metadata = (): Record<string, unknown> => {
    const m: Record<string, unknown> = {};
    if (needsRepo) m.repository_id = parseInt(repositoryId, 10);
    if (needsPath) m.file_path = filePath.trim();
    if (needsNumber) m.number = parseInt(itemNumber, 10);
    if (needsText) m.text = text;
    return m;
  };
  const valid =
    (!needsRepo || Number.isFinite(parseInt(repositoryId, 10))) &&
    (!needsPath || filePath.trim() !== "") &&
    (!needsNumber || Number.isFinite(parseInt(itemNumber, 10))) &&
    (!needsText || text.trim() !== "");

  const saveMut = useMutation({
    mutationFn: () =>
      resource
        ? updateCopilotSpaceResource(org, spaceNumber, resource.id, metadata())
        : addCopilotSpaceResource(org, spaceNumber, { resource_type: effectiveType, metadata: metadata() }),
    onSuccess: () => {
      setError(null);
      setRepositoryId("");
      setFilePath("");
      setItemNumber("");
      setText("");
      qc.invalidateQueries({ queryKey: ["copilot-space-resources", org, spaceNumber] });
      onDone();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <div className="mt-2">
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <div className="flex flex-wrap gap-2">
        <select
          aria-label="Resource type"
          value={effectiveType}
          disabled={!!resource}
          onChange={(e) => setResourceType(e.target.value as SpaceResourceType)}
        >
          {SPACE_RESOURCE_TYPES.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
        {needsRepo && (
          <input
            aria-label="Resource repository ID"
            type="number"
            min={1}
            placeholder="repository ID"
            value={repositoryId}
            onChange={(e) => setRepositoryId(e.target.value)}
            style={{ width: "9rem" }}
          />
        )}
        {needsPath && (
          <input
            aria-label="Resource file path"
            placeholder="path/to/file"
            value={filePath}
            onChange={(e) => setFilePath(e.target.value)}
            className="min-w-0 flex-1"
          />
        )}
        {needsNumber && (
          <input
            aria-label={effectiveType === "github_issue" ? "Resource issue number" : "Resource pull request number"}
            type="number"
            min={1}
            placeholder="number"
            value={itemNumber}
            onChange={(e) => setItemNumber(e.target.value)}
            style={{ width: "7rem" }}
          />
        )}
        {needsText && (
          <input
            aria-label="Resource free text"
            placeholder="free text"
            value={text}
            onChange={(e) => setText(e.target.value)}
            className="min-w-0 flex-1"
          />
        )}
        <Button
          size="sm"
          disabled={saveMut.isPending || !valid}
          onClick={() => {
            setError(null);
            saveMut.mutate();
          }}
        >
          {resource ? "Save resource" : "Attach"}
        </Button>
        {resource && (
          <Button size="sm" variant="ghost" disabled={saveMut.isPending} onClick={onDone}>
            Cancel
          </Button>
        )}
      </div>
    </div>
  );
}
