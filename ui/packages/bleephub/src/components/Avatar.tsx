/*
 * Account avatar — renders the account's avatar image when the server
 * supplies one, otherwise a monogram tile from the login's first letter.
 * GitHub renders every account (user, org, team) with a rounded avatar;
 * this keeps that shape without depending on an external image host.
 */
export function Avatar({
  login,
  src,
  size = 40,
  square = false,
}: {
  login: string;
  src?: string | null;
  size?: number;
  square?: boolean;
}) {
  const radius = square ? "var(--radius-md)" : "50%";
  if (src) {
    return (
      <img
        src={src}
        alt=""
        width={size}
        height={size}
        style={{
          width: size,
          height: size,
          borderRadius: radius,
          objectFit: "cover",
          border: "1px solid var(--color-border)",
          flexShrink: 0,
          background: "var(--color-bg-subtle)",
        }}
      />
    );
  }
  return (
    <span
      aria-hidden
      className="inline-flex items-center justify-center"
      style={{
        width: size,
        height: size,
        borderRadius: radius,
        border: "1px solid var(--color-border)",
        background: "var(--color-bg-subtle)",
        color: "var(--color-fg-muted)",
        fontWeight: 600,
        fontSize: size * 0.42,
        flexShrink: 0,
        textTransform: "uppercase",
      }}
    >
      {(login || "?").charAt(0)}
    </span>
  );
}
