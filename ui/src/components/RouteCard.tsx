import { useState } from "react";
import type { Route, GRPCRoute } from "../types/config";
import { CaseDetail } from "./CaseDetail";
import { ConditionBadge } from "./ConditionBadge";
import { TransitionTimeline } from "./TransitionTimeline";

const METHOD_COLORS: Record<string, string> = {
  GET: "bg-emerald-500/15 text-emerald-400 border-emerald-500/25",
  POST: "bg-blue-500/15 text-blue-400 border-blue-500/25",
  PUT: "bg-amber-500/15 text-amber-400 border-amber-500/25",
  PATCH: "bg-orange-500/15 text-orange-400 border-orange-500/25",
  DELETE: "bg-red-500/15 text-red-400 border-red-500/25",
  HEAD: "bg-purple-500/15 text-purple-400 border-purple-500/25",
  OPTIONS: "bg-zinc-500/15 text-zinc-400 border-zinc-500/25",
};

export function RouteCard({
  route,
  kind,
  onTest,
}: {
  route: Route | GRPCRoute;
  kind: "http" | "grpc";
  onTest: (route: Route | GRPCRoute, kind: "http" | "grpc") => void;
}) {
  const [expanded, setExpanded] = useState(false);

  const method = kind === "http" ? (route as Route).method : "gRPC";
  const enabled = route.enabled !== false;
  const cases = route.cases ?? {};
  const conditions = route.conditions ?? [];
  const transitions = route.transitions ?? [];
  const caseCount = Object.keys(cases).length;
  const methodColor =
    METHOD_COLORS[method.toUpperCase()] ?? "bg-zinc-500/15 text-zinc-400 border-zinc-500/25";

  return (
    <div
      className={`border border-zinc-800 rounded-lg transition-colors ${!enabled ? "opacity-50" : ""}`}
    >
      <div className="flex items-center">
        <button
          onClick={() => setExpanded(!expanded)}
          className="flex-1 flex items-center gap-3 px-4 py-3 text-left hover:bg-zinc-900/50 rounded-l-lg cursor-pointer"
        >
          <span
            className={`text-xs font-mono font-semibold px-2 py-0.5 rounded border ${methodColor}`}
          >
            {method.toUpperCase()}
          </span>
          <span className="font-mono text-sm text-zinc-200 flex-1">{route.match}</span>
          <span className="text-xs text-zinc-500">
            {caseCount} case{caseCount !== 1 && "s"}
            {conditions.length > 0 && ` \u00b7 ${conditions.length} cond`}
            {transitions.length > 0 && ` \u00b7 ${transitions.length} trans`}
          </span>
          <svg
            className={`w-4 h-4 text-zinc-500 transition-transform ${expanded ? "rotate-180" : ""}`}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
          </svg>
        </button>
        <button
          onClick={(e) => {
            e.stopPropagation();
            onTest(route, kind);
          }}
          className="px-3 py-3 text-zinc-500 hover:text-blue-400 transition-colors cursor-pointer border-l border-zinc-800"
          title={kind === "grpc" ? "Invoke this method" : "Test this route"}
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 3l14 9-14 9V3z" />
          </svg>
        </button>
      </div>

      {expanded && (
        <div className="px-4 pb-4 space-y-4">
          {route.fallback && (
            <div className="text-xs text-zinc-500">
              Fallback: <span className="text-zinc-300">{route.fallback}</span>
            </div>
          )}

          {conditions.length > 0 && (
            <div>
              <h4 className="text-xs font-semibold uppercase tracking-wider text-zinc-500 mb-2">
                Conditions
              </h4>
              <div className="flex flex-wrap gap-1.5">
                {conditions.map((c, i) => (
                  <ConditionBadge key={i} condition={c} />
                ))}
              </div>
            </div>
          )}

          {transitions.length > 0 && (
            <div>
              <h4 className="text-xs font-semibold uppercase tracking-wider text-zinc-500 mb-2">
                Transitions
              </h4>
              <TransitionTimeline transitions={transitions} />
            </div>
          )}

          {caseCount > 0 && (
            <div>
              <h4 className="text-xs font-semibold uppercase tracking-wider text-zinc-500 mb-2">
                Cases
              </h4>
              <div className="space-y-2">
                {Object.entries(cases).map(([name, c]) => (
                  <CaseDetail key={name} name={name} caseData={c} />
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
