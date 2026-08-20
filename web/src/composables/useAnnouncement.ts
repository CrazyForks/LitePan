import { ref } from "vue";
import { fetchAnnouncement, type AnnouncementItem } from "@/api/announcement";

// 模块级单例：公告状态（打开后台自动检查 + 侧边栏「关于」手动查看共用）。
const open = ref(false);
const item = ref<AnnouncementItem | null>(null);

const READ_VERSION_KEY = "litepan:announcement:read-version";

// 版本判重：对齐文档站 important-notice 机制——notice_version 与本地已读版本「不同」即弹出。
// 这样一天内发多次公告时，只要每次换一个新版本号（如 2026-08-20-1 / 2026-08-20-2
// 或带时间 2026-08-20T10:30:00），已读过的用户也会再次弹出；版本号相同则不重复弹。
function shouldShow(remote: string, local: string): boolean {
  return local === "" || remote !== local;
}

function readVersion(): string {
  try {
    return localStorage.getItem(READ_VERSION_KEY) ?? "";
  } catch {
    return "";
  }
}

function persistVersion(version: string): void {
  try {
    localStorage.setItem(READ_VERSION_KEY, version);
  } catch {}
}

async function load(): Promise<AnnouncementItem | null> {
  try {
    const res = await fetchAnnouncement();
    const it = res.item;
    if (!res.enabled || !it || !it.notice_version) return null;
    return it;
  } catch {
    return null;
  }
}

export function useAnnouncement() {
  // 打开后台时检查：远端版本号与本地已读不同才弹出。
  async function check(): Promise<void> {
    const it = await load();
    if (!it) return;
    if (!shouldShow(it.notice_version, readVersion())) return;
    item.value = it;
    open.value = true;
  }

  // 手动查看（点「关于」）：拉到公告即无条件弹出，不改变已读状态。
  async function forceOpen(): Promise<boolean> {
    const it = await load();
    if (!it) return false;
    item.value = it;
    open.value = true;
    return true;
  }

  // 关闭并标记当前公告为已读（与文档站「我知道了」语义一致：版本号不同会再次弹出）。
  function dismiss(): void {
    if (item.value) persistVersion(item.value.notice_version);
    open.value = false;
  }

  function close(): void {
    open.value = false;
  }

  return { open, item, check, forceOpen, dismiss, close };
}
