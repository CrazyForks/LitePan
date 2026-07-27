import { http } from "./client";

export interface EmbyConfig {
  enabled: boolean;
  emby_url: string;
  api_key: string;
  proxy_port: string;
  proxy_url: string;
  running: boolean;
  last_error?: string;
}

export interface EmbyConfigUpdate {
  enabled: boolean;
  emby_url: string;
  api_key: string;
  proxy_port: string;
}

export function fetchEmbyConfig() {
  return http.get<EmbyConfig>("/admin/emby/config");
}

export function saveEmbyConfig(values: EmbyConfigUpdate) {
  return http.put<EmbyConfig>("/admin/emby/config", values);
}

export function testEmbyConfig(values: EmbyConfigUpdate) {
  return http.post<{ ok: boolean }>("/admin/emby/test", values);
}

export function refreshEmbyLibrary() {
  return http.post<{ mode: string; task_id?: string }>("/admin/emby/refresh");
}
