import { http } from "./client";

export interface CloudTool115Status {
  enabled: boolean;
  cache_count: number;
  available: boolean;
}

export interface CloudTool115Accounts {
  accounts: { id: number; name: string; is_active: boolean }[];
}

export const cloudToolsApi = {
  status115: () => http.get<CloudTool115Status>("/admin/tools/115-strm/status"),
  set115Enabled: (enabled: boolean) =>
    http.post<{ enabled: boolean }>("/admin/tools/115-strm/enabled", { enabled }),
  clear115Cache: (accountId = 0) =>
    http.post<{ removed: number }>("/admin/tools/115-strm/cache/clear", { account_id: accountId }),
};
