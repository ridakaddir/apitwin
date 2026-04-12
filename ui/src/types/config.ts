export interface Config {
  routes: Route[];
  grpc_routes: GRPCRoute[];
}

export interface Route {
  method: string;
  match: string;
  enabled?: boolean;
  fallback: string;
  conditions: Condition[];
  cases: Record<string, Case>;
  transitions: Transition[];
}

export interface GRPCRoute {
  match: string;
  enabled?: boolean;
  fallback: string;
  conditions: Condition[];
  cases: Record<string, Case>;
  transitions: Transition[];
}

export interface Case {
  status: number;
  json: string;
  file: string;
  delay: number;
  persist: boolean;
  merge: string;
  key: string;
  array_key: string;
  defaults: string;
  primary?: CascadePrimary;
  cascade?: CascadeTarget[];
}

export interface CascadePrimary {
  file: string;
  merge: string;
  path?: string;
}

export interface CascadeTarget {
  pattern: string;
  merge: string;
  path?: string;
  transform?: string;
  condition?: string;
}

export interface Condition {
  source: string;
  field: string;
  op: string;
  value: string;
  case: string;
}

export interface Transition {
  case: string;
  duration: number;
}

export interface TestTarget {
  route: Route | GRPCRoute;
  kind: "http" | "grpc";
}

export interface GRPCMethodSchema {
  method: string;
  service: string;
  inputType: string;
  outputType: string;
  streaming: boolean;
  fields: GRPCFieldInfo[];
}

export interface GRPCFieldInfo {
  name: string;
  jsonName: string;
  kind: string;
  repeated: boolean;
  messageType?: string;
  enumType?: string;
}

export interface GRPCInvokeResult {
  code: string;
  message?: string;
  response?: unknown;
  latencyMs: number;
}
