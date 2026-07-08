import { ApiError } from "./errors";

const rawBaseUrl = import.meta.env.VITE_API_BASE_URL;
const API_BASE_URL = typeof rawBaseUrl === "string" && rawBaseUrl.length > 0 ? rawBaseUrl : "/api";

type HttpMethod = "GET" | "POST" | "PATCH";

function hasMessage(value: object): value is { message: unknown } {
  return "message" in value;
}

async function readErrorMessage(response: Response): Promise<string | undefined> {
  try {
    const json: unknown = await response.json();
    if (typeof json === "object" && json !== null && hasMessage(json)) {
      return typeof json.message === "string" ? json.message : undefined;
    }
    return undefined;
  } catch {
    return undefined;
  }
}

async function request(method: HttpMethod, path: string, body?: unknown): Promise<unknown> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}${path}`, {
      method,
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (cause) {
    throw new ApiError("Network request failed", { status: 0, cause });
  }

  if (!response.ok) {
    const message = await readErrorMessage(response);
    throw new ApiError(message ?? `Request failed with status ${response.status}`, {
      status: response.status,
    });
  }

  if (response.status === 204) {
    return undefined;
  }

  try {
    return await response.json();
  } catch (cause) {
    throw new ApiError("Failed to parse response as JSON", { status: response.status, cause });
  }
}

/** Fetch wrapper: GET, returning the parsed (still unvalidated) JSON body. */
export function httpGet(path: string): Promise<unknown> {
  return request("GET", path);
}

/** Fetch wrapper: POST with a JSON body, returning the parsed JSON body. */
export function httpPost(path: string, body: unknown): Promise<unknown> {
  return request("POST", path, body);
}

/** Fetch wrapper: PATCH with a JSON body, returning the parsed JSON body. */
export function httpPatch(path: string, body: unknown): Promise<unknown> {
  return request("PATCH", path, body);
}
