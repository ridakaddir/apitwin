import type { Condition } from "../types/config";

export function ConditionBadge({ condition }: { condition: Condition }) {
  const label =
    condition.op === "exists" || condition.op === "not_exists"
      ? `${condition.source}.${condition.field} ${condition.op}`
      : `${condition.source}.${condition.field} ${condition.op} "${condition.value}"`;

  return (
    <span className="inline-flex items-center gap-1.5 text-xs bg-zinc-800 border border-zinc-700 rounded-full px-2.5 py-1 font-mono">
      <span className="text-zinc-300">{label}</span>
      <span className="text-zinc-500">&rarr;</span>
      <span className="text-violet-400">{condition.case}</span>
    </span>
  );
}
