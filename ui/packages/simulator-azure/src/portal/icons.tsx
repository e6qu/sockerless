import { type CSSProperties } from "react";

// Fluent-style icons, drawn as inline SVG so the portal is self-contained. They
// approximate the Fluent UI System Icons set (Microsoft, MIT-licensed) — a 20px
// grid with the family's rounded strokes — rather than reproducing a specific
// asset. Each is an original rendering in that style.
const PATHS: Record<string, string> = {
  menu: "M2.5 5.5h15M2.5 10h15M2.5 14.5h15",
  search: "M8.5 3a5.5 5.5 0 1 1-3.9 9.4 5.5 5.5 0 0 1 3.9-9.4Zm0 1.5a4 4 0 1 0 0 8 4 4 0 0 0 0-8Zm4.9 7.8 4 4",
  refresh:
    "M10 4.5a5.5 5.5 0 0 0-5.3 4H3.2l2.4 3 2.4-3H6.3A4 4 0 1 1 6 12.6l-1.3.9A5.5 5.5 0 1 0 10 4.5Z",
  delete:
    "M8 2.5h4a1 1 0 0 1 1 1V4h3v1.5h-1V16a1.5 1.5 0 0 1-1.5 1.5h-7A1.5 1.5 0 0 1 5 16V5.5H4V4h3v-.5a1 1 0 0 1 1-1Zm-1.5 3V16h7V5.5h-7ZM8 4v0h4V4H8Zm0 4h1.5v6H8V8Zm2.5 0H12v6h-1.5V8Z",
  tag: "M3 3h6.2a1.5 1.5 0 0 1 1 .4l6 6a1.5 1.5 0 0 1 0 2.1l-4.3 4.3a1.5 1.5 0 0 1-2.1 0l-6-6a1.5 1.5 0 0 1-.4-1V4a1 1 0 0 1 1-1Zm3 2.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3Z",
  download: "M10 2.5v8.5m0 0 3.5-3.5M10 11 6.5 7.5M4 14.5v1a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1v-1",
  feedback:
    "M4 3.5h12a1.5 1.5 0 0 1 1.5 1.5v7A1.5 1.5 0 0 1 16 13.5H8l-3.5 3v-3H4A1.5 1.5 0 0 1 2.5 12V5A1.5 1.5 0 0 1 4 3.5Z",
  add: "M10 4v12M4 10h12",
  copy: "M7 3h6.5A1.5 1.5 0 0 1 15 4.5V13M5.5 6H12a1.5 1.5 0 0 1 1.5 1.5V16A1.5 1.5 0 0 1 12 17.5H5.5A1.5 1.5 0 0 1 4 16V7.5A1.5 1.5 0 0 1 5.5 6Z",
  chevron_down: "M5 7.5 10 12l5-4.5",
  chevron_right: "M7.5 5 12 10l-4.5 5",
  sort: "M6 4v12M6 16 3.5 13M6 16l2.5-3M14 16V4M14 4l-2.5 3M14 4l2.5 3",
  sort_up: "M6 15V5M6 5 3 8M6 5l3 3M13 15h4M11 11h6M13 7h2",
  sort_down: "M6 5v10M6 15l-3-3M6 15l3-3M13 5h2M11 9h6M13 13h4",
  sun: "M10 5.5A4.5 4.5 0 1 0 10 14.5 4.5 4.5 0 0 0 10 5.5ZM10 1v2.2M10 16.8V19M1 10h2.2M16.8 10H19M3.5 3.5l1.6 1.6M14.9 14.9l1.6 1.6M16.5 3.5l-1.6 1.6M5.1 14.9l-1.6 1.6",
  moon: "M7 2.6A7.4 7.4 0 1 0 17.4 13 5.7 5.7 0 0 1 7 2.6Z",
  status_success: "M10 2.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 0 0 0-15Zm3.7 5.3-4.4 4.4a1 1 0 0 1-1.4 0L6 10.3l1.1-1.1 1.5 1.5 3.9-3.9 1.2 1Z",
  status_error: "M10 2.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 0 0 0-15Zm2.8 4.1L10 9.4 7.2 6.6 6.1 7.7 8.9 10.5l-2.8 2.8 1.1 1.1L10 11.6l2.8 2.8 1.1-1.1-2.8-2.8 2.8-2.8-1.1-1.1Z",
  status_warning: "M10 2.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 0 0 0-15Zm.9 4L10.7 11H9.3L9.1 6.5h1.8ZM10 12.2a1 1 0 1 1 0 2 1 1 0 0 1 0-2Z",
  status_inactive: "M10 2.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 0 0 0-15Zm0 6a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3Z",
  not_supported: "M10 2.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 0 0 0-15ZM4.7 4.7l10.6 10.6",
  play: "M7 5v10l8-5-8-5Z",
  stop: "M6 6h8v8H6V6Z",
  notifications:
    "M10 3.5a4 4 0 0 0-4 4v2.6c0 .6-.2 1.2-.6 1.7L4 13.5h12l-1.4-1.7a2.7 2.7 0 0 1-.6-1.7V7.5a4 4 0 0 0-4-4Z M8.3 15.5a1.7 1.7 0 0 0 3.4 0",
  help: "M10 2.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 0 0 0-15ZM8.4 8.1a1.8 1.8 0 1 1 2.6 1.6c-.6.3-1 .7-1 1.3v.4M10 13.9v.1",
  cloud_shell:
    "M3.5 5.5h13a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1h-13a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1Z M5.5 8.3l2.6 1.7-2.6 1.7 M9.7 12.5h3.3",
};

// Which icons are drawn as strokes (line icons) rather than filled shapes.
const STROKE = new Set([
  "menu",
  "search",
  "download",
  "add",
  "copy",
  "chevron_down",
  "chevron_right",
  "sort",
  "sort_up",
  "sort_down",
  "sun",
  "not_supported",
  "notifications",
  "help",
  "cloud_shell",
]);

export type AzureIconName = keyof typeof PATHS;

export function AzureIcon({ name, size, style }: { name: AzureIconName; size?: number | string; style?: CSSProperties }) {
  const dimension = size ?? "1em";
  const stroked = STROKE.has(name);
  return (
    <svg
      viewBox="0 0 20 20"
      width={dimension}
      height={dimension}
      fill={stroked ? "none" : "currentColor"}
      stroke={stroked ? "currentColor" : "none"}
      strokeWidth={stroked ? 1.5 : 0}
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
