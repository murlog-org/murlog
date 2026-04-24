// JSON-RPC 2.0 client for /api/mur/v1/rpc.
// /api/mur/v1/rpc 用の JSON-RPC 2.0 クライアント。

const ENDPOINT = "/api/mur/v1/rpc";

let nextId = 1;

export type RpcError = {
  code: number;
  message: string;
  data?: unknown;
};

type RpcResponse<T> = {
  jsonrpc: "2.0";
  result?: T;
  error?: RpcError;
  id: number;
};

// call sends a JSON-RPC 2.0 request and returns the result.
// Throws on network error. Returns { result, error } on RPC-level errors.
export async function call<T>(
  method: string,
  params?: Record<string, unknown>
): Promise<{ result?: T; error?: RpcError }> {
  const id = nextId++;
  const body = {
    jsonrpc: "2.0" as const,
    method,
    params: params ?? {},
    id,
  };

  const res = await fetch(ENDPOINT, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: JSON.stringify(body),
  });

  // サーバーが HTML (500 等) を返した場合に res.json() が throw するのを防ぐ。
  // Guard against non-JSON responses (e.g. 500 HTML pages).
  let data: RpcResponse<T>;
  try {
    data = await res.json();
  } catch {
    return { error: { code: -32603, message: `server error (HTTP ${res.status})` } };
  }

  if (data.error) {
    return { error: data.error };
  }
  return { result: data.result };
}

// callBatch sends multiple JSON-RPC 2.0 requests in a single HTTP request.
// 複数の JSON-RPC 2.0 リクエストを1回の HTTP リクエストで送信する。
type BatchRequest = { method: string; params?: Record<string, unknown> };
type BatchResultEntry<T = unknown> = { result?: T; error?: RpcError };

export async function callBatch<T extends unknown[]>(
  requests: { [K in keyof T]: BatchRequest }
): Promise<{ [K in keyof T]: BatchResultEntry<T[K]> }> {
  const bodies = (requests as BatchRequest[]).map((r) => ({
    jsonrpc: "2.0" as const,
    method: r.method,
    params: r.params ?? {},
    id: nextId++,
  }));

  const res = await fetch(ENDPOINT, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: JSON.stringify(bodies),
  });

  let data: RpcResponse<unknown>[];
  try {
    data = await res.json();
  } catch {
    return bodies.map(() => ({
      error: { code: -32603, message: `server error (HTTP ${res.status})` },
    })) as { [K in keyof T]: BatchResultEntry<T[K]> };
  }

  // Map responses back by id. / id でレスポンスをマッピング。
  const byId = new Map<number, RpcResponse<unknown>>();
  for (const d of data) byId.set(d.id, d);

  return bodies.map((b) => {
    const d = byId.get(b.id);
    if (!d) return { error: { code: -32603, message: "missing response" } };
    if (d.error) return { error: d.error };
    return { result: d.result };
  }) as { [K in keyof T]: BatchResultEntry<T[K]> };
}

// callOrThrow sends a JSON-RPC request and throws on error.
// Use with useAsyncLoad for automatic error/retry handling.
// エラー時に throw する JSON-RPC リクエスト。useAsyncLoad と組み合わせて使う。
export async function callOrThrow<T>(
  method: string,
  params?: Record<string, unknown>
): Promise<T> {
  const { result, error } = await call<T>(method, params);
  if (error) {
    if (error.code === ERR_UNAUTHORIZED) {
      location.replace("/my/login");
    }
    throw new ApiError(error.message, error.code);
  }
  return result as T;
}

// ApiError wraps an RPC error with code for programmatic handling.
// プログラム的な処理のために code を持つ RPC エラーラッパー。
export class ApiError extends Error {
  code: number;
  constructor(message: string, code: number) {
    super(message);
    this.code = code;
  }
}

// Error code constants (match server-side).
export const ERR_UNAUTHORIZED = -32000;
export const ERR_NOT_FOUND = -32001;
export const ERR_CONFLICT = -32002;

// isUnauthorized checks if an RPC error is an auth failure.
export function isUnauthorized(err?: RpcError): boolean {
  return err?.code === ERR_UNAUTHORIZED;
}

// redirectIfUnauthorized checks for auth error and redirects to login if so.
// Returns true if redirected (caller should return early).
// 認証エラーをチェックし、ログインページにリダイレクトする。リダイレクト時は true を返す。
export function redirectIfUnauthorized(err?: RpcError): boolean {
  if (err?.code === ERR_UNAUTHORIZED) {
    location.replace("/my/login");
    return true;
  }
  return false;
}

// AttachmentMeta is the metadata returned after a media upload.
// メディアアップロード後に返却されるメタデータ。
export type AttachmentMeta = {
  id: string;
  url: string;
  path: string;
  mime_type: string;
  alt: string;
  width: number;
  height: number;
};

// uploadMedia uploads a file to the media endpoint and returns attachment metadata.
// ファイルをメディアエンドポイントにアップロードし、添付メタデータを返す。
export async function uploadMedia(
  file: File,
  alt?: string,
  prefix?: string
): Promise<{ result?: AttachmentMeta; error?: string }> {
  const form = new FormData();
  form.append("file", file);
  if (alt) form.append("alt", alt);
  if (prefix) form.append("prefix", prefix);
  const res = await fetch("/api/mur/v1/media", {
    method: "POST",
    credentials: "include",
    body: form,
  });
  if (res.status === 401) return { error: "unauthorized" };
  if (!res.ok) {
    const text = await res.text();
    return { error: text || "upload failed" };
  }
  return { result: await res.json() };
}
