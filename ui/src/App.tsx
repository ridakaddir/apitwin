import { useState } from "react";
import { useConfig } from "./api/client";
import { RouteDashboard } from "./components/RouteDashboard";
import { TestPanel } from "./components/TestPanel";
import type { Route, GRPCRoute } from "./types/config";

interface TestTarget {
  route: Route | GRPCRoute;
  kind: "http" | "grpc";
}

export default function App() {
  const { config, error } = useConfig();
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

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100">
      <RouteDashboard config={config} onTest={(route, kind) => setTestTarget({ route, kind })} />
      <TestPanel target={testTarget} onClose={() => setTestTarget(null)} />
    </div>
  );
}
