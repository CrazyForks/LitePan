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

export interface AIOrganizeConfig {
  enabled: boolean;
  base_url: string;
  api_key: string;
  model: string;
}

export interface QuarkTVBinding {
  account_id: number;
  account_name: string;
  tv_nickname: string;
  preferred_resolution: string;
  allow_dolby: boolean;
  membership: string;
}

export interface QuarkTVStatus {
  enabled: boolean;
  available: boolean;
  bindings: QuarkTVBinding[];
}

export interface QuarkTVAccount {
  id: number;
  name: string;
}

export interface QuarkTVBindStart {
  token: string;
  qr_image: string;
  expires_in: number;
}

export interface QuarkTVBindPoll {
  status: "waiting" | "success" | "failed" | "expired";
  message: string;
}

export interface QuarkTVBindingSettingsPayload {
  account_id: number;
  preferred_resolution: string;
  allow_dolby: boolean;
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

export const aiOrganizeApi = {
  getConfig: () => http.get<AIOrganizeConfig>("/admin/tools/ai-organize/config"),
  saveConfig: (payload: AIOrganizeConfig) =>
    http.put<AIOrganizeConfig>("/admin/tools/ai-organize/config", payload),
  testConfig: (payload: AIOrganizeConfig) =>
    http.post<{ ok: boolean }>("/admin/tools/ai-organize/test", payload),
};

export const quarkTVApi = {
  status: () => http.get<QuarkTVStatus>("/admin/tools/quarktv/status"),
  setEnabled: (enabled: boolean) =>
    http.post<{ enabled: boolean }>("/admin/tools/quarktv/enabled", { enabled }),
  accounts: () => http.get<{ accounts: QuarkTVAccount[] }>("/admin/tools/quarktv/accounts"),
  bindStart: (accountId: number) =>
    http.post<QuarkTVBindStart>("/admin/tools/quarktv/bind/start", { account_id: accountId }),
  bindPoll: (token: string) =>
    http.post<QuarkTVBindPoll>("/admin/tools/quarktv/bind/poll", { token }),
  updateBindingSettings: (payload: QuarkTVBindingSettingsPayload) =>
    http.put<QuarkTVBinding>("/admin/tools/quarktv/binding/settings", payload),
  unbind: (accountId: number) =>
    http.del<{ removed: boolean }>("/admin/tools/quarktv/bind", { account_id: accountId }),
};
