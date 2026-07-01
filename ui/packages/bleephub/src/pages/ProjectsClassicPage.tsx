import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import { RepoHeader } from "../components/Shell.js";
import { PageTitle, Button, Box } from "../components/ui.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import {
  fetchProjectsClassic,
  createProjectClassic,
  updateProjectClassic,
  deleteProjectClassic,
  fetchProjectColumns,
  createProjectColumn,
  updateProjectColumn,
  deleteProjectColumn,
  moveProjectColumn,
  fetchProjectCards,
  createProjectCard,
  updateProjectCard,
  deleteProjectCard,
  moveProjectCard,
} from "../api.js";
import type {
  GithubProjectClassic,
  GithubProjectColumn,
  GithubProjectCard,
} from "../types.js";

export function ProjectsClassicPage() {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  const [selectedProject, setSelectedProject] = useState<GithubProjectClassic | null>(null);
  const counts = useOpenCounts(owner, repo);

  const { data: projects = [], isLoading, isError, error } = useQuery({
    queryKey: ["projects-classic", owner, repo],
    queryFn: () => fetchProjectsClassic(owner, repo),
    enabled: !!owner && !!repo,
  });

  useEffect(() => {
    if (projects.length > 0 && !selectedProject) {
      setSelectedProject(projects[0]);
    }
  }, [projects, selectedProject]);

  if (isLoading) return <Spinner label={`loading ${owner}/${repo}`} />;
  if (isError)
    return <InlineError title={`Failed to load ${owner}/${repo}`} detail={String(error)} />;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="projects-classic" {...counts} />
      <PageTitle title="Projects (classic)" />
      <ProjectList
        owner={owner}
        repo={repo}
        projects={projects}
        selected={selectedProject}
        onSelect={setSelectedProject}
      />
      {selectedProject && (
        <ProjectBoard
          owner={owner}
          repo={repo}
          project={selectedProject}
        />
      )}
    </div>
  );
}

function ProjectList({
  owner,
  repo,
  projects,
  selected,
  onSelect,
}: {
  owner: string;
  repo: string;
  projects: GithubProjectClassic[];
  selected: GithubProjectClassic | null;
  onSelect: (p: GithubProjectClassic) => void;
}) {
  const queryClient = useQueryClient();
  const [isCreating, setIsCreating] = useState(false);
  const [name, setName] = useState("");
  const [body, setBody] = useState("");

  const create = useMutation({
    mutationFn: (payload: { name: string; body: string }) =>
      createProjectClassic(owner, repo, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects-classic", owner, repo] });
      setIsCreating(false);
      setName("");
      setBody("");
    },
  });

  return (
    <Box header={<span style={{ fontWeight: 600 }}>Projects</span>} className="mb-4">
      <div style={{ padding: "1rem" }}>
        {projects.length === 0 && !isCreating && (
          <div className="mb-3" style={{ color: "var(--color-fg-muted)", fontSize: "0.9rem" }}>
            No projects yet.
          </div>
        )}
        <div className="mb-3 flex flex-wrap gap-2">
          {projects.map((p) => (
            <button
              key={p.id}
              type="button"
              onClick={() => onSelect(p)}
              style={{
                padding: "0.35rem 0.7rem",
                fontSize: "0.85rem",
                borderRadius: "var(--radius-md)",
                border: "1px solid var(--color-border)",
                background: selected?.id === p.id ? "var(--color-accent-soft)" : "var(--color-bg-subtle)",
                color: selected?.id === p.id ? "var(--color-accent)" : "var(--color-fg)",
                cursor: "pointer",
              }}
            >
              {p.name} <span style={{ color: "var(--color-fg-muted)" }}>({p.state})</span>
            </button>
          ))}
        </div>
        {isCreating ? (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              create.mutate({ name, body });
            }}
            style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}
          >
            <input
              placeholder="Project name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
            />
            <textarea
              placeholder="Description"
              value={body}
              onChange={(e) => setBody(e.target.value)}
              rows={3}
              style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
            />
            <div className="flex gap-2">
              <Button type="submit" variant="primary" disabled={!name.trim() || create.isPending}>
                {create.isPending ? "Creating..." : "Create"}
              </Button>
              <Button type="button" variant="secondary" onClick={() => setIsCreating(false)}>
                Cancel
              </Button>
            </div>
            {create.isError && (
              <div style={{ color: "var(--color-danger-fg)", fontSize: "0.85rem" }}>
                {create.error instanceof Error ? create.error.message : String(create.error)}
              </div>
            )}
          </form>
        ) : (
          <Button variant="secondary" size="sm" onClick={() => setIsCreating(true)}>
            New project
          </Button>
        )}
      </div>
    </Box>
  );
}

function ProjectBoard({
  owner,
  repo,
  project,
}: {
  owner: string;
  repo: string;
  project: GithubProjectClassic;
}) {
  const queryClient = useQueryClient();
  const [isEditing, setIsEditing] = useState(false);
  const [name, setName] = useState(project.name);
  const [body, setBody] = useState(project.body);

  useEffect(() => {
    setName(project.name);
    setBody(project.body);
  }, [project]);

  const update = useMutation({
    mutationFn: (payload: Partial<{ name: string; body: string; state: "open" | "closed" }>) =>
      updateProjectClassic(project.id, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects-classic", owner, repo] });
      setIsEditing(false);
    },
  });

  const remove = useMutation({
    mutationFn: () => deleteProjectClassic(project.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects-classic", owner, repo] });
    },
  });

  return (
    <div>
      {isEditing ? (
        <Box header={<span style={{ fontWeight: 600 }}>Edit project</span>} className="mb-4">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              update.mutate({ name, body, state: project.state });
            }}
            style={{ padding: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}
          >
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
            />
            <textarea
              value={body}
              onChange={(e) => setBody(e.target.value)}
              rows={3}
              style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
            />
            <div className="flex gap-2">
              <Button type="submit" variant="primary" disabled={update.isPending}>
                Save
              </Button>
              <Button type="button" variant="secondary" onClick={() => setIsEditing(false)}>
                Cancel
              </Button>
            </div>
          </form>
        </Box>
      ) : (
        <div className="mb-4 flex items-center justify-between">
          <div>
            <div style={{ fontSize: "1.1rem", fontWeight: 600 }}>{project.name}</div>
            {project.body && (
              <div style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
                {project.body}
              </div>
            )}
          </div>
          <div className="flex gap-2">
            <Button variant="secondary" size="sm" onClick={() => setIsEditing(true)}>
              Edit
            </Button>
            <Button
              variant="danger"
              size="sm"
              onClick={() => {
                if (window.confirm(`Delete project "${project.name}"?`)) {
                  remove.mutate();
                }
              }}
              disabled={remove.isPending}
            >
              {remove.isPending ? "Deleting..." : "Delete"}
            </Button>
          </div>
        </div>
      )}
      <ColumnsBoard owner={owner} repo={repo} project={project} />
    </div>
  );
}

function ColumnsBoard({
  owner,
  repo,
  project,
}: {
  owner: string;
  repo: string;
  project: GithubProjectClassic;
}) {
  const queryClient = useQueryClient();
  const { data: columns = [], isLoading } = useQuery({
    queryKey: ["project-columns", project.id],
    queryFn: () => fetchProjectColumns(project.id),
  });
  const [newColName, setNewColName] = useState("");

  const createCol = useMutation({
    mutationFn: (name: string) => createProjectColumn(project.id, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project-columns", project.id] });
      setNewColName("");
    },
  });

  if (isLoading) return <Spinner label="loading columns" />;

  return (
    <div style={{ display: "flex", gap: "1rem", overflowX: "auto" }}>
      {columns.map((col) => (
        <ColumnCard
          key={col.id}
          owner={owner}
          repo={repo}
          project={project}
          column={col}
          columns={columns}
        />
      ))}
      <Box
        header={<span style={{ fontWeight: 600 }}>Add column</span>}
        style={{ minWidth: 260, maxWidth: 260 }}
      >
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (newColName.trim()) createCol.mutate(newColName.trim());
          }}
          style={{ padding: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}
        >
          <input
            placeholder="Column name"
            value={newColName}
            onChange={(e) => setNewColName(e.target.value)}
            style={{ fontSize: "0.85rem", padding: "0.35rem 0.5rem" }}
          />
          <Button type="submit" variant="secondary" size="sm" disabled={createCol.isPending}>
            {createCol.isPending ? "Adding..." : "Add"}
          </Button>
        </form>
      </Box>
    </div>
  );
}

function ColumnCard({
  owner,
  repo,
  project,
  column,
  columns,
}: {
  owner: string;
  repo: string;
  project: GithubProjectClassic;
  column: GithubProjectColumn;
  columns: GithubProjectColumn[];
}) {
  const queryClient = useQueryClient();
  const { data: cards = [], isLoading } = useQuery({
    queryKey: ["project-cards", column.id],
    queryFn: () => fetchProjectCards(column.id),
  });
  const [isEditing, setIsEditing] = useState(false);
  const [name, setName] = useState(column.name);
  const [newNote, setNewNote] = useState("");

  const update = useMutation({
    mutationFn: (n: string) => updateProjectColumn(column.id, n),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project-columns", project.id] });
      setIsEditing(false);
    },
  });

  const remove = useMutation({
    mutationFn: () => deleteProjectColumn(column.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project-columns", project.id] });
    },
  });

  const createCard = useMutation({
    mutationFn: (note: string) => createProjectCard(column.id, { note }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project-cards", column.id] });
      setNewNote("");
    },
  });

  const move = useMutation({
    mutationFn: (position: string) => moveProjectColumn(column.id, position),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project-columns", project.id] });
    },
  });

  return (
    <Box
      header={
        isEditing ? (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              update.mutate(name);
            }}
            style={{ display: "flex", gap: "0.5rem" }}
          >
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              style={{ fontSize: "0.85rem", padding: "0.2rem 0.4rem", flex: 1 }}
            />
            <Button type="submit" variant="primary" size="sm" disabled={update.isPending}>
              Save
            </Button>
          </form>
        ) : (
          <div className="flex items-center justify-between">
            <span>{column.name}</span>
            <div className="flex gap-1">
              {columns.findIndex((c) => c.id === column.id) > 0 && (
                <button
                  type="button"
                  onClick={() => {
                    const prev = columns[columns.findIndex((c) => c.id === column.id) - 1];
                    move.mutate("after:" + prev.id);
                  }}
                  style={{ fontSize: "0.7rem", background: "transparent", border: "none", cursor: "pointer" }}
                >
                  ←
                </button>
              )}
              {columns.findIndex((c) => c.id === column.id) < columns.length - 1 && (
                <button
                  type="button"
                  onClick={() => {
                    const idx = columns.findIndex((c) => c.id === column.id);
                    const next = columns[idx + 1];
                    move.mutate("after:" + next.id);
                  }}
                  style={{ fontSize: "0.7rem", background: "transparent", border: "none", cursor: "pointer" }}
                >
                  →
                </button>
              )}
              <button
                type="button"
                onClick={() => setIsEditing(true)}
                style={{ fontSize: "0.7rem", background: "transparent", border: "none", cursor: "pointer" }}
              >
                ✎
              </button>
              <button
                type="button"
                onClick={() => {
                  if (window.confirm(`Delete column "${column.name}"?`)) {
                    remove.mutate();
                  }
                }}
                style={{ fontSize: "0.7rem", color: "var(--color-danger-fg)", background: "transparent", border: "none", cursor: "pointer" }}
              >
                ×
              </button>
            </div>
          </div>
        )
      }
      style={{ minWidth: 260, maxWidth: 260 }}
    >
      <div style={{ padding: "0.75rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
        {isLoading ? (
          <Spinner label="loading cards" />
        ) : (
          cards.map((card) => (
            <ProjectCardItem
              key={card.id}
              owner={owner}
              repo={repo}
              project={project}
              column={column}
              card={card}
              columns={columns}
            />
          ))
        )}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (newNote.trim()) createCard.mutate(newNote.trim());
          }}
          style={{ display: "flex", flexDirection: "column", gap: "0.5rem" }}
        >
          <textarea
            placeholder="New note card"
            value={newNote}
            onChange={(e) => setNewNote(e.target.value)}
            rows={2}
            style={{ fontSize: "0.85rem", padding: "0.35rem 0.5rem" }}
          />
          <Button type="submit" variant="secondary" size="sm" disabled={createCard.isPending}>
            {createCard.isPending ? "Adding..." : "Add card"}
          </Button>
        </form>
      </div>
    </Box>
  );
}

function ProjectCardItem({
  owner,
  repo,
  project,
  column,
  card,
  columns,
}: {
  owner: string;
  repo: string;
  project: GithubProjectClassic;
  column: GithubProjectColumn;
  card: GithubProjectCard;
  columns: GithubProjectColumn[];
}) {
  const queryClient = useQueryClient();
  const [isEditing, setIsEditing] = useState(false);
  const [note, setNote] = useState(card.note ?? "");

  const update = useMutation({
    mutationFn: (n: string) => updateProjectCard(card.id, n),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project-cards", column.id] });
      setIsEditing(false);
    },
  });

  const remove = useMutation({
    mutationFn: () => deleteProjectCard(card.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project-cards", column.id] });
    },
  });

  const move = useMutation({
    mutationFn: (payload: { position: string; column_id?: number }) => moveProjectCard(card.id, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project-cards"] });
    },
  });

  return (
    <div
      style={{
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-md)",
        padding: "0.6rem",
        background: "var(--color-bg-subtle)",
      }}
    >
      {isEditing ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            update.mutate(note);
          }}
          style={{ display: "flex", flexDirection: "column", gap: "0.5rem" }}
        >
          <textarea
            value={note}
            onChange={(e) => setNote(e.target.value)}
            rows={2}
            style={{ fontSize: "0.85rem", padding: "0.35rem 0.5rem" }}
          />
          <div className="flex gap-2">
            <Button type="submit" variant="primary" size="sm" disabled={update.isPending}>
              Save
            </Button>
            <Button type="button" variant="secondary" size="sm" onClick={() => setIsEditing(false)}>
              Cancel
            </Button>
          </div>
        </form>
      ) : (
        <div>
          <div style={{ fontSize: "0.85rem", whiteSpace: "pre-wrap" }}>
            {card.note || (
              <a
                href={card.content_url?.replace("/api/v3/repos/", "/").replace("/issues/", "/issues/") ?? "#"}
                style={{ color: "var(--color-accent)" }}
              >
                linked issue
              </a>
            )}
          </div>
          <div className="mt-2 flex flex-wrap gap-1">
            <select
              value=""
              onChange={(e) => {
                const [action, target] = e.target.value.split(":");
                if (action === "col") {
                  move.mutate({ position: "first", column_id: Number(target) });
                } else if (action === "pos") {
                  move.mutate({ position: target });
                }
                e.target.value = "";
              }}
              style={{ fontSize: "0.75rem", padding: "0.2rem 0.3rem" }}
            >
              <option value="">Move...</option>
              <optgroup label="Column">
                {columns
                  .filter((c) => c.id !== column.id)
                  .map((c) => (
                    <option key={c.id} value={`col:${c.id}`}>
                      {c.name}
                    </option>
                  ))}
              </optgroup>
              <optgroup label="Position">
                <option value="pos:first">First</option>
                <option value="pos:last">Last</option>
              </optgroup>
            </select>
            <button
              type="button"
              onClick={() => setIsEditing(true)}
              style={{ fontSize: "0.75rem", background: "transparent", border: "none", cursor: "pointer" }}
            >
              Edit
            </button>
            <button
              type="button"
              onClick={() => {
                if (window.confirm("Delete this card?")) {
                  remove.mutate();
                }
              }}
              style={{ fontSize: "0.75rem", color: "var(--color-danger-fg)", background: "transparent", border: "none", cursor: "pointer" }}
            >
              Delete
            </button>
          </div>
          {move.isError && (
            <div style={{ fontSize: "0.75rem", color: "var(--color-danger-fg)" }}>
              {move.error instanceof Error ? move.error.message : String(move.error)}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
