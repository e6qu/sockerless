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

/** Brand mark — a chat bubble (a "bleep"), filled with the accent. */
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
      <rect x="2" y="3" width="20" height="15" rx="4" fill="var(--color-accent)" />
      <path d="M8 18l-2 3v-3" fill="var(--color-accent)" />
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

export function RefreshIcon(p: IconProps) {
  return (
    <Svg {...p}>
      <path d="M13 8a5 5 0 1 1-1.5-3.5" />
      <path d="M13 2.5V5h-2.5" />
    </Svg>
  );
}
