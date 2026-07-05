import { useMemo, useState } from "react";
import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { InlineError, Spinner } from "@sockerless/ui-core/components";
import { fetchOrgTeams } from "../api.js";
import type { GithubOrgTeam } from "../types.js";
import { OrgHeader } from "../components/Shell.js";
import { Box, SectionLabel, Blankslate } from "../components/ui.js";
import { TeamIcon, LockIcon } from "../components/octicons.js";

export function OrgTeamsPage() {
  const { org = "" } = useParams<{ org: string }>();
  const [filter, setFilter] = useState("");

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["org-teams", org],
    queryFn: () => fetchOrgTeams(org),
  });

  const filtered = useMemo(() => {
    if (!data) return [];
    const q = filter.trim().toLowerCase();
    if (!q) return data;
    return data.filter(
      (t) => t.name.toLowerCase().includes(q) || t.slug.toLowerCase().includes(q),
    );
  }, [data, filter]);

  return (
    <div>
      <OrgHeader org={org} active="teams" />
      <SectionLabel>Teams{data ? ` · ${data.length}` : ""}</SectionLabel>

      {isLoading && <Spinner label="loading teams" />}
      {isError && <InlineError title="Failed to load teams" detail={String(error)} />}
      {data && (
        <>
          <input
            type="search"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Find a team…"
            aria-label="Find a team"
            className="mb-4 w-full"
            style={{ maxWidth: "20rem", fontSize: "0.85rem" }}
          />
          {data.length === 0 ? (
            <Blankslate icon={<TeamIcon size={28} />} title="No teams">
              This organization has no teams yet.
            </Blankslate>
          ) : filtered.length === 0 ? (
            <Blankslate icon={<TeamIcon size={28} />} title="No matches">
              No team matches “{filter}”.
            </Blankslate>
          ) : (
            <Box>
              {filtered.map((t, i) => (
                <TeamRow key={t.id} team={t} last={i === filtered.length - 1} />
              ))}
            </Box>
          )}
        </>
      )}
    </div>
  );
}

function TeamRow({ team, last }: { team: GithubOrgTeam; last: boolean }) {
  return (
    <div
      className="flex flex-wrap items-center gap-3"
      style={{
        padding: "0.75rem 1rem",
        borderBottom: last ? "none" : "1px solid var(--color-border)",
      }}
    >
      <TeamIcon size={16} style={{ color: "var(--color-fg-muted)" }} />
      <div className="min-w-0 flex-1">
        <span style={{ fontWeight: 600, fontSize: "0.92rem" }}>{team.name}</span>
        <span className="ml-2" style={{ fontSize: "0.8rem", color: "var(--color-fg-muted)" }}>
          @{team.slug}
        </span>
        {team.description && (
          <p className="mt-0.5" style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
            {team.description}
          </p>
        )}
      </div>
      <span
        className="inline-flex items-center gap-1"
        style={{ fontSize: "0.75rem", color: "var(--color-fg-muted)" }}
      >
        {team.privacy === "secret" && <LockIcon size={12} />}
        {team.privacy}
      </span>
    </div>
  );
}
