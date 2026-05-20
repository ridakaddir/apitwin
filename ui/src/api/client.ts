import { useEffect, useState } from "react";
import type { Config, GRPCMethodSchema, GRPCInvokeResult } from "../types/config";

export async function fetchConfig(): Promise<Config> {
  const res = await fetch("/__api/routes");
  if (!res.ok) throw new Error(`Failed to fetch config: ${res.status}`);
  return res.json();
}

// fetchGRPCSchemas returns null when apitwin is running without
// --grpc-proto (the endpoint responds 503). The UI uses that signal to
// hide the schema reference panel and fall back to an empty-JSON tester.
export async function fetchGRPCSchemas(): Promise<GRPCMethodSchema[] | null> {
  const res = await fetch("/__api/grpc/methods");
  if (res.status === 503) return null;
  if (!res.ok) throw new Error(`Failed to fetch gRPC schemas: ${res.status}`);
  return res.json();
}

export async function invokeGRPC(
  method: string,
  message: unknown,
  metadata?: Record<string, string>,
): Promise<GRPCInvokeResult> {
  const res = await fetch("/__api/grpc/invoke", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ method, message, metadata }),
  });
  const body = await res.json();
  if (!res.ok) {
    throw new Error(body?.error ?? `invoke failed: ${res.status}`);
  }
  return body as GRPCInvokeResult;
}

export function useGRPCSchemas() {
  const [schemas, setSchemas] = useState<GRPCMethodSchema[] | null>(null);
  useEffect(() => {
    let active = true;
    fetchGRPCSchemas()
      .then((s) => active && setSchemas(s))
      .catch(() => active && setSchemas(null));
    return () => {
      active = false;
    };
  }, []);
  return schemas;
}

export function useConfig(pollInterval = 3000) {
  const [config, setConfig] = useState<Config | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let active = true;

    const load = async () => {
      try {
        const cfg = await fetchConfig();
        if (active) {
          setConfig(cfg);
          setError(null);
        }
      } catch (err) {
        if (active) setError(String(err));
      }
    };

    load();
    if (pollInterval > 0) {
      const id = setInterval(load, pollInterval);
      return () => {
        active = false;
        clearInterval(id);
      };
    }
    return () => {
      active = false;
    };
  }, [pollInterval, reloadKey]);

  return { config, error, reload: () => setReloadKey((k) => k + 1) };
}
