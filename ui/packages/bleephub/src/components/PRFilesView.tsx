import { useQuery } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import { fetchPRFiles } from "../api.js";
import type { GithubPRFile } from "../types.js";
import { Box, Blankslate } from "./ui.js";
import { FileIcon } from "./octicons.js";

/** Color a unified-diff line by its leading marker. */
function diffLineStyle(line: string): { bg: string; fg: string } {
  if (line.startsWith("@@")) {
    return { bg: "color-mix(in srgb, var(--color-accent) 10%, transparent)", fg: "var(--color-accent)" };
  }
  if (line.startsWith("+")) {
    return { bg: "color-mix(in srgb, var(--gh-open) 14%, transparent)", fg: "var(--color-fg)" };
  }
  if (line.startsWith("-")) {
    return {
      bg: "color-mix(in srgb, var(--color-status-error) 14%, transparent)",
      fg: "var(--color-fg)",
    };
  }
  return { bg: "transparent", fg: "var(--color-fg)" };
}

function FileDiff({ file }: { file: GithubPRFile }) {
  return (
    <div className="mb-3">
      <Box
        header={
          <span className="flex min-w-0 flex-1 items-center gap-2">
            <FileIcon size={14} style={{ color: "var(--color-fg-muted)", flexShrink: 0 }} />
            <span className="font-mono min-w-0 flex-1 truncate" style={{ color: "var(--color-fg)" }}>
              {file.previous_filename && file.previous_filename !== file.filename
                ? `${file.previous_filename} → ${file.filename}`
                : file.filename}
            </span>
            <span className="tabular-nums" style={{ color: "var(--gh-open)", fontSize: "0.76rem" }}>
              +{file.additions}
            </span>
            <span
              className="tabular-nums"
              style={{ color: "var(--color-status-error)", fontSize: "0.76rem" }}
            >
              −{file.deletions}
            </span>
          </span>
        }
      >
        {file.patch ? (
          <div style={{ overflowX: "auto" }}>
            {file.patch.split("\n").map((line, i) => {
              const s = diffLineStyle(line);
              return (
                <pre
                  key={i}
                  className="font-mono"
                  style={{
                    margin: 0,
                    padding: "0 1rem",
                    fontSize: "0.76rem",
                    lineHeight: 1.6,
                    whiteSpace: "pre",
                    background: s.bg,
                    color: s.fg,
                  }}
                >
                  {line || " "}
                </pre>
              );
            })}
          </div>
        ) : (
          <div style={{ padding: "0.6rem 1rem", fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
            {file.status === "removed"
              ? "File removed."
              : file.status === "added"
                ? "New file."
                : "Binary file or no textual diff."}
          </div>
        )}
      </Box>
    </div>
  );
}

/** GitHub's "Files changed" tab — the PR's changed files rendered as diffs. */
export function PRFilesView({ owner, repo, number }: { owner: string; repo: string; number: number }) {
  const q = useQuery({
    queryKey: ["pr-files", owner, repo, number],
    queryFn: () => fetchPRFiles(owner, repo, number),
  });

  if (q.isLoading) return <Spinner label="loading changed files" />;
  if (q.isError) return <InlineError title="Failed to load changed files" detail={String(q.error)} />;
  const files = q.data ?? [];
  if (files.length === 0) {
    return <Blankslate icon={<FileIcon size={26} />} title="No file changes" />;
  }

  const totalAdd = files.reduce((n, f) => n + f.additions, 0);
  const totalDel = files.reduce((n, f) => n + f.deletions, 0);
  return (
    <div>
      <div className="mb-3" style={{ fontSize: "0.83rem", color: "var(--color-fg-muted)" }}>
        Showing {files.length} changed file{files.length === 1 ? "" : "s"} with{" "}
        <span style={{ color: "var(--gh-open)" }}>{totalAdd} additions</span> and{" "}
        <span style={{ color: "var(--color-status-error)" }}>{totalDel} deletions</span>.
      </div>
      {files.map((f) => (
        <FileDiff key={f.sha + f.filename} file={f} />
      ))}
    </div>
  );
}
