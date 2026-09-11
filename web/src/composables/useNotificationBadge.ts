import { ref } from "vue";
import { ApiError } from "@/api/client";
import { fetchUnreadCount } from "@/api/notifications";

// 未读通知数全局单例：以服务端 SSE 推送为主，连接不可用时退回轮询。
// 组件只读这里的 unreadCount，避免多个铃铛实例各开一条连接、各自轮询。
const unreadCount = ref(0);
// 每次收到服务端推送自增，通知面板据此在打开状态下刷新列表。
const unreadRevision = ref(0);

const STREAM_URL = "/api/admin/notifications/stream";
// SSE 在线时的兜底巡检（防止长连接静默失效）。
const ONLINE_POLL_MS = 60_000;
// SSE 断开后的轮询频率，与改造前保持一致。
const OFFLINE_POLL_MS = 30_000;
const RECONNECT_MS = 10_000;

let source: EventSource | null = null;
let pollTimer: ReturnType<typeof setInterval> | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let mountedConsumers = 0;
let authDenied = false;

function isAdminAuthError(error: unknown) {
  return error instanceof ApiError && error.status === 401 && error.errorType === "ADMIN_AUTH_REQUIRED";
}

function applyCount(next: number) {
  unreadCount.value = Number.isFinite(next) && next > 0 ? Math.floor(next) : 0;
}

/** 本地已读/删除后的乐观更新；服务端推送到达时会覆盖成权威值。 */
export function setUnreadCount(next: number) {
  applyCount(next);
}

export async function refreshUnread() {
  if (authDenied) return;
  try {
    const data = await fetchUnreadCount();
    authDenied = false;
    applyCount(data.count ?? 0);
  } catch (error) {
    if (isAdminAuthError(error)) {
      authDenied = true;
      stopBadgeRuntime();
    }
  }
}

function startPolling(interval: number) {
  if (pollTimer) clearInterval(pollTimer);
  pollTimer = setInterval(() => void refreshUnread(), interval);
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

function disconnectStream() {
  source?.close();
  source = null;
}

function scheduleReconnect() {
  if (reconnectTimer || authDenied) return;
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    connectStream();
  }, RECONNECT_MS);
}

function connectStream() {
  if (authDenied || source) return;
  if (typeof EventSource === "undefined") {
    startPolling(OFFLINE_POLL_MS);
    return;
  }
  const es = new EventSource(STREAM_URL);
  source = es;
  es.addEventListener("unread", (ev) => {
    try {
      const payload = JSON.parse((ev as MessageEvent).data || "{}") as { count?: number };
      applyCount(Number(payload.count ?? 0));
      unreadRevision.value += 1;
    } catch {
      /* 坏帧不处理，兜底轮询会纠正 */
    }
  });
  es.onopen = () => startPolling(ONLINE_POLL_MS);
  es.onerror = () => {
    // 自己接管重连，避免 EventSource 默认重连策略不可控。
    disconnectStream();
    if (authDenied) return;
    startPolling(OFFLINE_POLL_MS);
    scheduleReconnect();
  };
}

function stopBadgeRuntime() {
  disconnectStream();
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  stopPolling();
  document.removeEventListener("visibilitychange", onVisibilityChange);
}

function onVisibilityChange() {
  // 休眠/切后台期间长连接可能已被断开，回到前台先对齐一次。
  if (document.visibilityState === "visible") void refreshUnread();
}

/** 组件挂载时调用，可重复调用（内部按消费方计数）。 */
export function startNotificationBadge() {
  mountedConsumers += 1;
  if (mountedConsumers > 1) return;
  authDenied = false;
  void refreshUnread();
  connectStream();
  // 没有 EventSource 的浏览器直接按离线频率轮询；有长连接的走低频兜底巡检。
  startPolling(typeof EventSource === "undefined" ? OFFLINE_POLL_MS : ONLINE_POLL_MS);
  document.addEventListener("visibilitychange", onVisibilityChange);
}

/** 组件卸载时调用，最后一个消费方退出才真正断开。 */
export function stopNotificationBadge() {
  mountedConsumers = Math.max(0, mountedConsumers - 1);
  if (mountedConsumers > 0) return;
  stopBadgeRuntime();
}

export function useNotificationBadge() {
  return { unreadCount, unreadRevision };
}
