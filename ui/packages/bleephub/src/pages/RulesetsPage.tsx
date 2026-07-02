import { useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import {
  fetchOrgRulesets,
  createOrgRuleset,
  updateOrgRuleset,
  deleteOrgRuleset,
} from "../api.js";
import type { GithubRuleset, GithubRulesetTarget, GithubRulesetEnforcement } from "../types.js";
import { OrgHeader } from "../components/Shell.js";
import {
  Box,
  Button,
  Modal,
  FormLabel,
  ErrorBanner,
  DialogActions,
  PageTitle,
} from "../components/ui.js";

const col = createColumnHelper<GithubRuleset>();

const TARGETS: GithubRulesetTarget[] = ["branch", "tag"];
const ENFORCEMENTS: GithubRulesetEnforcement[] = ["disabled", "active", "evaluate"];

const RULE_TYPES = [
  "branch_name_pattern",
  "tag_name_pattern",
  "commit_author_email_pattern",
  "commit_message_pattern",
  "committer_email_pattern",
  "required_linear_history",
  "required_signatures",
  "required_deployments",
  "required_status_checks",
  "pull_request",
  "non_fast_forward",
  "creation",
  "update",
  "deletion",
];

export function RulesetsPage() {
  const { org } = useParams<{ org: string }>();
  if (!org) {
    return <InlineError title="Missing organization" detail="No organization login provided." />;
  }

  return (
    <div>
      <OrgHeader org={org} active="rulesets" />
      <RulesetsContent org={org} />
    </div>
  );
}

function RulesetsContent({ org }: { org: string }) {
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<GithubRuleset | null>(null);
  const [editing, setEditing] = useState<GithubRuleset | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [mutationError, setMutationError] = useState<string | null>(null);

  const {
    data: rulesets = [],
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["org-rulesets", org],
    queryFn: () => fetchOrgRulesets(org),
    enabled: !!org,
  });

  const createMutation = useMutation({
    mutationFn: (payload: Parameters<typeof createOrgRuleset>[1]) => createOrgRuleset(org, payload),
    onSuccess: () => {
      setMutationError(null);
      queryClient.invalidateQueries({ queryKey: ["org-rulesets", org] });
      setShowCreate(false);
    },
    onError: (err: Error) => setMutationError(err.message),
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, payload }: { id: number; payload: Parameters<typeof updateOrgRuleset>[2] }) => updateOrgRuleset(org, id, payload),
    onSuccess: () => {
      setMutationError(null);
      queryClient.invalidateQueries({ queryKey: ["org-rulesets", org] });
      setEditing(null);
    },
    onError: (err: Error) => setMutationError(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteOrgRuleset(org, id),
    onSuccess: () => {
      setMutationError(null);
      queryClient.invalidateQueries({ queryKey: ["org-rulesets", org] });
      setSelected(null);
    },
    onError: (err: Error) => setMutationError(err.message),
  });

  if (isLoading) return <Spinner label={`loading ${org} rulesets`} />;
  if (isError) return <InlineError title="Failed to load rulesets" detail={String(error)} />;

  const columns = [
    col.accessor("id", {
      header: "ID",
      cell: (info) => (
        <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
          {info.getValue()}
        </span>
      ),
    }),
    col.accessor("name", {
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
    col.accessor("target", { header: "Target" }),
    col.accessor("enforcement", { header: "Enforcement" }),
    col.accessor("source", { header: "Source" }),
    col.display({
      id: "rules",
      header: "Rules",
      cell: (info) => info.row.original.rules?.length ?? 0,
    }),
    col.display({
      id: "actions",
      header: "Actions",
      cell: (info) => {
        const ruleset = info.row.original;
        return (
          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm" variant="ghost" onClick={() => setEditing(ruleset)}>
              Edit
            </Button>
            <Button
              size="sm"
              variant="danger"
              onClick={() => {
                if (confirm(`Delete ruleset "${ruleset.name}"?`)) {
                  deleteMutation.mutate(ruleset.id);
                }
              }}
              disabled={deleteMutation.isPending}
            >
              Delete
            </Button>
          </div>
        );
      },
    }),
  ];

  return (
    <div className="mt-4">
      <PageTitle
        title="Rulesets"
        meta={
          <Link to={`/ui/orgs/${org}/repos`} style={{ color: "var(--color-accent)", textDecoration: "none" }}>
            ← Back to repositories
          </Link>
        }
        actions={
          <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
            New ruleset
          </Button>
        }
      />

      {mutationError && <ErrorBanner>{mutationError}</ErrorBanner>}

      <Box>
        <DataTable
          data={rulesets}
          columns={columns}
          emptyMessage="No rulesets configured for this organization."
        />
      </Box>

      {selected && <RulesetDetailDialog ruleset={selected} onClose={() => setSelected(null)} />}

      {(showCreate || editing) && (
        <RulesetFormModal
          org={org}
          ruleset={editing}
          onClose={() => {
            setShowCreate(false);
            setEditing(null);
          }}
          onSubmit={(payload) => {
            if (editing) {
              updateMutation.mutate({ id: editing.id, payload });
            } else {
              createMutation.mutate(payload);
            }
          }}
          pending={createMutation.isPending || updateMutation.isPending}
          error={createMutation.error ?? updateMutation.error}
        />
      )}
    </div>
  );
}

function RulesetDetailDialog({ ruleset, onClose }: { ruleset: GithubRuleset; onClose: () => void }) {
  return (
    <Modal title={ruleset.name} onClose={onClose}>
      <div style={{ fontSize: "0.85rem", display: "flex", flexDirection: "column", gap: "0.5rem" }}>
        <div>
          <strong>ID:</strong> {ruleset.id}
        </div>
        <div>
          <strong>Target:</strong> {ruleset.target}
        </div>
        <div>
          <strong>Enforcement:</strong> {ruleset.enforcement}
        </div>
        <div>
          <strong>Source:</strong> {ruleset.source} ({ruleset.source_type})
        </div>
        {ruleset.created_at && (
          <div>
            <strong>Created:</strong> {new Date(ruleset.created_at).toLocaleString()}
          </div>
        )}
        {ruleset.updated_at && (
          <div>
            <strong>Updated:</strong> {new Date(ruleset.updated_at).toLocaleString()}
          </div>
        )}
        <div style={{ marginTop: "0.5rem" }}>
          <strong>Rules ({ruleset.rules?.length ?? 0})</strong>
          {ruleset.rules && ruleset.rules.length > 0 ? (
            <ul style={{ listStyle: "none", padding: 0, margin: "0.5rem 0 0" }}>
              {ruleset.rules.map((rule, idx) => (
                <li key={idx} style={{ padding: "0.25rem 0", borderBottom: "1px solid var(--color-border)" }}>
                  {rule.type}
                </li>
              ))}
            </ul>
          ) : (
            <p style={{ color: "var(--color-fg-muted)" }}>No rules.</p>
          )}
        </div>
      </div>
      <DialogActions>
        <Button onClick={onClose} variant="ghost">Close</Button>
      </DialogActions>
    </Modal>
  );
}

function RulesetFormModal({
  org,
  ruleset,
  onClose,
  onSubmit,
  pending,
  error,
}: {
  org: string;
  ruleset: GithubRuleset | null;
  onClose: () => void;
  onSubmit: (payload: Parameters<typeof createOrgRuleset>[1]) => void;
  pending: boolean;
  error: Error | null;
}) {
  const [name, setName] = useState(ruleset?.name ?? "");
  const [target, setTarget] = useState<GithubRulesetTarget>(ruleset?.target ?? "branch");
  const [enforcement, setEnforcement] = useState<GithubRulesetEnforcement>(
    ruleset?.enforcement ?? "active",
  );
  const [selectedRules, setSelectedRules] = useState<Set<string>>(
    () => new Set(ruleset?.rules?.map((r) => r.type) ?? []),
  );
  const [validationError, setValidationError] = useState<string | null>(null);

  const toggleRule = (type: string) => {
    setSelectedRules((prev) => {
      const next = new Set(prev);
      if (next.has(type)) {
        next.delete(type);
      } else {
        next.add(type);
      }
      return next;
    });
  };

  const handleSubmit = () => {
    setValidationError(null);
    if (!name.trim()) {
      setValidationError("Name is required.");
      return;
    }
    const payload: Parameters<typeof createOrgRuleset>[1] = {
      name: name.trim(),
      target,
      enforcement,
      rules: Array.from(selectedRules).map((type) => ({ type })),
    };
    onSubmit(payload);
  };

  return (
    <Modal title={ruleset ? "Edit ruleset" : "Create ruleset"} onClose={onClose}>
      <FormLabel id="ruleset-name">Name</FormLabel>
      <input
        id="ruleset-name"
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="e.g. main-branch-protection"
        className="mb-4 w-full"
      />

      <FormLabel id="ruleset-target">Target</FormLabel>
      <select
        id="ruleset-target"
        value={target}
        onChange={(e) => setTarget(e.target.value as GithubRulesetTarget)}
        className="mb-4 w-full"
      >
        {TARGETS.map((t) => (
          <option key={t} value={t}>
            {t}
          </option>
        ))}
      </select>

      <FormLabel id="ruleset-enforcement">Enforcement</FormLabel>
      <select
        id="ruleset-enforcement"
        value={enforcement}
        onChange={(e) => setEnforcement(e.target.value as GithubRulesetEnforcement)}
        className="mb-4 w-full"
      >
        {ENFORCEMENTS.map((e) => (
          <option key={e} value={e}>
            {e}
          </option>
        ))}
      </select>

      <FormLabel>Rules</FormLabel>
      <div className="mb-4" style={{ display: "flex", flexDirection: "column", gap: "0.4rem" }}>
        {RULE_TYPES.map((type) => (
          <label key={type} style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
            <input
              type="checkbox"
              checked={selectedRules.has(type)}
              onChange={() => toggleRule(type)}
            />
            {type}
          </label>
        ))}
      </div>

      {(validationError || error) && <ErrorBanner>{validationError ?? (error instanceof Error ? error.message : String(error))}</ErrorBanner>}

      <DialogActions>
        <Button onClick={onClose} disabled={pending} variant="ghost">
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={pending} variant="primary">
          {pending ? "Saving…" : ruleset ? "Save ruleset" : "Create ruleset"}
        </Button>
      </DialogActions>
    </Modal>
  );
}
