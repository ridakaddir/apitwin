import { useState } from "react";
import { useConfig, useGRPCSchemas } from "./api/client";
import { RouteDashboard } from "./components/RouteDashboard";
import { TestPanel } from "./components/TestPanel";
import { RouteEditor } from "./components/editor/RouteEditor";
import type { GRPCRoute, Route, RouteKind, TestTarget } from "./types/config";

type View =
  | { kind: "list" }
  | { kind: "edit"; routeKind: RouteKind; route: Route | GRPCRoute }
  | { kind: "new"; routeKind: RouteKind };

export default function App() {
  const [view, setView] = useState<View>({ kind: "list" });
  const { config, error, reload } = useConfig(view.kind === "list" ? 3000 : 0);
  const grpcSchemas = useGRPCSchemas();
  const [testTarget, setTestTarget] = useState<TestTarget | null>(null);

  if (error) {
    return (
      <div className="min-h-screen bg-zinc-950 text-zinc-100 flex items-center justify-center">
        <div className="text-center">
          <p className="text-red-400 text-lg">Failed to load config</p>
          <p className="text-zinc-500 text-sm mt-1">{error}</p>
        </div>
      </div>
    );
  }

  if (!config) {
    return (
      <div className="min-h-screen bg-zinc-950 text-zinc-100 flex items-center justify-center">
        <p className="text-zinc-500">Loading...</p>
      </div>
    );
  }

  if (view.kind === "edit" || view.kind === "new") {
    return (
      <div className="min-h-screen bg-zinc-950 text-zinc-100">
        <RouteEditor
          mode={view.kind}
          kind={view.routeKind}
          initialRoute={view.kind === "edit" ? view.route : undefined}
          onClose={() => setView({ kind: "list" })}
          onSaved={() => {
            reload();
            setView({ kind: "list" });
          }}
        />
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100">
      <RouteDashboard
        config={config}
        onTest={(route, kind) => setTestTarget({ route, kind })}
        onEdit={(route, kind) => setView({ kind: "edit", routeKind: kind, route })}
        onNew={(kind) => setView({ kind: "new", routeKind: kind })}
      />
      <TestPanel target={testTarget} grpcSchemas={grpcSchemas} onClose={() => setTestTarget(null)} />
    </div>
  );
}
