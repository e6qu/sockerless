import { parse } from "yaml";
import type { WorkflowDispatchInput } from "../types.js";

/** Parsed `on.workflow_dispatch` section of a workflow file. */
export interface WorkflowDispatchSpec {
  hasDispatch: boolean;
  /** Input name → definition, in YAML declaration order. */
  inputs: Record<string, WorkflowDispatchInput>;
}

/** Decode the base64 `content` member of a GitHub contents response (UTF-8). */
export function decodeContentsBase64(b64: string): string {
  const bin = atob(b64.replace(/\s/g, ""));
  const bytes = Uint8Array.from(bin, (c) => c.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

function normalizeInput(raw: unknown): WorkflowDispatchInput {
  if (raw === null || typeof raw !== "object") return {};
  const r = raw as Record<string, unknown>;
  const input: WorkflowDispatchInput = {};
  if (typeof r.description === "string") input.description = r.description;
  if (typeof r.required === "boolean") input.required = r.required;
  if (typeof r.default === "string" || typeof r.default === "boolean") input.default = r.default;
  if (typeof r.default === "number") input.default = String(r.default);
  if (
    r.type === "string" ||
    r.type === "choice" ||
    r.type === "boolean" ||
    r.type === "environment" ||
    r.type === "number"
  ) {
    input.type = r.type;
  }
  if (Array.isArray(r.options)) {
    input.options = r.options.map((o) => String(o));
  }
  return input;
}

/**
 * Read `on.workflow_dispatch` (and its `inputs`) out of workflow YAML.
 * Handles every shape `on:` takes: a scalar event name, an event list,
 * or an event map. A YAML-1.1-minded parser can also turn the bare `on`
 * key into boolean true (serialized as the "true" key) — accept both.
 * Unparsable YAML means "no dispatch form", never a crash.
 */
export function parseWorkflowDispatch(yamlText: string): WorkflowDispatchSpec {
  let doc: unknown;
  try {
    doc = parse(yamlText);
  } catch {
    return { hasDispatch: false, inputs: {} };
  }
  if (doc === null || typeof doc !== "object") return { hasDispatch: false, inputs: {} };
  const root = doc as Record<string, unknown>;
  const on = root.on ?? root.true;
  if (on === undefined || on === null) return { hasDispatch: false, inputs: {} };

  if (typeof on === "string") {
    return { hasDispatch: on === "workflow_dispatch", inputs: {} };
  }
  if (Array.isArray(on)) {
    return { hasDispatch: on.includes("workflow_dispatch"), inputs: {} };
  }
  if (typeof on === "object") {
    const events = on as Record<string, unknown>;
    if (!("workflow_dispatch" in events)) return { hasDispatch: false, inputs: {} };
    const wd = events.workflow_dispatch;
    const inputs: Record<string, WorkflowDispatchInput> = {};
    if (wd !== null && typeof wd === "object") {
      const rawInputs = (wd as Record<string, unknown>).inputs;
      if (rawInputs !== null && typeof rawInputs === "object" && !Array.isArray(rawInputs)) {
        for (const [name, def] of Object.entries(rawInputs as Record<string, unknown>)) {
          inputs[name] = normalizeInput(def);
        }
      }
    }
    return { hasDispatch: true, inputs };
  }
  return { hasDispatch: false, inputs: {} };
}
