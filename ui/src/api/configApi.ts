import type {
  ConfigFileInfo,
  GRPCRoute,
  Route,
  RouteKind,
  SaveRouteRequest,
  SaveRouteResponse,
  ValidateResponse,
} from "../types/config";

export async function fetchConfigFiles(filter?: {
  kind: RouteKind;
  match: string;
  method?: string;
}): Promise<ConfigFileInfo[]> {
  const url = new URL("/__api/config/files", window.location.origin);
  if (filter) {
    url.searchParams.set("kind", filter.kind);
    url.searchParams.set("match", filter.match);
    if (filter.method) url.searchParams.set("method", filter.method);
  }
  const res = await fetch(url.pathname + url.search);
  if (!res.ok) throw new Error(`fetchConfigFiles: ${res.status}`);
  return res.json();
}

export async function previewRoute(
  kind: RouteKind,
  route: Route | GRPCRoute,
  format: "toml" | "yaml" | "json",
): Promise<string> {
  const res = await fetch("/__api/config/preview", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ kind, route, format }),
  });
  const body = await res.json();
  if (!res.ok) throw new Error(body?.error ?? `preview: ${res.status}`);
  return body.text as string;
}

export async function validateRoute(
  kind: RouteKind,
  route: Route | GRPCRoute,
): Promise<ValidateResponse> {
  const res = await fetch("/__api/config/validate", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ kind, route }),
  });
  const body = await res.json();
  if (!res.ok) throw new Error(body?.error ?? `validate: ${res.status}`);
  return body as ValidateResponse;
}

export async function saveRoute(req: SaveRouteRequest): Promise<SaveRouteResponse> {
  const res = await fetch("/__api/config/routes", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  const body = await res.json();
  if (!res.ok) {
    const message = body?.error ?? `save: ${res.status}`;
    const err = new Error(message) as Error & { status?: number };
    err.status = res.status;
    throw err;
  }
  return body as SaveRouteResponse;
}

export async function deleteRoute(
  file: string,
  kind: RouteKind,
  match: string,
  method?: string,
): Promise<void> {
  const res = await fetch("/__api/config/routes", {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ file, kind, match, method }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body?.error ?? `delete: ${res.status}`);
  }
}
