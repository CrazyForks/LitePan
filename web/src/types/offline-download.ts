export interface OfflineDownloadCapabilities {
  supported: boolean;
  supports_urls: boolean;
  supports_batch_urls: boolean;
  supports_torrent: boolean;
  url_schemes: string[];
  root_target_allowed: boolean;
  remote_delete: boolean;
}

export type OfflineDownloadStatus = "pending" | "running" | "retrying" | "success" | "failed";

export interface OfflineDownloadTask {
  task_id: string;
  account_id: number;
  account_name: string;
  driver_type: string;
  source_kind: "url" | "bt";
  source: string;
  name: string;
  provider_task_id?: string;
  info_hash?: string;
  target_parent_id: string;
  target_display_path: string;
  status: OfflineDownloadStatus;
  progress: number;
  size: number;
  file_id?: string;
  message: string;
  error?: string;
  remote_delete: boolean;
  created_at: number;
  updated_at: number;
}

export interface OfflineTorrentFile {
  index: number;
  path: string;
  size: number;
  wanted: boolean;
}

export interface OfflineTorrentPreparation {
  preparation_id: string;
  torrent_name: string;
  total_size: number;
  files: OfflineTorrentFile[];
  expires_at: number;
}

export interface OfflineBatchDeleteResult {
  deleted_task_ids: string[];
  failed_task_ids: string[];
  failed_messages: Record<string, string>;
}
