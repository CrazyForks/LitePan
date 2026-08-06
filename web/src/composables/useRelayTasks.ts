import { computed, onUnmounted, ref } from "vue";
import { ApiError } from "@/api/client";
import {
  deleteCrossTransferRelayTasks,
  listCrossTransferRelayTasks,
  type CrossTransferRelayTask,
} from "@/api/crossTransfer";

function relayTaskActivityRank(task: CrossTransferRelayTask) {
  if (task.status === "running") return 0;
  if (task.status === "pending") return 1;
  return 2;
}

export function useRelayTasks() {
  const relayTasks = ref<CrossTransferRelayTask[]>([]);
  const relayTaskOrderMap = ref<Record<string, number>>({});
  let relayTaskOrderCounter = 0;
  let relayPollingTimer: ReturnType<typeof setInterval> | null = null;
  let relayEventSource: EventSource | null = null;
  let relaySseReconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let relayAuthDenied = false;

  function isAdminAuthError(error: unknown) {
    return error instanceof ApiError && error.status === 401 && error.errorType === "ADMIN_AUTH_REQUIRED";
  }

  function ensureRelayTaskDisplayOrder(task: CrossTransferRelayTask) {
    const key = String(task.task_id || "");
    if (!key || relayTaskOrderMap.value[key]) return;
    const preferred = Number(task.queue_order || 0);
    const next = preferred > 0 ? preferred : relayTaskOrderCounter + 1;
    relayTaskOrderCounter = Math.max(relayTaskOrderCounter, next);
    relayTaskOrderMap.value = { ...relayTaskOrderMap.value, [key]: next };
  }

  function sortRelayTasksForDisplay(tasks: CrossTransferRelayTask[]) {
    for (const task of tasks) ensureRelayTaskDisplayOrder(task);
    return [...tasks].sort((a, b) => {
      const rankA = relayTaskActivityRank(a);
      const rankB = relayTaskActivityRank(b);
      if (rankA !== rankB) return rankA - rankB;

      const orderA = relayTaskOrderMap.value[a.task_id];
      const orderB = relayTaskOrderMap.value[b.task_id];
      if (orderA && orderB && orderA !== orderB) return orderA - orderB;

      const queueOrderA = Number(a.queue_order || 0);
      const queueOrderB = Number(b.queue_order || 0);
      if (queueOrderA > 0 && queueOrderB > 0 && queueOrderA !== queueOrderB) {
        return queueOrderA - queueOrderB;
      }

      const createdAtA = Number(a.created_at || 0);
      const createdAtB = Number(b.created_at || 0);
      if (createdAtA !== createdAtB) return createdAtA - createdAtB;

      return (
        Number(orderA || Number.MAX_SAFE_INTEGER) - Number(orderB || Number.MAX_SAFE_INTEGER)
      );
    });
  }

  const displayRelayTasks = computed(() => sortRelayTasksForDisplay(relayTasks.value));

  const activeRelayTasks = computed(() =>
    displayRelayTasks.value.filter((task) => ["pending", "running"].includes(task.status)),
  );

  const failedRelayTasks = computed(() =>
    displayRelayTasks.value.filter((task) => ["failed", "canceled"].includes(task.status)),
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
    for (const id of taskIds) {
      if (!relayTaskOrderMap.value[id]) continue;
      const next = { ...relayTaskOrderMap.value };
      delete next[id];
      relayTaskOrderMap.value = next;
    }
    await fetchRelayTasks();
  }

  onUnmounted(() => {
    disconnectRelayStream();
  });

  return {
    relayTasks,
    displayRelayTasks,
    activeRelayTasks,
    failedRelayTasks,
    activeRelayCount,
    fetchRelayTasks,
    connectRelayStream,
    disconnectRelayStream,
    batchDeleteRelayTasks,
  };
}
