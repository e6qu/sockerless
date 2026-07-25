import { useId, useRef, useState, type KeyboardEvent, type ReactNode } from "react";

/**
 * The Cloudscape "Details page with tabs" pattern: a persistent summary
 * container (the caller's own key-value block, rendered above this
 * component) followed by task-oriented tabs — "start with the details of the
 * resource, followed by the most frequently accessed information type" is
 * Cloudscape's own ordering guidance (cloudscape.design/patterns/
 * resource-management/details/details-page-with-tabs/).
 */
export interface AwsTab {
  id: string;
  label: string;
  content: ReactNode;
}

const ARROW_KEYS = new Set(["ArrowLeft", "ArrowRight", "Home", "End"]);

/**
 * A tab strip built to the WAI-ARIA Tabs pattern Cloudscape's own Tabs
 * component implements: one `role="tablist"`, each label a `role="tab"` with
 * a roving `tabIndex` (only the active tab sits in the page's Tab order —
 * Left/Right/Home/End move among the rest), and one `role="tabpanel"` per
 * tab, rendered only while active so its content is never announced twice.
 */
export function AwsTabs({ tabs, ariaLabel }: { tabs: AwsTab[]; ariaLabel: string }) {
  const baseId = useId();
  const [activeId, setActiveId] = useState(tabs[0]?.id);
  const buttonRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const active = tabs.find((tab) => tab.id === activeId) ?? tabs[0];

  function activate(id: string, focus: boolean) {
    setActiveId(id);
    if (focus) buttonRefs.current[id]?.focus();
  }

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    if (!ARROW_KEYS.has(event.key)) return;
    event.preventDefault();
    const last = tabs.length - 1;
    const next =
      event.key === "ArrowLeft" ? (index === 0 ? last : index - 1)
      : event.key === "ArrowRight" ? (index === last ? 0 : index + 1)
      : event.key === "Home" ? 0
      : last;
    activate(tabs[next].id, true);
  }

  return (
    <div className="aws-tabs">
      <div role="tablist" aria-label={ariaLabel} className="aws-tabs-list">
        {tabs.map((tab, index) => {
          const selected = tab.id === active?.id;
          return (
            <button
              key={tab.id}
              ref={(node) => {
                buttonRefs.current[tab.id] = node;
              }}
              type="button"
              role="tab"
              id={`${baseId}-tab-${tab.id}`}
              aria-selected={selected}
              aria-controls={`${baseId}-panel-${tab.id}`}
              tabIndex={selected ? 0 : -1}
              className={selected ? "aws-tab aws-tab-active" : "aws-tab"}
              onClick={() => activate(tab.id, false)}
              onKeyDown={(event) => onKeyDown(event, index)}
            >
              {tab.label}
            </button>
          );
        })}
      </div>
      {tabs.map((tab) => (
        <div
          key={tab.id}
          role="tabpanel"
          id={`${baseId}-panel-${tab.id}`}
          aria-labelledby={`${baseId}-tab-${tab.id}`}
          hidden={tab.id !== active?.id}
          tabIndex={0}
          className="aws-tab-panel"
        >
          {tab.id === active?.id ? tab.content : null}
        </div>
      ))}
    </div>
  );
}
