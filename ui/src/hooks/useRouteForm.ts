import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type {
  Case,
  Condition,
  GRPCRoute,
  Route,
  RouteKind,
  Transition,
  ValidationError,
} from "../types/config";
import { previewRoute, validateRoute } from "../api/configApi";

export type EditableRoute = Route | GRPCRoute;

export function emptyCase(): Case {
  return {
    status: 200,
    json: "",
    file: "",
    delay: 0,
    persist: false,
    merge: "",
    key: "",
    array_key: "",
    defaults: "",
    wrap: "",
    source: "",
  };
}

export function emptyHTTPRoute(): Route {
  return {
    method: "GET",
    match: "/",
    enabled: true,
    fallback: "success",
    conditions: [],
    cases: { success: { ...emptyCase(), json: '{"ok":true}' } },
    transitions: [],
  };
}

export function emptyGRPCRoute(): GRPCRoute {
  return {
    match: "/package.Service/Method",
    enabled: true,
    fallback: "success",
    conditions: [],
    cases: { success: { ...emptyCase(), json: "{}" } },
    transitions: [],
  };
}

// normalizeRoute fills in missing optional collections so the editor's
// patch helpers can safely spread them. The Go API now omits empty arrays
// from JSON (omitempty), so we get e.g. `{method, match, cases}` with no
// `conditions` or `transitions` field — without this normalization,
// `[...r.conditions, x]` would throw and crash the whole form.
export function normalizeRoute<R extends EditableRoute>(r: R): R {
  return {
    ...r,
    conditions: r.conditions ?? [],
    transitions: r.transitions ?? [],
    cases: r.cases ?? {},
  } as R;
}

export interface UseRouteForm {
  kind: RouteKind;
  route: EditableRoute;
  dirty: boolean;
  errors: Record<string, string>;
  previewText: string;
  previewFormat: "toml" | "yaml" | "json";
  setPreviewFormat: (f: "toml" | "yaml" | "json") => void;
  grpcSchemaAvailable: boolean | undefined;

  setRoute: (next: EditableRoute) => void;
  patchRoute: (fn: (r: EditableRoute) => EditableRoute) => void;

  setMethod: (m: string) => void;
  setMatch: (m: string) => void;
  setEnabled: (b: boolean) => void;
  setFallback: (c: string) => void;

  addCase: (name: string) => void;
  removeCase: (name: string) => void;
  renameCase: (oldName: string, newName: string) => void;
  patchCase: (name: string, fn: (c: Case) => Case) => void;

  addCondition: () => void;
  removeCondition: (i: number) => void;
  patchCondition: (i: number, fn: (c: Condition) => Condition) => void;

  addTransition: () => void;
  removeTransition: (i: number) => void;
  patchTransition: (i: number, fn: (t: Transition) => Transition) => void;
  moveTransition: (from: number, to: number) => void;

  resetTo: (kind: RouteKind, route: EditableRoute) => void;
}

export function useRouteForm(
  initialKind: RouteKind,
  initialRoute: EditableRoute,
): UseRouteForm {
  const seedRoute = normalizeRoute(initialRoute);
  const [kind, setKind] = useState<RouteKind>(initialKind);
  const [route, setRouteState] = useState<EditableRoute>(seedRoute);
  const [initial, setInitial] = useState<{ kind: RouteKind; json: string }>({
    kind: initialKind,
    json: JSON.stringify(seedRoute),
  });

  const [errors, setErrors] = useState<Record<string, string>>({});
  const [previewText, setPreviewText] = useState("");
  const [previewFormat, setPreviewFormat] = useState<"toml" | "yaml" | "json">("toml");
  const [grpcSchemaAvailable, setGRPCSchemaAvailable] = useState<boolean | undefined>(undefined);

  const dirty = useMemo(
    () => JSON.stringify(route) !== initial.json || kind !== initial.kind,
    [route, kind, initial],
  );

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      try {
        const [preview, validation] = await Promise.all([
          previewRoute(kind, route, previewFormat).catch(() => ""),
          validateRoute(kind, route).catch(() => null),
        ]);
        setPreviewText(preview);
        if (validation) {
          const map: Record<string, string> = {};
          for (const e of validation.errors as ValidationError[]) {
            map[e.path || "_"] = e.reason;
          }
          setErrors(map);
          setGRPCSchemaAvailable(validation.grpcSchemaAvailable);
        }
      } catch {
        // network errors are non-fatal here; the save endpoint re-validates
      }
    }, 300);
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [kind, route, previewFormat]);

  const setRoute = useCallback((next: EditableRoute) => setRouteState(next), []);
  const patchRoute = useCallback(
    (fn: (r: EditableRoute) => EditableRoute) => setRouteState((r) => fn(r)),
    [],
  );

  const setMethod = useCallback(
    (m: string) => setRouteState((r) => ({ ...(r as Route), method: m })),
    [],
  );
  const setMatch = useCallback(
    (m: string) => setRouteState((r) => ({ ...r, match: m })),
    [],
  );
  const setEnabled = useCallback(
    (b: boolean) => setRouteState((r) => ({ ...r, enabled: b })),
    [],
  );
  const setFallback = useCallback(
    (c: string) => setRouteState((r) => ({ ...r, fallback: c })),
    [],
  );

  const addCase = useCallback(
    (name: string) =>
      setRouteState((r) => ({
        ...r,
        cases: { ...r.cases, [name]: emptyCase() },
      })),
    [],
  );
  const removeCase = useCallback(
    (name: string) =>
      setRouteState((r) => {
        const cases = { ...r.cases };
        delete cases[name];
        return { ...r, cases };
      }),
    [],
  );
  const renameCase = useCallback((oldName: string, newName: string) => {
    if (oldName === newName) return;
    setRouteState((r) => {
      const cases: Record<string, Case> = {};
      for (const [k, v] of Object.entries(r.cases)) {
        cases[k === oldName ? newName : k] = v;
      }
      const conditions = r.conditions.map((c) =>
        c.case === oldName ? { ...c, case: newName } : c,
      );
      const transitions = r.transitions.map((t) =>
        t.case === oldName ? { ...t, case: newName } : t,
      );
      const fallback = r.fallback === oldName ? newName : r.fallback;
      return { ...r, cases, conditions, transitions, fallback };
    });
  }, []);
  const patchCase = useCallback(
    (name: string, fn: (c: Case) => Case) =>
      setRouteState((r) => ({
        ...r,
        cases: { ...r.cases, [name]: fn(r.cases[name] ?? emptyCase()) },
      })),
    [],
  );

  const addCondition = useCallback(
    () =>
      setRouteState((r) => ({
        ...r,
        conditions: [
          ...r.conditions,
          { source: "body", field: "", op: "eq", value: "", case: "" },
        ],
      })),
    [],
  );
  const removeCondition = useCallback(
    (i: number) =>
      setRouteState((r) => ({
        ...r,
        conditions: r.conditions.filter((_, idx) => idx !== i),
      })),
    [],
  );
  const patchCondition = useCallback(
    (i: number, fn: (c: Condition) => Condition) =>
      setRouteState((r) => ({
        ...r,
        conditions: r.conditions.map((c, idx) => (idx === i ? fn(c) : c)),
      })),
    [],
  );

  const addTransition = useCallback(
    () =>
      setRouteState((r) => ({
        ...r,
        transitions: [...r.transitions, { case: "", duration: 0 }],
      })),
    [],
  );
  const removeTransition = useCallback(
    (i: number) =>
      setRouteState((r) => ({
        ...r,
        transitions: r.transitions.filter((_, idx) => idx !== i),
      })),
    [],
  );
  const patchTransition = useCallback(
    (i: number, fn: (t: Transition) => Transition) =>
      setRouteState((r) => ({
        ...r,
        transitions: r.transitions.map((t, idx) => (idx === i ? fn(t) : t)),
      })),
    [],
  );
  const moveTransition = useCallback((from: number, to: number) => {
    setRouteState((r) => {
      const next = [...r.transitions];
      const [item] = next.splice(from, 1);
      next.splice(to, 0, item);
      return { ...r, transitions: next };
    });
  }, []);

  const resetTo = useCallback((k: RouteKind, r: EditableRoute) => {
    const normalized = normalizeRoute(r);
    setKind(k);
    setRouteState(normalized);
    setInitial({ kind: k, json: JSON.stringify(normalized) });
    setErrors({});
  }, []);

  return {
    kind,
    route,
    dirty,
    errors,
    previewText,
    previewFormat,
    setPreviewFormat,
    grpcSchemaAvailable,
    setRoute,
    patchRoute,
    setMethod,
    setMatch,
    setEnabled,
    setFallback,
    addCase,
    removeCase,
    renameCase,
    patchCase,
    addCondition,
    removeCondition,
    patchCondition,
    addTransition,
    removeTransition,
    patchTransition,
    moveTransition,
    resetTo,
  };
}
