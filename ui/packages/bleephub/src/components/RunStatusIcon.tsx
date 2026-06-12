import type { GHConclusion, GHRunStatus } from "../types.js";
import {
  CheckCircleIcon,
  ClockIcon,
  DotFillIcon,
  ProgressArcIcon,
  SkipCircleIcon,
  StopCircleIcon,
  XCircleIcon,
} from "./octicons.js";

/**
 * GitHub Actions status glyph for runs / jobs / steps / check runs:
 * green check (success), red X (failure), spinning yellow arc
 * (in_progress), yellow dot (queued), clock (waiting on approval),
 * gray stop/slash (cancelled / skipped).
 */
export function RunStatusIcon({
  status,
  conclusion,
  size = 16,
}: {
  status: GHRunStatus;
  conclusion: GHConclusion | null;
  size?: number;
}) {
  if (status === "completed") {
    switch (conclusion) {
      case "success":
        return <CheckCircleIcon size={size} title="success" style={{ color: "var(--gh-open)" }} />;
      case "failure":
      case "timed_out":
      case "action_required":
        return <XCircleIcon size={size} title={conclusion} style={{ color: "var(--color-status-error)" }} />;
      case "cancelled":
        return <StopCircleIcon size={size} title="cancelled" style={{ color: "var(--color-fg-subtle)" }} />;
      case "skipped":
        return <SkipCircleIcon size={size} title="skipped" style={{ color: "var(--color-fg-subtle)" }} />;
      case "neutral":
      default:
        return <DotFillIcon size={size} title={conclusion ?? "completed"} style={{ color: "var(--color-fg-subtle)" }} />;
    }
  }
  if (status === "in_progress") {
    return (
      <ProgressArcIcon
        size={size}
        title="in progress"
        className="animate-spin"
        style={{ color: "var(--color-status-warn)" }}
      />
    );
  }
  if (status === "waiting") {
    return <ClockIcon size={size} title="waiting" style={{ color: "var(--color-status-warn)" }} />;
  }
  return <DotFillIcon size={size} title="queued" style={{ color: "var(--color-status-warn)" }} />;
}
