/**
 * Edge configuration — read + validate env once at process start.
 *
 * Hand-rolled (no zod) to keep the seed dependency-light. If your real service
 * grows a large config surface, swapping in zod is a drop-in change: replace
 * `loadConfig` with a schema `.parse(process.env)` and keep the exported shape.
 *
 * Import `config` everywhere; it is parsed once (module singleton). Call
 * `loadConfig(env)` directly only in tests, to validate a synthetic env.
 */

export interface Config {
  /** Base URL of the platform Gateway this edge talks to over gRPC/Connect. */
  readonly GATEWAY_URL: string;
  /** Port the thin HTTP/BFF surface listens on. */
  readonly PORT: number;
  /**
   * Static bearer token for the service-to-gateway hop (ADR-0003). Optional:
   * unset in local dev where the Gateway allows anonymous. TODO: replace with
   * JWT/cookie resolved at the edge per ADR-0003 (Open section).
   */
  readonly AUTH_BEARER_TOKEN?: string;
  /** OTLP endpoint for trace export (ADR-0004). Optional; unset = no export. */
  readonly OTEL_EXPORTER_OTLP_ENDPOINT?: string;
  /** Logical service name stamped on emitted spans (ADR-0004). */
  readonly OTEL_SERVICE_NAME: string;
}

/** Raised when the environment fails validation; message lists every problem. */
export class ConfigError extends Error {
  constructor(problems: string[]) {
    super(`invalid configuration:\n  - ${problems.join("\n  - ")}`);
    this.name = "ConfigError";
  }
}

type Env = Record<string, string | undefined>;

/**
 * Validate a raw environment into a typed, frozen Config. Pure — no I/O, no
 * process.env read — so it is unit-testable (see tests). Collects ALL problems
 * before throwing, rather than failing on the first.
 */
export function loadConfig(env: Env): Config {
  const problems: string[] = [];

  const GATEWAY_URL = env.GATEWAY_URL?.trim() || "http://localhost:8080";
  if (!isHttpUrl(GATEWAY_URL)) {
    problems.push(`GATEWAY_URL must be an http(s) URL, got "${GATEWAY_URL}"`);
  }

  const PORT = parsePort(env.PORT, problems);

  const OTEL_ENDPOINT = env.OTEL_EXPORTER_OTLP_ENDPOINT?.trim() || undefined;
  if (OTEL_ENDPOINT !== undefined && !isHttpUrl(OTEL_ENDPOINT)) {
    problems.push(
      `OTEL_EXPORTER_OTLP_ENDPOINT must be an http(s) URL when set, got "${OTEL_ENDPOINT}"`,
    );
  }

  if (problems.length > 0) {
    throw new ConfigError(problems);
  }

  return Object.freeze({
    GATEWAY_URL,
    PORT,
    AUTH_BEARER_TOKEN: env.AUTH_BEARER_TOKEN?.trim() || undefined,
    OTEL_EXPORTER_OTLP_ENDPOINT: OTEL_ENDPOINT,
    OTEL_SERVICE_NAME: env.OTEL_SERVICE_NAME?.trim() || "team-frontend-edge",
  });
}

function parsePort(raw: string | undefined, problems: string[]): number {
  if (raw === undefined || raw.trim() === "") return 3000;
  const n = Number(raw);
  if (!Number.isInteger(n) || n < 1 || n > 65535) {
    problems.push(`PORT must be an integer in 1..65535, got "${raw}"`);
    return 3000;
  }
  return n;
}

function isHttpUrl(value: string): boolean {
  try {
    const u = new URL(value);
    return u.protocol === "http:" || u.protocol === "https:";
  } catch {
    return false;
  }
}

/** Process-wide config, parsed once from the real environment. */
export const config: Config = loadConfig(process.env);
