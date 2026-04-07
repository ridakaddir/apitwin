import { useState } from "react";
import type { Config, Route, GRPCRoute } from "../types/config";
import { RouteCard } from "./RouteCard";

export function RouteDashboard({
  config,
  onTest,
}: {
  config: Config;
  onTest: (route: Route | GRPCRoute, kind: "http" | "grpc") => void;
}) {
  const [search, setSearch] = useState("");

  const httpRoutes = config.routes ?? [];
  const grpcRoutes = config.grpc_routes ?? [];

  const filteredHttp = httpRoutes.filter(
    (r) =>
      r.match.toLowerCase().includes(search.toLowerCase()) ||
      r.method.toLowerCase().includes(search.toLowerCase()),
  );
  const filteredGrpc = grpcRoutes.filter((r) =>
    r.match.toLowerCase().includes(search.toLowerCase()),
  );

  return (
    <div className="max-w-5xl mx-auto px-6 py-8">
      <header className="mb-8">
        <h1 className="text-2xl font-bold tracking-tight">apitwin</h1>
        <div className="flex items-center gap-4 mt-3 text-sm text-zinc-400">
          <span>
            {httpRoutes.length} HTTP route{httpRoutes.length !== 1 && "s"}
          </span>
          {grpcRoutes.length > 0 && (
            <span>
              {grpcRoutes.length} gRPC route{grpcRoutes.length !== 1 && "s"}
            </span>
          )}
        </div>
        <input
          type="text"
          placeholder="Filter routes..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="mt-4 w-full max-w-sm bg-zinc-900 border border-zinc-800 rounded-lg px-3 py-2 text-sm text-zinc-100 placeholder-zinc-600 focus:outline-none focus:border-zinc-600"
        />
      </header>

      {filteredHttp.length > 0 && (
        <section>
          <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500 mb-3">
            HTTP Routes
          </h2>
          <div className="space-y-2">
            {filteredHttp.map((route, i) => (
              <RouteCard key={`http-${i}`} route={route} kind="http" onTest={onTest} />
            ))}
          </div>
        </section>
      )}

      {filteredGrpc.length > 0 && (
        <section className="mt-8">
          <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500 mb-3">
            gRPC Routes
          </h2>
          <div className="space-y-2">
            {filteredGrpc.map((route, i) => (
              <RouteCard key={`grpc-${i}`} route={route} kind="grpc" onTest={onTest} />
            ))}
          </div>
        </section>
      )}

      {filteredHttp.length === 0 && filteredGrpc.length === 0 && (
        <p className="text-zinc-600 text-center mt-12">No routes match your filter.</p>
      )}
    </div>
  );
}
