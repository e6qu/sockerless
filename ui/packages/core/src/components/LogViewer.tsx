export interface LogViewerProps {
  lines: string[];
  maxHeight?: string;
}

/** Map common ANSI SGR codes to CSS colors. */
const ansiColors: Record<string, string> = {
  "30": "#000", "31": "#c00", "32": "#0a0", "33": "#a50",
  "34": "#00c", "35": "#c0c", "36": "#0cc", "37": "#ccc",
  "90": "#555", "91": "#f55", "92": "#5f5", "93": "#ff5",
  "94": "#55f", "95": "#f5f", "96": "#5ff", "97": "#fff",
};

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function ansiToHtml(text: string): string {
  const escaped = escapeHtml(text);
  let open = false;
  const result = escaped.replace(
    /\x1b\[([0-9;]*)m/g,
    (_, codes: string) => {
      const parts = codes.split(";");
      const styles: string[] = [];
      let hasReset = false;
      for (const code of parts) {
        if (code === "0" || code === "") {
          hasReset = true;
          continue;
        }
        if (code === "1") styles.push("font-weight:bold");
        const color = ansiColors[code];
        if (color) styles.push(`color:${color}`);
      }
      let out = "";
      if (open && (hasReset || styles.length > 0)) {
        out += "</span>";
        open = false;
      }
      if (styles.length > 0) {
        out += `<span style="${styles.join(";")}">`;
        open = true;
      }
      return out;
    },
  );
  return open ? result + "</span>" : result;
}

export function LogViewer({ lines, maxHeight = "24rem" }: LogViewerProps) {
  if (lines.length === 0) {
    return (
      <div className="rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm text-gray-400 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-500">
        No log output
      </div>
    );
  }

  return (
    <div
      className="overflow-auto rounded-lg border border-gray-200 bg-gray-950 dark:border-gray-700"
      style={{ maxHeight }}
    >
      <pre className="p-3 text-xs leading-5 text-gray-200">
        {lines.map((line, i) => (
          <div key={i} className="flex">
            <span className="mr-3 inline-block w-8 select-none text-right text-gray-600">
              {i + 1}
            </span>
            <span dangerouslySetInnerHTML={{ __html: ansiToHtml(line) }} />
          </div>
        ))}
      </pre>
    </div>
  );
}
