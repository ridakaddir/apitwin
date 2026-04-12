import { useEffect } from "react";
import type { Route } from "../types/config";
import type { TestTarget, GRPCMethodSchema } from "../types/config";
import { RequestTester } from "./RequestTester";

export function TestPanel({
  target,
  grpcSchemas,
  onClose,
}: {
  target: TestTarget | null;
  grpcSchemas: GRPCMethodSchema[] | null;
  onClose: () => void;
}) {
  // Close on Escape
  useEffect(() => {
    if (!target) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [target, onClose]);

  const method =
    target?.kind === "http" ? (target.route as Route).method : "gRPC";

  return (
    <>
      {/* Backdrop */}
      <div
        className={`fixed inset-0 bg-black/40 z-40 transition-opacity duration-200 ${
          target ? "opacity-100" : "opacity-0 pointer-events-none"
        }`}
        onClick={onClose}
      />

      {/* Panel */}
      <div
        className={`fixed top-0 right-0 h-full w-[520px] max-w-[90vw] bg-zinc-900 border-l border-zinc-800 z-50 flex flex-col transition-transform duration-200 ease-out ${
          target ? "translate-x-0" : "translate-x-full"
        }`}
      >
        {/* Header */}
        <div className="flex items-center gap-3 px-5 py-4 border-b border-zinc-800 shrink-0">
          <span className="text-xs font-mono font-semibold px-2 py-0.5 rounded border bg-zinc-800 text-zinc-300 border-zinc-700">
            {method?.toUpperCase()}
          </span>
          <span className="font-mono text-sm text-zinc-200 truncate flex-1">
            {target?.route.match}
          </span>
          <button
            onClick={onClose}
            className="text-zinc-500 hover:text-zinc-300 transition-colors cursor-pointer p-1"
          >
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto px-5 py-4">
          {target && (
            <RequestTester
              route={target.route}
              kind={target.kind}
              grpcSchemas={grpcSchemas}
            />
          )}
        </div>
      </div>
    </>
  );
}
