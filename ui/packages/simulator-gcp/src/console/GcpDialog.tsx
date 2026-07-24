import { type ReactNode } from "react";

// The console's modal dialog: a scrim over the working area with a white card,
// title at the top, content beneath, and the caller's actions (text buttons,
// primary at the right) at the bottom.
export function GcpDialog({
  title,
  children,
  testId,
}: {
  title: string;
  children: ReactNode;
  testId?: string;
}) {
  return (
    <div className="gc-dialog-overlay" role="presentation">
      <div className="gc-dialog" role="dialog" aria-modal="true" aria-label={title} data-testid={testId}>
        <h2 className="gc-dialog-title">{title}</h2>
        {children}
      </div>
    </div>
  );
}
