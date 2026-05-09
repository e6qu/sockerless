import { useTheme } from "../hooks/useTheme.js";

/**
 * Compact light/dark toggle. Lives in the AppShell's left sidebar
 * footer so every backend UI gets it for free. Sun/moon icon flips
 * by class — no JS-side icon swap, easier to keep accessible.
 */
export function ThemeToggle() {
  const { theme, toggle } = useTheme();
  const isDark = theme === "dark";
  return (
    <button
      type="button"
      onClick={toggle}
      aria-label={isDark ? "Switch to light theme" : "Switch to dark theme"}
      title={isDark ? "Switch to light theme" : "Switch to dark theme"}
      className="inline-flex h-8 w-8 items-center justify-center"
      style={{
        background: "transparent",
        color: "var(--color-fg-muted)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-sm)",
        transition: "color 0.12s var(--ease-out-quint), border-color 0.12s var(--ease-out-quint)",
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.color = "var(--color-fg)";
        e.currentTarget.style.borderColor = "var(--color-border-strong)";
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.color = "var(--color-fg-muted)";
        e.currentTarget.style.borderColor = "var(--color-border)";
      }}
    >
      <span aria-hidden style={{ fontSize: 14, lineHeight: 1 }}>
        {isDark ? "☀" : "☾"}
      </span>
    </button>
  );
}
