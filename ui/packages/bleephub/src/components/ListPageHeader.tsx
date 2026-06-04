import type { ReactNode } from "react";
import { Link } from "react-router";
import { PageHeading } from "@sockerless/ui-core/components";
import { StateToggle } from "./StateToggle.js";

/** Shared header for open/closed list pages (Issues, PRs). */
export function ListPageHeader({
  owner,
  repo,
  backTo,
  title,
  meta,
  actions,
  state,
  stateLabels,
  onStateChange,
}: {
  owner: string;
  repo: string;
  backTo: string;
  title: ReactNode;
  meta?: string;
  actions?: ReactNode;
  state: "open" | "closed";
  stateLabels: { open: string; closed: string };
  onStateChange: (s: "open" | "closed") => void;
}) {
  return (
    <>
      <div style={{ marginBottom: "0.25rem" }}>
        <Link
          to={backTo}
          style={{ color: "var(--color-fg-muted)", fontSize: "0.8rem", fontFamily: "var(--font-mono)" }}
        >
          ← {owner}/{repo}
        </Link>
      </div>
      <PageHeading kicker={`${owner}/${repo}`} title={title} meta={meta} actions={actions} />
      <StateToggle
        value={state}
        options={["open", "closed"] as const}
        labels={stateLabels}
        onChange={onStateChange}
      />
    </>
  );
}
