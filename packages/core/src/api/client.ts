import { CliError } from "../errors.js";
import type { paths } from "./generated/types.js";

type LoginRequest =
  paths["/api/auth/login"]["post"]["requestBody"]["content"]["application/json"];
type RefreshRequest =
  paths["/api/auth/refresh"]["post"]["requestBody"]["content"]["application/json"];

export interface Session {
  accessToken: string;
  refreshToken: string;
  expiresAt: number;
}

export function isSession(value: unknown): value is Session {
  const data = record(value);
  return Boolean(
    data &&
      nonempty(data.accessToken) &&
      nonempty(data.refreshToken) &&
      typeof data.expiresAt === "number" &&
      Number.isFinite(data.expiresAt),
  );
}

function record(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function nonempty(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}

export function apiOrigin(value?: string): string {
  try {
    const url = new URL(value ?? "");
    const loopback = ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname);
    if (
      !url.username &&
      !url.password &&
      url.pathname === "/" &&
      !url.search &&
      !url.hash &&
      (url.protocol === "https:" || (url.protocol === "http:" && loopback))
    ) {
      return url.origin;
    }
  } catch {
    // Report configuration errors without echoing the supplied URL.
  }
  throw new CliError(
    "INVALID_ARGUMENT",
    "Set --api-url or SA2_API_URL to an HTTPS origin or loopback HTTP origin.",
    2,
  );
}

export class ApiError extends CliError {
  constructor(
    readonly status: number,
    readonly expired: boolean,
  ) {
    super(
      status === 401 || status === 403
        ? "AUTH_REJECTED"
        : status === 400
          ? "INVALID_ARGUMENT"
          : "API_UNAVAILABLE",
      status === 401 || status === 403
        ? "SA2 rejected authentication."
        : status === 400
          ? "SA2 rejected the supplied parameters."
          : "The SA2 API could not complete the request.",
      status === 401 || status === 403 ? 3 : status === 400 ? 2 : 1,
    );
  }
}

function invalidResponse(): CliError {
  return new CliError(
    "INVALID_API_RESPONSE",
    "SA2 returned an invalid authentication response.",
  );
}

function identity(value: unknown) {
  const user = record(value);
  if (!user || !nonempty(user.id)) throw invalidResponse();
  const roles = Array.isArray(user.roles) ? user.roles : [];
  return {
    id: user.id,
    email: typeof user.email === "string" ? user.email : null,
    roles: roles.map((role: unknown) => record(role)?.enName).filter(nonempty),
  };
}

export class AuthClient {
  constructor(private readonly origin: string) {}

  async login(body: LoginRequest) {
    return this.exchange("/api/auth/login", body);
  }

  async refresh(body: RefreshRequest) {
    return this.exchange("/api/auth/refresh", body);
  }

  async whoami(accessToken: string) {
    return identity(
      await this.request("/api/users/me", {
        headers: { Authorization: `Bearer ${accessToken}` },
      }),
    );
  }

  private async exchange(path: string, input: LoginRequest | RefreshRequest) {
    const value = record(
      await this.request(path, { method: "POST", body: JSON.stringify(input) }),
    );
    const expiresIn = value?.expiresIn;
    if (
      typeof expiresIn !== "number" ||
      !Number.isFinite(expiresIn) ||
      expiresIn <= 0
    ) {
      throw invalidResponse();
    }
    const session = {
      accessToken: value?.accessToken,
      refreshToken: value?.refreshToken,
      expiresAt: Date.now() + expiresIn * 1000,
    };
    if (!isSession(session)) throw invalidResponse();
    return { session, user: identity(value?.user) };
  }

  private async request(path: string, init: RequestInit): Promise<unknown> {
    let response: Response;
    let value: unknown;
    try {
      response = await fetch(new URL(path, this.origin), {
        ...init,
        cache: "no-store",
        redirect: "error",
        signal: AbortSignal.timeout(10_000),
        headers: {
          Accept: "application/json",
          ...(init.body ? { "Content-Type": "application/json" } : {}),
          ...init.headers,
        },
      });
      const body = await response.text();
      try {
        value = JSON.parse(body);
      } catch {
        value = undefined;
      }
    } catch {
      throw new CliError("API_UNAVAILABLE", "Could not reach the SA2 API.");
    }
    if (!response.ok) {
      throw new ApiError(
        response.status,
        record(value)?.reason === "EXPIRED_TOKEN",
      );
    }
    return value;
  }
}
