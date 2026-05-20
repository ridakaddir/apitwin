import type { Condition } from "../../types/config";
import { FieldLabel, GhostButton, SectionCard, Select, TextInput } from "./SectionCard";

const SOURCES = [
  { value: "body", label: "body" },
  { value: "query", label: "query" },
  { value: "header", label: "header" },
];

const OPS = [
  { value: "eq", label: "equals" },
  { value: "neq", label: "not equals" },
  { value: "contains", label: "contains" },
  { value: "regex", label: "regex" },
  { value: "exists", label: "exists" },
  { value: "not_exists", label: "does not exist" },
];

export function ConditionsSection({
  conditions,
  caseNames,
  onAdd,
  onRemove,
  onPatch,
}: {
  conditions: Condition[];
  caseNames: string[];
  onAdd: () => void;
  onRemove: (i: number) => void;
  onPatch: (i: number, fn: (c: Condition) => Condition) => void;
}) {
  return (
    <SectionCard
      title={`Conditions (${conditions.length})`}
      description="First matching condition wins. Each routes to a case by name."
      action={
        <GhostButton onClick={onAdd} disabled={caseNames.length === 0}>
          + Add condition
        </GhostButton>
      }
    >
      {conditions.length === 0 && (
        <p className="text-sm text-zinc-500 text-center py-4">
          No conditions. Without any, the route always serves the fallback case.
        </p>
      )}
      {conditions.map((c, i) => {
        const needsValue = c.op !== "exists" && c.op !== "not_exists";
        return (
          <div
            key={i}
            className="border border-zinc-800 rounded-lg p-3 bg-zinc-950/60"
          >
            <div className="grid grid-cols-2 md:grid-cols-12 gap-2 items-end">
              <div className="md:col-span-2">
                <FieldLabel>Source</FieldLabel>
                <Select
                  value={c.source || "body"}
                  onChange={(v) => onPatch(i, (c) => ({ ...c, source: v }))}
                  options={SOURCES}
                />
              </div>
              <div className="md:col-span-3">
                <FieldLabel>Field</FieldLabel>
                <TextInput
                  value={c.field}
                  onChange={(v) => onPatch(i, (c) => ({ ...c, field: v }))}
                  placeholder="payment.method"
                />
              </div>
              <div className="md:col-span-2">
                <FieldLabel>Op</FieldLabel>
                <Select
                  value={c.op || "eq"}
                  onChange={(v) => onPatch(i, (c) => ({ ...c, op: v }))}
                  options={OPS}
                />
              </div>
              <div className={`md:col-span-${needsValue ? "2" : "0"} ${needsValue ? "" : "hidden md:hidden"}`}>
                {needsValue && (
                  <>
                    <FieldLabel>Value</FieldLabel>
                    <TextInput
                      value={c.value}
                      onChange={(v) => onPatch(i, (c) => ({ ...c, value: v }))}
                      placeholder="card"
                    />
                  </>
                )}
              </div>
              <div className="md:col-span-2">
                <FieldLabel>Case</FieldLabel>
                <Select
                  value={c.case || ""}
                  onChange={(v) => onPatch(i, (c) => ({ ...c, case: v }))}
                  options={caseNames.map((n) => ({ value: n }))}
                  emptyLabel="(pick)"
                  error={!c.case || !caseNames.includes(c.case)}
                />
              </div>
              <div className="md:col-span-1 flex justify-end">
                <GhostButton onClick={() => onRemove(i)} danger title="Remove">
                  ×
                </GhostButton>
              </div>
            </div>
          </div>
        );
      })}
    </SectionCard>
  );
}
