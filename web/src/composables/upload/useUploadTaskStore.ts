import { computed, ref } from "vue";
import type { UploadTask } from "@/types/upload";
import { getUploadTaskStableKey } from "@/composables/upload/uploadTaskFormatters";
import type { LocalUploadPayload, UploadTaskDeps } from "@/composables/upload/uploadTaskTypes";

export function useUploadTaskStore(deps: UploadTaskDeps) {
  const uploadTasks = ref<UploadTask[]>([]);
  const localUploadTasks = ref<UploadTask[]>([]);
  const uploadTaskPanelOpen = ref(false);
  const taskPanelCategory = ref<"upload" | "relay" | "offline">("upload");
  const uploadTaskView = ref<"running" | "completed">("running");
  const uploadTaskPanelLoading = ref(false);
  const uploadTaskPanelLoadingText = ref("正在准备上传任务...");
  const uploadTaskOrderMap = ref<Record<string, number>>({});
  const uploadTaskServerConcurrency = ref(3);
  const batchPauseInProgress = ref(false);

  let uploadTaskOrderCounter = 0;

  const localUploadTaskControllers = new Map<string, AbortController>();
  const localUploadTaskPayloads = new Map<string, LocalUploadPayload>();
  const canceledLocalUploadTaskIds = new Set<string>();
  const pausedLocalUploadTaskIds = new Set<string>();
  const localDispatchingTaskIds = new Set<string>();
  const pendingRemoteResumeTaskIds = new Set<string>();
  const hiddenUploadTaskKeys = new Set<string>();
  let folderUploadRefreshPending = false;

  const {
    filteredRelayTasks,
    runningRelayTasks,
    completedRelayTasks,
    activeRelayCount,
  } = deps.relay;

  function uploadAffectsCurrentDirectory(task: UploadTask, currentPath: string) {
    if (String(task.account_id) !== String(deps.selectedAccountId.value)) return false;
    if (task.status !== "success" && task.status !== "skipped") return false;
    const parentId = String(task.result?.parent_id ?? task.target_path ?? "");
    return parentId === currentPath || task.target_path === currentPath;
  }

  function ensureUploadTaskDisplayOrder(task: UploadTask) {
    const key = getUploadTaskStableKey(task);
    if (!key || uploadTaskOrderMap.value[key]) return;
    const preferred = Number(task.queue_order || 0);
    const next = preferred > 0 ? preferred : uploadTaskOrderCounter + 1;
    uploadTaskOrderCounter = Math.max(uploadTaskOrderCounter, next);
    uploadTaskOrderMap.value = { ...uploadTaskOrderMap.value, [key]: next };
  }

  const displayUploadTasks = computed(() => {
    const merged = [...localUploadTasks.value, ...uploadTasks.value].filter(
      (t) => !hiddenUploadTaskKeys.has(getUploadTaskStableKey(t)),
    );
    return [...merged].sort((a, b) => {
      const oa = uploadTaskOrderMap.value[getUploadTaskStableKey(a)] ?? Number.MAX_SAFE_INTEGER;
      const ob = uploadTaskOrderMap.value[getUploadTaskStableKey(b)] ?? Number.MAX_SAFE_INTEGER;
      return oa - ob;
    });
  });

  const activeUploadTasks = computed(() =>
    displayUploadTasks.value.filter((t) => t.status === "pending" || t.status === "running"),
  );
  const runningUploadTasks = computed(() =>
    displayUploadTasks.value.filter((t) =>
      ["pending", "running", "failed", "paused", "canceled"].includes(t.status),
    ),
  );
  const completedUploadTasks = computed(() =>
    displayUploadTasks.value.filter((t) => t.status === "success" || t.status === "skipped"),
  );
  const filteredUploadTasks = computed(() =>
    uploadTaskView.value === "completed" ? completedUploadTasks.value : runningUploadTasks.value,
  );

  const canBatchPause = computed(() =>
    filteredUploadTasks.value.some((t) => t.status === "pending" || t.status === "running"),
  );
  const canBatchResume = computed(() =>
    filteredUploadTasks.value.some((t) => ["failed", "paused", "canceled"].includes(t.status)),
  );
  const canBatchToggle = computed(() => canBatchPause.value || canBatchResume.value);
  const canBatchDelete = computed(() => filteredUploadTasks.value.length > 0);
  const batchToggleMode = computed(() => (canBatchResume.value ? "resume" : "pause"));
  const batchToggleLabel = computed(() => (batchToggleMode.value === "pause" ? "全部暂停" : "全部开始"));
  const batchDeleteLabel = computed(() =>
    uploadTaskView.value === "completed" ? "全部清空" : "全部删除",
  );
  const uploadTaskEmptyText = computed(() =>
    uploadTaskView.value === "completed" ? "暂无已完成任务" : "暂无进行中的上传任务",
  );

  const uploadTaskBadgeText = computed(() => {
    const running = activeUploadTasks.value.length;
    if (running > 0) return `上传中 ${running}`;
    if (activeRelayCount.value > 0) return `跨盘中 ${activeRelayCount.value}`;
    const failed = displayUploadTasks.value.filter((t) => t.status === "failed").length;
    if (failed > 0) return `失败 ${failed}`;
    const paused = displayUploadTasks.value.filter((t) => t.status === "paused").length;
    if (paused > 0) return `已暂停 ${paused}`;
    const success = displayUploadTasks.value.filter((t) => t.status === "success").length;
    if (success > 0) return `上传完成 ${success}`;
    return "";
  });

  const uploadTaskTitle = computed(() => uploadTaskBadgeText.value || "传输列表");
  const uploadTaskLabel = computed(() => uploadTaskBadgeText.value || "暂无传输任务");
  const batchToggleTitle = computed(() =>
    !canBatchToggle.value
      ? "当前没有可操作的任务"
      : batchToggleMode.value === "pause"
        ? "暂停当前页签中的任务"
        : "继续当前页签中的任务",
  );

  function createLocalUploadTask(file: File, options: Partial<UploadTask> = {}): UploadTask {
    return {
      task_id: `local-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      account_id: deps.selectedAccountId.value as number,
      account_name: deps.selectedAccountName.value,
      file_name: options.file_name || file.name,
      target_path: options.target_path || deps.currentPath.value,
      target_display_path: options.target_display_path || "",
      status: "pending",
      progress: 0,
      message: "等待发送到 LitePan 服务器",
      error: "",
    };
  }

  function createSkippedUploadTask(file: File, reason: string, options: Partial<UploadTask> = {}): UploadTask {
    return {
      ...createLocalUploadTask(file, options),
      task_id: `local-skip-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      status: "skipped",
      message: reason,
    };
  }

  function addLocalUploadTask(task: UploadTask) {
    ensureUploadTaskDisplayOrder(task);
    localUploadTasks.value = [task, ...localUploadTasks.value];
  }

  function updateLocalUploadTask(taskId: string, patch: Partial<UploadTask>) {
    localUploadTasks.value = localUploadTasks.value.map((t) =>
      t.task_id === taskId ? { ...t, ...patch } : t,
    );
  }

  function removeLocalUploadTask(taskId: string) {
    localUploadTasks.value = localUploadTasks.value.filter((t) => t.task_id !== taskId);
  }

  function patchRemoteUploadTask(taskId: string, patch: Partial<UploadTask>) {
    uploadTasks.value = uploadTasks.value.map((t) => (t.task_id === taskId ? { ...t, ...patch } : t));
  }

  function removeRemoteUploadTask(taskId: string) {
    uploadTasks.value = uploadTasks.value.filter((t) => t.task_id !== taskId);
  }

  function markFolderUploadRefreshPending() {
    folderUploadRefreshPending = true;
  }

  function consumeFolderUploadRefreshPending() {
    const pending = folderUploadRefreshPending;
    folderUploadRefreshPending = false;
    return pending;
  }

  return {
    uploadTasks,
    localUploadTasks,
    uploadTaskPanelOpen,
    taskPanelCategory,
    uploadTaskView,
    uploadTaskPanelLoading,
    uploadTaskPanelLoadingText,
    uploadTaskOrderMap,
    uploadTaskServerConcurrency,
    batchPauseInProgress,
    localUploadTaskControllers,
    localUploadTaskPayloads,
    canceledLocalUploadTaskIds,
    pausedLocalUploadTaskIds,
    localDispatchingTaskIds,
    pendingRemoteResumeTaskIds,
    hiddenUploadTaskKeys,
    filteredRelayTasks,
    runningRelayTasks,
    completedRelayTasks,
    activeRelayCount,
    displayUploadTasks,
    activeUploadTasks,
    runningUploadTasks,
    completedUploadTasks,
    filteredUploadTasks,
    canBatchToggle,
    canBatchDelete,
    batchToggleMode,
    batchToggleLabel,
    batchToggleTitle,
    batchDeleteLabel,
    uploadTaskEmptyText,
    uploadTaskTitle,
    uploadTaskLabel,
    uploadAffectsCurrentDirectory,
    ensureUploadTaskDisplayOrder,
    createLocalUploadTask,
    createSkippedUploadTask,
    addLocalUploadTask,
    updateLocalUploadTask,
    removeLocalUploadTask,
    patchRemoteUploadTask,
    removeRemoteUploadTask,
    markFolderUploadRefreshPending,
    consumeFolderUploadRefreshPending,
  };
}

export type UploadTaskStore = ReturnType<typeof useUploadTaskStore>;
