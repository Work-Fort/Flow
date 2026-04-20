// SPDX-License-Identifier: GPL-2.0-only
// Auth is handled by Scope's reverse proxy via session cookie — no explicit Bearer header needed.

const BASE = ''; // relative — same-origin via Scope's reverse proxy

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const res = await fetch(BASE + path, {
    method,
    headers,
    credentials: 'same-origin',
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    throw new FlowAPIError(res.status, text || res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export class FlowAPIError extends Error {
  constructor(public status: number, message: string) { super(message); }
}

export const flowAPI = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  delete: <T>(path: string) => request<T>('DELETE', path),
};
