import type { CSSProperties } from "react";

/*
 * Original, deliberately-simple glyphs in the spirit of GitHub's UI
 * (repo / issue / pull-request vocabulary) drawn fresh here — not copied
 * octicon path data. They inherit `currentColor` so callers colour them
 * with the surrounding text colour or a state token.
 */

export interface IconProps {
  size?: number;
  className?: string;
  style?: CSSProperties;
  title?: string;
}

function Svg({
  size = 16,
  className,
  style,
  title,
  children,
}: IconProps & { children: React.ReactNode }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden={title ? undefined : true}
      role={title ? "img" : undefined}
      style={{ flexShrink: 0, display: "inline-block", verticalAlign: "text-bottom", ...style }}
      className={className}
    >
      {title ? <title>{title}</title> : null}
      {children}
    </svg>
  );
}

/** Brand mark — a polished chat bubble (a "bleep") using the active theme. */
export function Mark({ size = 22, style, className }: IconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      aria-hidden
      style={{ flexShrink: 0, ...style }}
      className={className}
    >
      <defs>
        <linearGradient id="bleephub-mark" x1="4" y1="3" x2="20" y2="20" gradientUnits="userSpaceOnUse">
          <stop stopColor="var(--color-brand-cyan)" />
          <stop offset="0.48" stopColor="var(--color-brand-blue)" />
          <stop offset="1" stopColor="var(--color-brand-pink)" />
        </linearGradient>
      </defs>
      <rect x="2" y="3" width="20" height="15" rx="4" fill="url(#bleephub-mark)" />
      <path d="M8 18l-2 3v-3" fill="var(--color-brand-purple)" />
      <path d="M6 5.5h12" stroke="rgba(255,255,255,0.28)" strokeWidth="1" strokeLinecap="round" />
      <circle cx="8" cy="10.5" r="1.4" fill="var(--color-accent-fg)" />
      <circle cx="12" cy="10.5" r="1.4" fill="var(--color-accent-fg)" />
      <circle cx="16" cy="10.5" r="1.4" fill="var(--color-accent-fg)" />
    </svg>
  );
}

export function RepoIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M3 2.5h7.5a2 2 0 0 1 2 2V13a1 1 0 0 0-1-1H4a1 1 0 0 0-1 1z" />
      <path d="M3 13a1 1 0 0 0 1 1h8.5" />
    </Svg>
  );
}

export function FileIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M4 2.5h5l3 3v8a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-10a1 1 0 0 1 1-1z" />
      <path d="M9 2.5v3h3" />
    </Svg>
  );
}

export function DirectoryIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M2 4.5a1 1 0 0 1 1-1h3l2 2h5a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1z" />
    </Svg>
  );
}

export function IssueOpenedIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="8" r="5.5" />
      <circle cx="8" cy="8" r="1.4" fill="currentColor" stroke="none" />
    </Svg>
  );
}

export function IssueClosedIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="8" r="5.5" />
      <path d="M5.8 8.2l1.6 1.6 3-3.4" />
    </Svg>
  );
}

export function PullRequestIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="4.5" cy="4" r="1.6" />
      <circle cx="4.5" cy="12" r="1.6" />
      <circle cx="11.5" cy="12" r="1.6" />
      <path d="M4.5 5.6v4.8" />
      <path d="M11.5 10.4V7.5a2 2 0 0 0-2-2H7" />
      <path d="M8.4 4.1L7 5.5l1.4 1.4" />
    </Svg>
  );
}

export function MergedIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="4.5" cy="4" r="1.6" />
      <circle cx="4.5" cy="12" r="1.6" />
      <circle cx="11.5" cy="8" r="1.6" />
      <path d="M4.5 5.6v4.8" />
      <path d="M4.5 6.4c0 2.4 1.8 3.6 5.4 3.6" />
    </Svg>
  );
}

export function PullClosedIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="4.5" cy="4" r="1.6" />
      <circle cx="4.5" cy="12" r="1.6" />
      <path d="M4.5 5.6v4.8" />
      <path d="M10 4l3 3M13 4l-3 3" />
    </Svg>
  );
}

export function CommentIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M2.5 3.5h11v7h-6l-3 2.5v-2.5h-2z" />
    </Svg>
  );
}

export function TagIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M2.5 2.5h5l6 6-5 5-6-6z" />
      <circle cx="5.4" cy="5.4" r="1" fill="currentColor" stroke="none" />
    </Svg>
  );
}

export function BranchIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="4.5" cy="3.5" r="1.5" />
      <circle cx="4.5" cy="12.5" r="1.5" />
      <circle cx="11.5" cy="5.5" r="1.5" />
      <path d="M4.5 5v6" />
      <path d="M11.5 7c0 2.5-2.5 3-4 3.5" />
    </Svg>
  );
}

export function LockIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <rect x="3.5" y="7" width="9" height="6.5" rx="1.2" />
      <path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2" />
    </Svg>
  );
}

export function SunIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="8" r="3" />
      <path d="M8 1.5v1.5M8 13v1.5M1.5 8h1.5M13 8h1.5M3.4 3.4l1 1M11.6 11.6l1 1M12.6 3.4l-1 1M4.4 11.6l-1 1" />
    </Svg>
  );
}

export function MoonIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M13 9.5A5.5 5.5 0 0 1 6.5 3a5.5 5.5 0 1 0 6.5 6.5z" />
    </Svg>
  );
}

export function SignOutIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M6 2.5H3.5a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1H6" />
      <path d="M9.5 11l3-3-3-3M12.5 8H6" />
    </Svg>
  );
}

/* Actions status vocabulary — run / job / step / check states. */

export function CheckCircleIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="8" r="5.5" />
      <path d="M5.6 8.3l1.7 1.7 3.1-3.6" />
    </Svg>
  );
}

export function XCircleIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="8" r="5.5" />
      <path d="M6 6l4 4M10 6l-4 4" />
    </Svg>
  );
}

/** Cancelled: circle with a horizontal stop bar. */
export function StopCircleIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="8" r="5.5" />
      <path d="M5.5 8h5" />
    </Svg>
  );
}

/** Skipped: circle with a diagonal slash. */
export function SkipCircleIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="8" r="5.5" />
      <path d="M5 11l6-6" />
    </Svg>
  );
}

/** Queued: solid dot. */
export function DotFillIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="8" r="4.5" fill="currentColor" stroke="none" />
    </Svg>
  );
}

/** In-progress: 3/4 arc — callers spin it with the animate-spin class. */
export function ProgressArcIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M8 2.5a5.5 5.5 0 1 1-5.5 5.5" />
    </Svg>
  );
}

/** Waiting on approval / wait timer: clock face. */
export function ClockIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="8" r="5.5" />
      <path d="M8 5v3.2l2.2 1.3" />
    </Svg>
  );
}

export function PlayIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="8" r="5.5" />
      <path d="M6.7 5.8v4.4L10.3 8z" fill="currentColor" stroke="none" />
    </Svg>
  );
}

export function GearIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="8" r="2" />
      <path d="M8 1.8v2M8 12.2v2M1.8 8h2M12.2 8h2M3.6 3.6l1.4 1.4M11 11l1.4 1.4M12.4 3.6L11 5M5 11l-1.4 1.4" />
    </Svg>
  );
}

export function KebabIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="3.5" r="1.1" fill="currentColor" stroke="none" />
      <circle cx="8" cy="8" r="1.1" fill="currentColor" stroke="none" />
      <circle cx="8" cy="12.5" r="1.1" fill="currentColor" stroke="none" />
    </Svg>
  );
}

export function ChevronRightIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M6 3.5L10.5 8 6 12.5" />
    </Svg>
  );
}

export function ChevronDownIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M3.5 6L8 10.5 12.5 6" />
    </Svg>
  );
}

export function DownloadIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M8 2.5v7M5 6.8L8 9.8l3-3" />
      <path d="M2.5 11v1.5a1 1 0 0 0 1 1h9a1 1 0 0 0 1-1V11" />
    </Svg>
  );
}

export function KeyIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="10.5" cy="5.5" r="2.8" />
      <path d="M8.4 7.6L3 13M5 11l1.5 1.5M3.5 12.5L5 14" />
    </Svg>
  );
}

export function PeopleIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="6" cy="5" r="2.2" />
      <circle cx="12" cy="5" r="2.2" />
      <path d="M2 14c1-2.5 2.5-3.5 4-3.5s3 1 4 3.5" />
      <path d="M8 14c1-2.5 2.5-3.5 4-3.5s3 1 4 3.5" />
    </Svg>
  );
}

export function OrganizationIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <rect x="2.5" y="2.5" width="11" height="11" rx="1" />
      <path d="M6 2.5v11M10 2.5v11M2.5 6h11M2.5 10h11" />
    </Svg>
  );
}

export function TeamIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="5" r="2.2" />
      <path d="M3 13.5c1-2.8 2.8-4 5-4s4 1.2 5 4" />
      <circle cx="14" cy="6" r="1.4" />
      <path d="M12.5 13.5c0.5-1.8 1.5-2.6 2.5-2.6" />
    </Svg>
  );
}

export function AuditLogIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M3.5 2.5h9a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1h-9a1 1 0 0 1-1-1v-9a1 1 0 0 1 1-1z" />
      <path d="M5.5 6h5M5.5 9h3" />
      <path d="M2.5 5l-1-1M2.5 8l-1-1M2.5 11l-1-1" />
    </Svg>
  );
}

export function ServerIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <rect x="2.5" y="2" width="11" height="5" rx="1" />
      <rect x="2.5" y="9" width="11" height="5" rx="1" />
      <circle cx="5" cy="4.5" r="0.8" fill="currentColor" stroke="none" />
      <circle cx="5" cy="11.5" r="0.8" fill="currentColor" stroke="none" />
    </Svg>
  );
}

export function GistIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M4 2.5h5l3 3v8a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-10a1 1 0 0 1 1-1z" />
      <path d="M9 2.5v3h3" />
      <path d="M5.5 8h5M5.5 10.5h5" />
    </Svg>
  );
}

export function StarIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M8 1.5l1.8 4.6h4.9l-3.9 2.9 1.5 4.6L8 10.6 4.7 13.6l1.5-4.6L2.3 6.1h4.9z" />
    </Svg>
  );
}

export function NotificationBellIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M8 1.5c-2.5 0-4 2-4 4.5v3l-1.5 2h11L12 9V6c0-2.5-1.5-4.5-4-4.5z" />
      <path d="M6 13.5a2 2 0 0 0 4 0" />
    </Svg>
  );
}

export function ProjectIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <rect x="2.5" y="2.5" width="11" height="11" rx="1" />
      <path d="M6 2.5v11M10 2.5v11" />
    </Svg>
  );
}

export function MigrationIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <rect x="2.5" y="2.5" width="11" height="11" rx="1" />
      <path d="M5 8h6M8 5v6" />
      <path d="M12.5 3.5l-2-2M12.5 3.5v-2h-2" />
    </Svg>
  );
}

export function TrashIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M3 4.5h10M5.5 4.5V3a1 1 0 0 1 1-1h3a1 1 0 0 1 1 1v1.5" />
      <path d="M6 6.5v7M10 6.5v7M4.5 13.5h7" />
    </Svg>
  );
}

export function SquareIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <rect x="3.5" y="3.5" width="9" height="9" rx="1" />
    </Svg>
  );
}

export function PackageIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M8.5 1.5 14 4.75v6.5L8.5 14.5l-5.5-3.25v-6.5L8.5 1.5z" />
      <path d="M3 4.75 8.5 8 14 4.75" />
      <path d="M8.5 8v6.5" />
    </Svg>
  );
}

export function DiscussionIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M2.5 3.5h11v7h-7l-3.5 3v-3h-0.5z" />
      <path d="M5.5 6.5h5M5.5 9h3" />
    </Svg>
  );
}

export function CodespaceIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <rect x="2" y="3" width="12" height="10" rx="1.5" />
      <path d="M5 7h6M5 10h4" />
      <circle cx="11" cy="10" r="1" fill="currentColor" stroke="none" />
    </Svg>
  );
}

export function GraphIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M2.5 2.5v11h11" />
      <path d="M5.5 10.5v-3M8.5 10.5v-6M11.5 10.5v-4.5" />
    </Svg>
  );
}

export function GlobeIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="8" r="5.5" />
      <path d="M2.5 8h11M8 2.5c-1.8 1.5-2.7 3.4-2.7 5.5s0.9 4 2.7 5.5c1.8-1.5 2.7-3.4 2.7-5.5s-0.9-4-2.7-5.5z" />
    </Svg>
  );
}

/** Webhook: an emitting node fanned out to two receivers. */
export function WebhookIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="8" cy="4" r="2" />
      <circle cx="3.5" cy="12" r="2" />
      <circle cx="12.5" cy="12" r="2" />
      <path d="M7 5.8 4.6 10.3M9 5.8l2.4 4.5" />
    </Svg>
  );
}

/** Magnifying glass for the global search page. */
export function SearchIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="7" cy="7" r="4.5" />
      <path d="M10.5 10.5L14 14" />
    </Svg>
  );
}

/** Plus — the "create new" header menu. */
export function PlusIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M8 3v10M3 8h10" />
    </Svg>
  );
}

/** Three-bar "hamburger" — opens the global navigation drawer. */
export function ThreeBarsIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M2.5 4h11M2.5 8h11M2.5 12h11" />
    </Svg>
  );
}

/** Downward caret for menu triggers. */
export function TriangleDownIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M4 6l4 4 4-4z" fill="currentColor" stroke="none" />
    </Svg>
  );
}

/** Angle brackets — the "Code" clone button glyph. */
export function CodeIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M5.5 4.5 2 8l3.5 3.5M10.5 4.5 14 8l-3.5 3.5" />
    </Svg>
  );
}

/** Two overlapping sheets — copy to clipboard. */
export function CopyIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <rect x="5.5" y="5.5" width="8" height="8" rx="1.5" />
      <path d="M10.5 5.5V4a1.5 1.5 0 0 0-1.5-1.5H4A1.5 1.5 0 0 0 2.5 4v5A1.5 1.5 0 0 0 4 10.5h1.5" />
    </Svg>
  );
}

/** Plain checkmark — confirms a copy succeeded. */
export function CheckIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M3 8.5 6.5 12 13 4.5" />
    </Svg>
  );
}

/** Eye — the repository "Watch" counter. */
export function EyeIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M1.5 8S4 3.5 8 3.5 14.5 8 14.5 8 12 12.5 8 12.5 1.5 8 1.5 8z" />
      <circle cx="8" cy="8" r="1.8" />
    </Svg>
  );
}

/** Forked repository — the "Fork" counter. */
export function RepoForkedIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <circle cx="4" cy="3.5" r="1.6" />
      <circle cx="12" cy="3.5" r="1.6" />
      <circle cx="8" cy="12.5" r="1.6" />
      <path d="M4 5.1v1.4a1.5 1.5 0 0 0 1.5 1.5h5A1.5 1.5 0 0 0 12 6.5V5.1M8 8v2.9" />
    </Svg>
  );
}
