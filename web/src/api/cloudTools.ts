import { http } from "./client";

export interface CloudTool115Status {
  enabled: boolean;
  cache_count: number;
  available: boolean;
}

export interface CloudTool115Accounts {
  accounts: { id: number; name: string; is_active: boolean }[];
}

export interface LocalUploadMapping {
  name: string;
  path: string;
}

export interface LocalUploadConfig {
  enabled: boolean;
  mappings: LocalUploadMapping[];
}

export interface LocalUploadEntry {
  name: string;
  is_dir: boolean;
  size: number;
  mtime: number;
  rel_path: string;
}

export interface LocalUploadBrowseResult {
  mapping: string;
  path: string;
  items: LocalUploadEntry[];
}

export interface LocalUploadCreatePayload {
  account_id: number;
  mapping: string;
  target_path: string;
  target_display_path?: string;
  conflict_policy: string;
  client_task_id: string;
  display_name?: string;
  items: { rel_path: string; is_dir: boolean }[];
}

export const cloudToolsApi = {
  status115: () => http.get<CloudTool115Status>("/admin/tools/115-strm/status"),
  set115Enabled: (enabled: boolean) =>
    http.post<{ enabled: boolean }>("/admin/tools/115-strm/enabled", { enabled }),
  clear115Cache: (accountId = 0) =>
    http.post<{ removed: number }>("/admin/tools/115-strm/cache/clear", { account_id: accountId }),
};

export const localUploadApi = {
  getConfig: () => http.get<LocalUploadConfig>("/admin/tools/local-upload/config"),
  saveConfig: (payload: LocalUploadConfig) =>
    http.put<LocalUploadConfig>("/admin/tools/local-upload/config", payload),
  browse: (mapping: string, path = "") =>
    http.post<LocalUploadBrowseResult>("/admin/tools/local-upload/browse", { mapping, path }),
  upload: (payload: LocalUploadCreatePayload) =>
    http.post<{ accepted: boolean; count: number }>("/admin/tools/local-upload/upload", payload),
};
