import type { Transition } from "../../types/config";
import { FieldLabel, GhostButton, SectionCard, Select, TextInput } from "./SectionCard";

export function TransitionsSection({
  transitions,
  caseNames,
  onAdd,
  onRemove,
  onPatch,
  onMove,
}: {
  transitions: Transition[];
  caseNames: string[];
  onAdd: () => void;
  onRemove: (i: number) => void;
  onPatch: (i: number, fn: (t: Transition) => Transition) => void;
  onMove: (from: number, to: number) => void;
}) {
  return (
    <SectionCard
      title={`Transitions (${transitions.length})`}
      description="Time-based response sequence after the route first fires. Set the last duration to 0 for a terminal state."
      action={
        <GhostButton onClick={onAdd} disabled={caseNames.length === 0}>
          + Add transition
        </GhostButton>
      }
    >
      {transitions.length === 0 && (
        <p className="text-sm text-zinc-500 text-center py-4">
          No transitions. The route serves its case-matched response without state changes.
        </p>
      )}
      {transitions.map((t, i) => (
        <div
          key={i}
          className="border border-zinc-800 rounded-lg p-3 bg-zinc-950/60"
        >
          <div className="grid grid-cols-1 md:grid-cols-12 gap-2 items-end">
            <div className="md:col-span-1 text-zinc-500 text-xs pb-2 md:pb-0 font-mono">
              #{i + 1}
            </div>
            <div className="md:col-span-5">
              <FieldLabel>Case</FieldLabel>
              <Select
                value={t.case || ""}
                onChange={(v) => onPatch(i, (t) => ({ ...t, case: v }))}
                options={caseNames.map((n) => ({ value: n }))}
                emptyLabel="(pick)"
                error={!t.case || !caseNames.includes(t.case)}
              />
            </div>
            <div className="md:col-span-3">
              <FieldLabel>Duration (s)</FieldLabel>
              <TextInput
                type="number"
                min={0}
                value={t.duration || 0}
                onChange={(v) =>
                  onPatch(i, (t) => ({ ...t, duration: Number(v) || 0 }))
                }
              />
              {i === transitions.length - 1 && (t.duration || 0) === 0 && (
                <p className="text-xs text-zinc-500 mt-1">terminal</p>
              )}
            </div>
            <div className="md:col-span-3 flex gap-1 justify-end">
              <GhostButton
                onClick={() => onMove(i, i - 1)}
                disabled={i === 0}
                title="Move up"
              >
                ↑
              </GhostButton>
              <GhostButton
                onClick={() => onMove(i, i + 1)}
                disabled={i === transitions.length - 1}
                title="Move down"
              >
                ↓
              </GhostButton>
              <GhostButton onClick={() => onRemove(i)} danger title="Remove">
                ×
              </GhostButton>
            </div>
          </div>
        </div>
      ))}
    </SectionCard>
  );
}
