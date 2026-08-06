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

export interface EmbyLibrary {
  id: string;
  name: string;
  collection_type?: string;
}

export interface EmbyRefreshRequest {
  mode?: "global" | "library";
  library_id?: string;
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

export function fetchEmbyLibraries() {
  return http.get<EmbyLibrary[]>("/admin/emby/libraries");
}

export function refreshEmbyLibrary(body: EmbyRefreshRequest = {}) {
  return http.post<{ mode: string; task_id?: string; library_id?: string; library_name?: string }>("/admin/emby/refresh", body);
}
