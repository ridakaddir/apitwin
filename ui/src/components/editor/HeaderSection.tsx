import type { GRPCRoute, Route, RouteKind } from "../../types/config";
import { ErrorText, FieldLabel, Select, SectionCard, TextInput } from "./SectionCard";

const HTTP_METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"];

export function HeaderSection({
  kind,
  route,
  caseNames,
  onMethod,
  onMatch,
  onEnabled,
  onFallback,
  errors,
}: {
  kind: RouteKind;
  route: Route | GRPCRoute;
  caseNames: string[];
  onMethod: (m: string) => void;
  onMatch: (m: string) => void;
  onEnabled: (b: boolean) => void;
  onFallback: (c: string) => void;
  errors: Record<string, string>;
}) {
  const enabled = route.enabled !== false;
  return (
    <SectionCard
      title="Route"
      description={
        kind === "grpc"
          ? "gRPC method path (e.g. /atlas.v1.Continents/Get). Cases reference proto field names."
          : "HTTP method and URL pattern. Use {name} for named params, ~ prefix for regex."
      }
    >
      <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
        {kind === "http" && (
          <div>
            <FieldLabel>Method</FieldLabel>
            <Select
              value={(route as Route).method ?? "GET"}
              onChange={onMethod}
              options={HTTP_METHODS.map((m) => ({ value: m }))}
            />
          </div>
        )}
        <div className={kind === "http" ? "md:col-span-2" : "md:col-span-3"}>
          <FieldLabel>Match</FieldLabel>
          <TextInput
            value={route.match}
            onChange={onMatch}
            placeholder={kind === "grpc" ? "/package.Service/Method" : "/api/resource/{id}"}
          />
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div>
          <FieldLabel>Fallback case</FieldLabel>
          <Select
            value={route.fallback ?? ""}
            onChange={onFallback}
            options={caseNames.map((n) => ({ value: n }))}
            emptyLabel="(none)"
            error={!!errors.fallback}
          />
          {errors.fallback && <ErrorText>{errors.fallback}</ErrorText>}
        </div>
        <div className="flex items-end">
          <label className="flex items-center gap-2 text-sm text-zinc-300 cursor-pointer">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => onEnabled(e.target.checked)}
              className="accent-emerald-500"
            />
            Enabled
          </label>
        </div>
      </div>
    </SectionCard>
  );
}
