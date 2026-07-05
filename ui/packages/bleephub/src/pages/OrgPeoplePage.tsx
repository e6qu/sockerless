import { useMemo, useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { InlineError, Spinner } from "@sockerless/ui-core/components";
import { fetchOrgMembers } from "../api.js";
import type { GithubAccount } from "../types.js";
import { OrgHeader } from "../components/Shell.js";
import { Avatar } from "../components/Avatar.js";
import { Box, SectionLabel, Blankslate } from "../components/ui.js";
import { PeopleIcon } from "../components/octicons.js";

export function OrgPeoplePage() {
  const { org = "" } = useParams<{ org: string }>();
  const [filter, setFilter] = useState("");

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["org-members", org],
    queryFn: () => fetchOrgMembers(org),
  });

  const filtered = useMemo(() => {
    if (!data) return [];
    const q = filter.trim().toLowerCase();
    if (!q) return data;
    return data.filter((m) => m.login.toLowerCase().includes(q));
  }, [data, filter]);

  return (
    <div>
      <OrgHeader org={org} active="people" />
      <SectionLabel>
        People{data ? ` · ${data.length}` : ""}
      </SectionLabel>

      {isLoading && <Spinner label="loading members" />}
      {isError && <InlineError title="Failed to load members" detail={String(error)} />}
      {data && (
        <>
          <input
            type="search"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Find a member…"
            aria-label="Find a member"
            className="mb-4 w-full"
            style={{ maxWidth: "20rem", fontSize: "0.85rem" }}
          />
          {data.length === 0 ? (
            <Blankslate icon={<PeopleIcon size={28} />} title="No members">
              This organization has no visible members.
            </Blankslate>
          ) : filtered.length === 0 ? (
            <Blankslate icon={<PeopleIcon size={28} />} title="No matches">
              No member matches “{filter}”.
            </Blankslate>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {filtered.map((m) => (
                <MemberCard key={m.id} member={m} />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function MemberCard({ member }: { member: GithubAccount }) {
  return (
    <Box style={{ padding: "0.85rem 1rem" }}>
      <div className="flex items-center gap-3">
        <Avatar login={member.login} src={member.avatar_url} size={44} />
        <div className="min-w-0">
          <Link
            to={`/ui/${member.login}`}
            style={{ color: "var(--color-fg)", fontWeight: 600, fontSize: "0.92rem", textDecoration: "none" }}
          >
            {member.login}
          </Link>
          <div style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
            {member.site_admin ? "Site admin" : member.type}
          </div>
        </div>
      </div>
    </Box>
  );
}
