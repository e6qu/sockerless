import {
  type ButtonHTMLAttributes,
  type CSSProperties,
  type ReactNode,
  forwardRef,
} from "react";

/*
 * bleephub UI primitives — GitHub-familiar shapes (sans type, soft 6px
 * boxes, normal-case buttons, solid state pills). These intentionally do
 * NOT reuse the ui-core brutalist primitives (PageHeading/Button/
 * MetricsCard), whose italic-serif + uppercase-mono shapes can't be
 * reshaped by tokens alone. They DO read the same --color-* tokens, so
 * light/dark just works.
 */

// ─── Button ──────────────────────────────────────────────────────────────

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
export type ButtonSize = "sm" | "md";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = "secondary", size = "md", className = "", style, children, ...rest },
  ref,
) {
  const sizeStyle: CSSProperties =
    size === "sm"
      ? { padding: "0.2rem 0.65rem", fontSize: "0.78rem" }
      : { padding: "0.34rem 0.85rem", fontSize: "0.82rem" };

  const variantStyle: CSSProperties = (() => {
    switch (variant) {
      case "primary":
        return {
          background: "var(--gh-open-solid)",
          color: "#ffffff",
          border: "1px solid color-mix(in srgb, #000 12%, var(--gh-open-solid))",
        };
      case "secondary":
        return {
          background: "var(--color-bg-subtle)",
          color: "var(--color-fg)",
          border: "1px solid var(--color-border)",
        };
      case "ghost":
        return {
          background: "transparent",
          color: "var(--color-accent)",
          border: "1px solid transparent",
        };
      case "danger":
        return {
          background: "var(--color-bg-subtle)",
          color: "var(--color-status-error)",
          border: "1px solid var(--color-border)",
        };
    }
  })();

  return (
    <button
      ref={ref}
      className={`inline-flex items-center justify-center gap-1.5 ${className}`}
      style={{
        ...variantStyle,
        ...sizeStyle,
        fontFamily: "var(--font-sans)",
        fontWeight: 600,
        borderRadius: "var(--radius-md)",
        whiteSpace: "nowrap",
        transition: "filter 0.1s var(--ease-out-quint), background-color 0.1s var(--ease-out-quint)",
        ...style,
      }}
      {...rest}
    >
      {children}
    </button>
  );
});

// ─── PageTitle ───────────────────────────────────────────────────────────

export function PageTitle({
  icon,
  title,
  meta,
  actions,
}: {
  icon?: ReactNode;
  title: ReactNode;
  meta?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <header
      className="mb-5 flex flex-wrap items-start justify-between gap-x-6 gap-y-3 border-b pb-4"
      style={{ borderColor: "var(--color-border)" }}
    >
      <div className="min-w-0 flex-1">
        <h1
          className="flex items-center gap-2"
          style={{ fontSize: "1.45rem", fontWeight: 600, color: "var(--color-fg)", lineHeight: 1.2 }}
        >
          {icon}
          <span className="min-w-0 break-words">{title}</span>
        </h1>
        {meta && (
          <div className="mt-1.5" style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
            {meta}
          </div>
        )}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  );
}

// ─── Box ─────────────────────────────────────────────────────────────────

/** Bordered, rounded container — GitHub's "Box". Optional header strip. */
export function Box({
  header,
  children,
  style,
  className = "",
}: {
  header?: ReactNode;
  children: ReactNode;
  style?: CSSProperties;
  className?: string;
}) {
  return (
    <div
      className={className}
      style={{
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-md)",
        background: "var(--color-surface)",
        overflow: "hidden",
        ...style,
      }}
    >
      {header != null && (
        <div
          className="flex items-center gap-2"
          style={{
            padding: "0.6rem 1rem",
            background: "var(--color-bg-subtle)",
            borderBottom: "1px solid var(--color-border)",
            fontSize: "0.82rem",
            color: "var(--color-fg-muted)",
          }}
        >
          {header}
        </div>
      )}
      {children}
    </div>
  );
}

// ─── Blankslate ────────────────────────────────────────────────────────

export function Blankslate({
  icon,
  title,
  children,
}: {
  icon?: ReactNode;
  title: string;
  children?: ReactNode;
}) {
  return (
    <div
      className="flex flex-col items-center text-center"
      style={{
        padding: "2.75rem 1.5rem",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-md)",
        background: "var(--color-surface)",
        color: "var(--color-fg-muted)",
      }}
    >
      {icon && <div style={{ color: "var(--color-fg-subtle)", marginBottom: "0.6rem" }}>{icon}</div>}
      <div style={{ fontSize: "0.95rem", fontWeight: 600, color: "var(--color-fg)" }}>{title}</div>
      {children && (
        <div style={{ marginTop: "0.4rem", fontSize: "0.85rem", maxWidth: "30rem" }}>{children}</div>
      )}
    </div>
  );
}

// ─── State label (solid pill) ──────────────────────────────────────────

export type IssuePRState = "open" | "closed" | "merged" | "draft";

const stateColor: Record<IssuePRState, string> = {
  open: "var(--gh-open-solid)",
  closed: "var(--gh-closed)",
  merged: "var(--gh-merged)",
  draft: "var(--gh-draft)",
};

export function StateLabel({
  state,
  icon,
  children,
}: {
  state: IssuePRState;
  icon?: ReactNode;
  children: ReactNode;
}) {
  return (
    <span
      className="inline-flex items-center gap-1.5"
      style={{
        background: stateColor[state],
        color: "#ffffff",
        borderRadius: "2rem",
        padding: "0.28rem 0.7rem",
        fontSize: "0.8rem",
        fontWeight: 600,
        lineHeight: 1,
      }}
    >
      {icon}
      {children}
    </span>
  );
}

// ─── Counter bubble ────────────────────────────────────────────────────

export function Counter({ children }: { children: ReactNode }) {
  return (
    <span
      style={{
        display: "inline-block",
        minWidth: "1.25rem",
        textAlign: "center",
        padding: "0.05rem 0.4rem",
        background: "color-mix(in srgb, var(--color-fg-muted) 16%, transparent)",
        color: "var(--color-fg-muted)",
        borderRadius: "2rem",
        fontSize: "0.72rem",
        fontWeight: 600,
        lineHeight: 1.4,
      }}
    >
      {children}
    </span>
  );
}

// ─── StatCard ──────────────────────────────────────────────────────────

export function StatCard({
  title,
  value,
  emphasized,
}: {
  title: string;
  value: string | number;
  emphasized?: boolean;
}) {
  return (
    <div
      style={{
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-md)",
        background: "var(--color-surface)",
        padding: "0.85rem 1rem",
      }}
    >
      <div style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>{title}</div>
      <div
        className="tabular-nums"
        style={{
          marginTop: "0.2rem",
          fontSize: "1.7rem",
          fontWeight: 600,
          lineHeight: 1.1,
          color: emphasized ? "var(--color-accent)" : "var(--color-fg)",
        }}
      >
        {value}
      </div>
    </div>
  );
}

// ─── Section label ──────────────────────────────────────────────────────

export function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <h2
      style={{
        fontSize: "0.92rem",
        fontWeight: 600,
        color: "var(--color-fg)",
        marginBottom: "0.6rem",
      }}
    >
      {children}
    </h2>
  );
}

// ─── Tabs ────────────────────────────────────────────────────────────────

export interface TabItem<K extends string> {
  key: K;
  label: ReactNode;
}

/** Underline tab strip (GitHub's in-page tab nav). */
export function Tabs<K extends string>({
  items,
  active,
  onChange,
}: {
  items: TabItem<K>[];
  active: K;
  onChange: (key: K) => void;
}) {
  return (
    <div className="mb-5 flex flex-wrap gap-1" style={{ borderBottom: "1px solid var(--color-border)" }}>
      {items.map((t) => (
        <button
          key={t.key}
          type="button"
          onClick={() => onChange(t.key)}
          style={{
            padding: "0.45rem 0.7rem",
            marginBottom: "-1px",
            fontSize: "0.86rem",
            fontWeight: active === t.key ? 600 : 500,
            color: active === t.key ? "var(--color-fg)" : "var(--color-fg-muted)",
            background: "transparent",
            border: "none",
            borderBottom: `2px solid ${active === t.key ? "var(--color-accent)" : "transparent"}`,
          }}
        >
          {t.label}
        </button>
      ))}
    </div>
  );
}

// ─── Code block ──────────────────────────────────────────────────────────

export function CodeBlock({ children }: { children: ReactNode }) {
  return (
    <pre
      style={{
        background: "var(--color-bg-subtle)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-md)",
        padding: "0.7rem 0.85rem",
        fontFamily: "var(--font-mono)",
        fontSize: "0.78rem",
        color: "var(--color-fg)",
        overflow: "auto",
        whiteSpace: "pre-wrap",
        wordBreak: "break-all",
        margin: 0,
      }}
    >
      {children}
    </pre>
  );
}

// ─── Modal ───────────────────────────────────────────────────────────────

export function Modal({
  title,
  onClose,
  children,
}: {
  title: ReactNode;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-auto p-6"
      style={{ background: "color-mix(in srgb, #1f2328 50%, transparent)" }}
      onClick={onClose}
    >
      <div
        className="w-full max-w-lg my-auto"
        style={{
          background: "var(--color-surface-raised)",
          border: "1px solid var(--color-border)",
          borderRadius: "0.75rem",
          boxShadow: "0 8px 24px rgba(31,35,40,0.2)",
          maxHeight: "90vh",
          overflowY: "auto",
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div
          className="flex items-center justify-between"
          style={{
            padding: "0.85rem 1rem",
            borderBottom: "1px solid var(--color-border)",
          }}
        >
          <h2 style={{ fontSize: "1rem", fontWeight: 600, color: "var(--color-fg)" }}>{title}</h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            style={{
              border: "none",
              background: "transparent",
              color: "var(--color-fg-muted)",
              fontSize: "1.1rem",
              lineHeight: 1,
              padding: "0.2rem",
            }}
          >
            ✕
          </button>
        </div>
        <div style={{ padding: "1rem" }}>{children}</div>
      </div>
    </div>
  );
}

export function FormLabel({ id, children }: { id?: string; children: ReactNode }) {
  return (
    <label
      htmlFor={id}
      className="mb-1 block"
      style={{ fontSize: "0.82rem", fontWeight: 600, color: "var(--color-fg)" }}
    >
      {children}
    </label>
  );
}

export function ErrorBanner({ children }: { children: ReactNode }) {
  return (
    <div
      className="mb-4"
      style={{
        padding: "0.5rem 0.75rem",
        background: "var(--color-status-error-soft)",
        color: "var(--color-status-error)",
        border: "1px solid color-mix(in srgb, var(--color-status-error) 40%, transparent)",
        borderRadius: "var(--radius-md)",
        fontSize: "0.82rem",
      }}
    >
      {children}
    </div>
  );
}

export function DialogActions({ children }: { children: ReactNode }) {
  return <div className="flex justify-end gap-2 pt-1">{children}</div>;
}
