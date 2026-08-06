import { computed, onUnmounted, ref } from "vue";
import { ApiError } from "@/api/client";
import {
  deleteCrossTransferRelayTasks,
  listCrossTransferRelayTasks,
  type CrossTransferRelayTask,
} from "@/api/crossTransfer";

export function useRelayTasks() {
  const relayTasks = ref<CrossTransferRelayTask[]>([]);
  let relayPollingTimer: ReturnType<typeof setInterval> | null = null;
  let relayEventSource: EventSource | null = null;
  let relaySseReconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let relayAuthDenied = false;

  function isAdminAuthError(error: unknown) {
    return error instanceof ApiError && error.status === 401 && error.errorType === "ADMIN_AUTH_REQUIRED";
  }

  const activeRelayTasks = computed(() =>
    relayTasks.value.filter((task) => ["pending", "running"].includes(task.status)),
  );

  const failedRelayTasks = computed(() =>
    relayTasks.value.filter((task) => ["failed", "canceled"].includes(task.status)),
  );

  const activeRelayCount = computed(() => activeRelayTasks.value.length);

  function applyRelayTasks(tasks: CrossTransferRelayTask[]) {
    relayTasks.value = Array.isArray(tasks) ? tasks : [];
  }

  async function fetchRelayTasks() {
    try {
      applyRelayTasks(await listCrossTransferRelayTasks());
      relayAuthDenied = false;
    } catch (error) {
      if (isAdminAuthError(error)) {
        relayAuthDenied = true;
        disconnectRelayStream();
        stopRelayPolling();
        return;
      }
      console.error("获取跨盘任务失败:", error);
    }
  }

  function stopRelayPolling() {
    if (relayPollingTimer) {
      clearInterval(relayPollingTimer);
      relayPollingTimer = null;
    }
  }

  function startRelayPolling() {
    if (relayPollingTimer || relayAuthDenied) return;
    relayPollingTimer = setInterval(() => {
      void fetchRelayTasks();
    }, 4000);
  }

  function clearRelaySseReconnectTimer() {
    if (relaySseReconnectTimer) {
      clearTimeout(relaySseReconnectTimer);
      relaySseReconnectTimer = null;
    }
  }

  function disconnectRelayStream() {
    clearRelaySseReconnectTimer();
    if (relayEventSource) {
      relayEventSource.close();
      relayEventSource = null;
    }
    stopRelayPolling();
  }

  function scheduleRelayStreamReconnect(panelOpen?: boolean) {
    if (!panelOpen || relaySseReconnectTimer || relayAuthDenied) return;
    relaySseReconnectTimer = setTimeout(() => {
      relaySseReconnectTimer = null;
      if (relayAuthDenied) return;
      connectRelayStream(panelOpen);
    }, 3000);
  }

  function connectRelayStream(panelOpen?: boolean) {
    if (!panelOpen || relayEventSource || relayAuthDenied) return;
    if (typeof window === "undefined" || !window.EventSource) {
      startRelayPolling();
      void fetchRelayTasks();
      return;
    }
    stopRelayPolling();
    relayEventSource = new EventSource("/api/cross-transfer/relay/tasks/stream");
    relayEventSource.addEventListener("tasks", (event) => {
      try {
        const payload = JSON.parse((event as MessageEvent).data || "{}");
        applyRelayTasks(payload.tasks || []);
      } catch {}
    });
    relayEventSource.onerror = () => {
      disconnectRelayStream();
      if (relayAuthDenied) return;
      scheduleRelayStreamReconnect(panelOpen);
    };
  }

  async function batchDeleteRelayTasks(taskIds: string[]) {
    if (!taskIds.length) return;
    await deleteCrossTransferRelayTasks(taskIds);
    await fetchRelayTasks();
  }

  onUnmounted(() => {
    disconnectRelayStream();
  });

  return {
    relayTasks,
    activeRelayTasks,
    failedRelayTasks,
    activeRelayCount,
    fetchRelayTasks,
    connectRelayStream,
    disconnectRelayStream,
    batchDeleteRelayTasks,
  };
}
