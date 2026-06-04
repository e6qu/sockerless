import type { MouseEventHandler } from "react";

/** Shared hover style props for table row <Link> elements. */
export const rowHoverProps: {
  onMouseEnter: MouseEventHandler<HTMLAnchorElement>;
  onMouseLeave: MouseEventHandler<HTMLAnchorElement>;
} = {
  onMouseEnter: (e) => {
    e.currentTarget.style.background = "var(--color-bg-subtle)";
  },
  onMouseLeave: (e) => {
    e.currentTarget.style.background = "var(--color-surface-raised)";
  },
};
