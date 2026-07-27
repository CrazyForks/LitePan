import { http } from "./client";

export type StrmScrapeWriteMode = "missing_only" | "overwrite";
export type StrmScrapeItemStatus = "ok" | "miss" | "doubt";
export type StrmScrapeTVState = "ended" | "updating";

export interface StrmScrapeItem {
  id: string;
  rel_dir: string;
  strm_name?: string;
  title: string;
  year?: number;
  media_type: string;
  status: StrmScrapeItemStatus;
  has_nfo: boolean;
  has_poster: boolean;
  has_pending?: boolean;
  tmdb_id?: string;
  poster_url?: string;
  folder_name?: string;
  file_count: number;
  ep_local?: number;
  ep_tmdb?: number;
  ep_scraped?: number;
  tv_state?: StrmScrapeTVState | string;
  added_at?: string;
}

export interface StrmScrapeProgress {
  running: boolean;
  strm_task_id: number;
  total: number;
  done: number;
  skipped: number;
  failed: number;
  message: string;
  error?: string;
  started_at?: string;
  current_item_id: string;
  item_revision: number;
  updated_item?: StrmScrapeItem;
}

export interface StrmScrapeRematchResult {
  item: StrmScrapeItem;
  started: boolean;
  progress: StrmScrapeProgress;
}

export interface StrmScrapeSettings {
  write_mode: StrmScrapeWriteMode;
  tmdb_api_key: string;
  tmdb_language: string;
  tmdb_request_interval_ms: number;
  proxy_enabled: boolean;
  proxy_url: string;
  proxy_username: string;
  proxy_password: string;
}

export function fetchStrmScrapeSettings() {
  return http.get<StrmScrapeSettings>("/admin/strm-scrape/settings");
}

export function saveStrmScrapeSettings(settings: Partial<StrmScrapeSettings>) {
  return http.put<StrmScrapeSettings>("/admin/strm-scrape/settings", settings);
}

export function runStrmScrape(strmTaskId: number, writeMode?: StrmScrapeWriteMode) {
  return http.post<StrmScrapeProgress>("/admin/strm-scrape/run", {
    strm_task_id: strmTaskId,
    write_mode: writeMode,
  });
}

export function stopStrmScrape() {
  return http.post<StrmScrapeProgress>("/admin/strm-scrape/stop");
}

export function fetchStrmScrapeProgress() {
  return http.get<StrmScrapeProgress>("/admin/strm-scrape/progress");
}

export function fetchStrmScrapeItems(strmTaskId: number) {
  return http.get<{ items: StrmScrapeItem[] }>("/admin/strm-scrape/items", {
    strm_task_id: strmTaskId,
  });
}

export function refreshStrmScrapeIndex(strmTaskId: number) {
  return http.post<{ items: StrmScrapeItem[] }>("/admin/strm-scrape/refresh-index", {
    strm_task_id: strmTaskId,
  });
}

export function rematchStrmScrapeItem(input: {
  strm_task_id: number;
  item_id: string;
  tmdb_id: string;
  media_type: string;
  title?: string;
  year?: number;
}) {
  return http.post<StrmScrapeRematchResult>("/admin/strm-scrape/rematch", input);
}

export function markStrmScrapeNormal(input: { strm_task_id: number; item_id: string }) {
  return http.post<StrmScrapeItem>("/admin/strm-scrape/mark-normal", input);
}

export function rescrapeStrmScrapeItem(input: { strm_task_id: number; item_id: string }) {
  return http.post<StrmScrapeRematchResult>("/admin/strm-scrape/rescrape", input);
}
