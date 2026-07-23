import { type CSSProperties } from "react";

// Cloudscape-style icons, drawn as inline SVG so the console is self-contained.
// They approximate the Cloudscape Design System icon set (16px grid, 2px
// strokes) rather than reproducing a proprietary asset.
const PATHS: Record<string, string> = {
  menu: "M2 4h12M2 8h12M2 12h12",
  search: "M7 2a5 5 0 1 0 3.2 8.8l3 3 1.4-1.4-3-3A5 5 0 0 0 7 2Zm0 2a3 3 0 1 1 0 6 3 3 0 0 1 0-6Z",
  settings:
    "M8 5.5A2.5 2.5 0 1 0 8 10.5 2.5 2.5 0 0 0 8 5.5Zm6-.3-1.2-.4a5 5 0 0 0-.4-.9l.5-1.1-1.4-1.4-1.1.5a5 5 0 0 0-.9-.4L9.1 0H6.9l-.4 1.2a5 5 0 0 0-.9.4l-1.1-.5L3.1 2.5l.5 1.1a5 5 0 0 0-.4.9L2 4.9v2.2l1.2.4a5 5 0 0 0 .4.9l-.5 1.1 1.4 1.4 1.1-.5a5 5 0 0 0 .9.4l.4 1.2h2.2l.4-1.2a5 5 0 0 0 .9-.4l1.1.5 1.4-1.4-.5-1.1a5 5 0 0 0 .4-.9L14 7.1Z",
  notification: "M8 2a3 3 0 0 0-3 3v3l-1 2h8l-1-2V5a3 3 0 0 0-3-3Zm0 12a1.5 1.5 0 0 0 1.5-1.5h-3A1.5 1.5 0 0 0 8 14Z",
  help: "M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1Zm0 11.5a1 1 0 1 1 0-2 1 1 0 0 1 0 2Zm1.1-4.2c-.5.3-.6.5-.6.9v.3H7.5v-.4c0-.9.4-1.3 1-1.7.4-.3.6-.5.6-.9 0-.5-.4-.8-1-.8-.5 0-.9.3-1 .8l-1.3-.3C6 4.8 6.9 4.2 8 4.2c1.3 0 2.3.7 2.3 1.9 0 .9-.5 1.4-1.2 1.9Z",
  refresh:
    "M8 2.5A5.5 5.5 0 0 0 2.6 7H1l2.2 2.6L5.4 7H3.9A4.1 4.1 0 1 1 8 12.5a4.1 4.1 0 0 1-3.5-2l-1.2.7A5.5 5.5 0 1 0 8 2.5Z",
  filter: "M2 3h12l-4.5 5.5V13l-3 1V8.5Z",
  caret: "M4 6l4 4 4-4Z",
  external: "M6 3v1.5H4.5v7h7V10H13v3.5H3V3Zm3 0h4v4h-1.5V5.6L8 9 6.9 8 10.4 4.5H9Z",
  status_positive: "M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1ZM7 11 3.5 7.5 4.6 6.4 7 8.8l4.4-4.4 1.1 1.1Z",
  status_negative: "M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1Zm3 8.9L9.9 11 8 9.1 6.1 11 5 9.9 6.9 8 5 6.1 6.1 5 8 6.9 9.9 5 11 6.1 9.1 8Z",
  status_pending: "M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1Zm.8 7.2L11 10l-.8 1L7 8.4V4h1.5Z",
  sun: "M8 4.5A3.5 3.5 0 1 0 8 11.5 3.5 3.5 0 0 0 8 4.5ZM8 0v2M8 14v2M0 8h2M14 8h2M2.3 2.3l1.4 1.4M12.3 12.3l1.4 1.4M13.7 2.3l-1.4 1.4M3.7 12.3l-1.4 1.4",
  moon: "M6 1.5A6.5 6.5 0 1 0 14.5 10 5 5 0 0 1 6 1.5Z",
  account: "M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1Zm0 3a2.3 2.3 0 1 1 0 4.6A2.3 2.3 0 0 1 8 4Zm0 9.5a5 5 0 0 1-3.8-1.8C4.3 10.4 6 9.6 8 9.6s3.7.8 3.8 2.1A5 5 0 0 1 8 13.5Z",
};

const STROKE = new Set(["menu", "filter", "caret", "sun"]);

export type AwsIconName = keyof typeof PATHS;

export function AwsIcon({ name, size, style }: { name: AwsIconName; size?: number | string; style?: CSSProperties }) {
  const dimension = size ?? "1em";
  const stroked = STROKE.has(name);
  return (
    <svg
      viewBox="0 0 16 16"
      width={dimension}
      height={dimension}
      fill={stroked ? "none" : "currentColor"}
      stroke={stroked ? "currentColor" : "none"}
      strokeWidth={stroked ? 1.6 : 0}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
      focusable="false"
      style={{ display: "inline-block", flexShrink: 0, verticalAlign: "middle", ...style }}
    >
      <path d={PATHS[name]} />
    </svg>
  );
}
