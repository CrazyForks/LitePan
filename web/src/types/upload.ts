export type UploadTaskStatus =
  | "pending"
  | "running"
  | "paused"
  | "success"
  | "failed"
  | "canceled"
  | "skipped";

export interface UploadTask {
  task_id: string;
  client_task_id?: string;
  account_id: number;
  account_name?: string;
  driver_type?: string;
  file_name: string;
  target_path: string;
  target_display_path?: string;
  status: UploadTaskStatus;
  progress: number;
  uploaded_bytes?: number;
  speed_bytes_per_second?: number;
  total_bytes?: number;
  message?: string;
  error?: string;
  result?: {
    file_id?: string;
    parent_id?: string;
    parent_path?: string;
    file_name?: string;
    size?: number;
  };
  queue_order?: number;
  created_at?: number;
  updated_at?: number;
}

export interface BatchDeleteUploadResult {
  deleted_task_ids: string[];
  failed_task_ids: string[];
  missing_task_ids: string[];
  failed_messages: Record<string, string>;
}
