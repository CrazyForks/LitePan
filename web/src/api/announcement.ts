import { http } from "./client";

export interface AnnouncementSection {
  title: string;
  body: string;
}

export interface AnnouncementItem {
  /** 判重版本：日期字符串（如 2026-08-20）或内容哈希；比本地已读版本更新才弹出 */
  notice_version: string;
  badge: string;
  dialog_title: string;
  /** 黄色警示区（纯文字） */
  banner: string;
  /** 特别说明区：banner 之下、正文之上，支持 ![alt](url) 图片 */
  special: string;
  lead: string;
  issues: AnnouncementSection[];
  footnote: string;
  fetched_at: string;
}

export interface AnnouncementResponse {
  enabled: boolean;
  item: AnnouncementItem | null;
}

// 后台公告：enabled=false 或 item 为 null 时不弹窗。
export async function fetchAnnouncement() {
  return http.get<AnnouncementResponse>("/admin/announcement");
}
