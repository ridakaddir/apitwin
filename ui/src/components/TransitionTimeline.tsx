import type { Transition } from "../types/config";

export function TransitionTimeline({ transitions }: { transitions: Transition[] }) {
  return (
    <div className="flex items-center gap-1 overflow-x-auto">
      {transitions.map((t, i) => {
        const isTerminal = i === transitions.length - 1 && !t.duration;
        return (
          <div key={i} className="flex items-center gap-1">
            {i > 0 && (
              <svg className="w-4 h-4 text-zinc-600 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
              </svg>
            )}
            <div
              className={`text-xs font-mono px-2.5 py-1 rounded border shrink-0 ${
                isTerminal
                  ? "bg-emerald-500/10 text-emerald-400 border-emerald-500/25"
                  : "bg-zinc-800 text-zinc-300 border-zinc-700"
              }`}
            >
              {t.case}
              {t.duration > 0 && (
                <span className="text-zinc-500 ml-1.5">{t.duration}s</span>
              )}
              {isTerminal && (
                <span className="text-emerald-500/60 ml-1.5">final</span>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}
