import { useEffect, useState } from "react";
import type { Config } from "../types/config";

export async function fetchConfig(): Promise<Config> {
  const res = await fetch("/__api/routes");
  if (!res.ok) throw new Error(`Failed to fetch config: ${res.status}`);
  return res.json();
}

export function useConfig(pollInterval = 3000) {
  const [config, setConfig] = useState<Config | null>(null);
  const [error, setError] = useState<string | null>(null);

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
    const id = setInterval(load, pollInterval);
    return () => {
      active = false;
      clearInterval(id);
    };
  }, [pollInterval]);

  return { config, error };
}
