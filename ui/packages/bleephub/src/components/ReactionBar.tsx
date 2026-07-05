import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { InlineError } from "@sockerless/ui-core/components";
import type { GithubReaction, GithubReactionContent } from "../types.js";

export const REACTION_CONTENTS: GithubReactionContent[] = [
  "+1",
  "-1",
  "laugh",
  "confused",
  "heart",
  "hooray",
  "rocket",
  "eyes",
];

export const REACTION_EMOJI: Record<GithubReactionContent, string> = {
  "+1": "👍",
  "-1": "👎",
  laugh: "😄",
  confused: "😕",
  heart: "❤️",
  hooray: "🎉",
  rocket: "🚀",
  eyes: "👀",
};

/**
 * GitHub's reaction row for an issue/PR body or a comment. `viewerLogin`
 * identifies "my" reactions so the toggle can remove an existing one. Errors
 * surface via InlineError — never swallowed.
 */
export function ReactionBar({
  queryKey,
  fetchList,
  add,
  remove,
  viewerLogin,
}: {
  queryKey: (string | number)[];
  fetchList: () => Promise<GithubReaction[]>;
  add: (content: GithubReactionContent) => Promise<GithubReaction>;
  remove: (reactionId: number) => Promise<void>;
  viewerLogin: string | null;
}) {
  const qc = useQueryClient();
  const q = useQuery({ queryKey, queryFn: fetchList });
  const [pickerOpen, setPickerOpen] = useState(false);
  const toggle = useMutation({
    mutationFn: async (content: GithubReactionContent) => {
      const mine = (q.data ?? []).find(
        (r) => r.content === content && r.user?.login != null && r.user.login === viewerLogin,
      );
      if (mine) {
        await remove(mine.id);
      } else {
        await add(content);
      }
    },
    onSuccess: () => {
      setPickerOpen(false);
      qc.invalidateQueries({ queryKey });
    },
  });

  if (q.isLoading) return null;
  if (q.isError) {
    return <InlineError inline title="Failed to load reactions" detail={String(q.error)} />;
  }
  // A non-array body is a contract break — surface it, never iterate garbage.
  if (q.data !== undefined && !Array.isArray(q.data)) {
    return (
      <InlineError inline title="Failed to load reactions" detail="malformed response: expected an array" />
    );
  }
  const reactions = q.data ?? [];
  const byContent = new Map<GithubReactionContent, { count: number; mine: boolean }>();
  for (const r of reactions) {
    const entry = byContent.get(r.content) ?? { count: 0, mine: false };
    entry.count += 1;
    if (r.user?.login != null && r.user.login === viewerLogin) entry.mine = true;
    byContent.set(r.content, entry);
  }

  const pillStyle = (mine: boolean) =>
    ({
      border: mine ? "1px solid var(--color-accent)" : "1px solid var(--color-border)",
      background: mine ? "color-mix(in srgb, var(--color-accent) 12%, transparent)" : "transparent",
      borderRadius: "2rem",
      padding: "0.1rem 0.55rem",
      fontSize: "0.78rem",
      cursor: "pointer",
      color: "var(--color-fg)",
    }) as const;

  return (
    <div style={{ marginTop: "-0.6rem", marginBottom: "1rem" }}>
      <div className="flex flex-wrap items-center gap-1.5">
        {REACTION_CONTENTS.filter((c) => (byContent.get(c)?.count ?? 0) > 0).map((content) => {
          const entry = byContent.get(content);
          return (
            <button
              key={content}
              type="button"
              aria-label={`toggle ${content} reaction`}
              disabled={toggle.isPending}
              onClick={() => toggle.mutate(content)}
              style={pillStyle(entry?.mine ?? false)}
            >
              {REACTION_EMOJI[content]} {entry?.count}
            </button>
          );
        })}
        <button
          type="button"
          aria-label="add reaction"
          onClick={() => setPickerOpen((v) => !v)}
          style={{
            border: "1px solid var(--color-border)",
            background: "transparent",
            borderRadius: "2rem",
            padding: "0.1rem 0.55rem",
            fontSize: "0.78rem",
            cursor: "pointer",
            color: "var(--color-fg-muted)",
          }}
        >
          🙂＋
        </button>
        {pickerOpen &&
          REACTION_CONTENTS.map((content) => (
            <button
              key={content}
              type="button"
              aria-label={`react with ${content}`}
              disabled={toggle.isPending}
              onClick={() => toggle.mutate(content)}
              style={{
                border: "1px solid var(--color-border)",
                background: "var(--color-bg-subtle)",
                borderRadius: "0.4rem",
                padding: "0.1rem 0.35rem",
                fontSize: "0.85rem",
                cursor: "pointer",
              }}
            >
              {REACTION_EMOJI[content]}
            </button>
          ))}
      </div>
      {toggle.isError && (
        <InlineError inline title="Failed to update reaction" detail={String(toggle.error)} />
      )}
    </div>
  );
}
