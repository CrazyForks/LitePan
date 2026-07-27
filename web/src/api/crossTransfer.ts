import { http } from "./client";

export interface CrossTransferPanMeta {
  driver: string;
  name: string;
  logo: string;
  color: string;
  conflict_policies: string[];
}

export interface CrossTransferRoute {
  id: string;
  from: CrossTransferPanMeta;
  to: CrossTransferPanMeta;
  method: string;
  method_label: string;
  bidirectional: boolean;
}

export interface CrossTransferScanFile {
  source_file_id: string;
  rel_path: string;
  rel_dir: string;
  name: string;
  size: number;
  hash: string;
  eligible: boolean;
}

export interface CrossTransferScanResult {
  tree: unknown[];
  total: number;
  shallow_dirs: number;
  truncated: boolean;
  files: CrossTransferScanFile[];
}

export interface CrossTransferRelayTask {
  task_id: string;
  source_account_id: number;
  source_account_name: string;
  source_driver_type: string;
  target_account_id: number;
  target_account_name: string;
  target_driver_type: string;
  source_file_id: string;
  file_name: string;
  rel_path: string;
  rel_dir: string;
  target_parent_id: string;
  target_display_path: string;
  total_bytes: number;
  method: string;
  conflict_policy: string;
  status: string;
  phase: string;
  progress: number;
  downloaded_bytes: number;
  uploaded_bytes: number;
  speed_bytes_per_second: number;
  message: string;
  error: string;
  result?: Record<string, unknown>;
  queue_order?: number;
  created_at: number;
  updated_at: number;
}

export function listCrossTransferRoutes() {
  return http.get<CrossTransferRoute[]>("/cross-transfer/routes");
}

export function scanCrossTransferSource(body: {
  source_account_id: number;
  source_parent_id: string;
  method: string;
  source_display_path?: string;
}) {
  return http.post<CrossTransferScanResult>("/cross-transfer/scan", body);
}

export async function* streamCrossTransferNDJSON<T extends Record<string, unknown>>(
  path: string,
  body: unknown,
  signal?: AbortSignal,
): AsyncGenerator<T> {
  const resp = await fetch(`/api${path}`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      Origin: window.location.origin,
    },
    body: JSON.stringify(body),
    signal,
  });
  if (!resp.ok || !resp.body) {
    let message = `请求失败 (${resp.status})`;
    try {
      const payload = await resp.json();
      if (payload?.message) message = payload.message;
    } catch {}
    throw new Error(message);
  }
  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let idx = buffer.indexOf("\n");
    while (idx >= 0) {
      const line = buffer.slice(0, idx).trim();
      buffer = buffer.slice(idx + 1);
      if (line) yield JSON.parse(line) as T;
      idx = buffer.indexOf("\n");
    }
  }
  const tail = buffer.trim();
  if (tail) yield JSON.parse(tail) as T;
}

export function probeCrossTransferStream(
  body: Record<string, unknown>,
  signal?: AbortSignal,
) {
  return streamCrossTransferNDJSON<Record<string, unknown>>("/cross-transfer/probe", body, signal);
}

export function executeCrossTransferStream(
  body: Record<string, unknown>,
  signal?: AbortSignal,
) {
  return streamCrossTransferNDJSON<Record<string, unknown>>("/cross-transfer/execute", body, signal);
}

export function listCrossTransferRelayTasks() {
  return http.get<CrossTransferRelayTask[]>("/cross-transfer/relay/tasks");
}

export function deleteCrossTransferRelayTasks(taskIds: string[]) {
  return http.post<{ removed: number }>("/cross-transfer/relay/tasks/batch-delete", {
    task_ids: taskIds,
  });
}
