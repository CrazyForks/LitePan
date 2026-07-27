import {
  canDeleteUploadTask,
  canHandleUploadTaskPrimaryAction,
  formatRelayPart,
  formatRelaySpeed,
  formatUploadPart,
  getRelayPhaseLabel,
  getRelayStatusText,
  getUploadTaskDriverBadge,
  getUploadTaskPhaseLabel,
  getUploadTaskPrimaryActionIcon,
  getUploadTaskPrimaryActionTitle,
  getUploadTaskSpeedText,
  getUploadTaskStatusText,
  isRelayTaskActive,
  isUploadTaskActive,
  shouldShowRelayTaskHairline,
  shouldShowRelayTaskMetaPercent,
  shouldShowUploadTaskHairline,
  shouldShowUploadTaskMetaPercent,
} from "@/composables/upload/uploadTaskFormatters";
import { useLocalUploadDispatcher } from "@/composables/upload/useLocalUploadDispatcher";
import { useUploadTaskActions } from "@/composables/upload/useUploadTaskActions";
import { useUploadTaskStore } from "@/composables/upload/useUploadTaskStore";
import { useUploadTaskStream } from "@/composables/upload/useUploadTaskStream";
import type { UploadRuntimeHooks, UploadTaskDeps } from "@/composables/upload/uploadTaskTypes";

export type { UploadTaskDeps as Deps } from "@/composables/upload/uploadTaskTypes";

export function useUploadTasks(deps: UploadTaskDeps) {
  const store = useUploadTaskStore(deps);

  const hooks: UploadRuntimeHooks = {
    startScheduler: async () => {},
    fetchTasks: async () => {},
    startPolling: () => {},
    stopPolling: () => {},
    connectStream: () => {},
    disconnectStream: () => {},
    closePanel: () => {},
  };

  const stream = useUploadTaskStream(deps, store, hooks);
  const dispatcher = useLocalUploadDispatcher(deps, store, stream);
  const actions = useUploadTaskActions(deps, store, stream, dispatcher);

  hooks.startScheduler = dispatcher.startUploadTaskScheduler;
  hooks.fetchTasks = stream.fetchUploadTasks;
  hooks.startPolling = stream.startUploadTaskPolling;
  hooks.stopPolling = stream.stopUploadTaskPolling;
  hooks.connectStream = stream.connectUploadTaskStream;
  hooks.disconnectStream = stream.disconnectUploadTaskStream;
  hooks.closePanel = actions.closeUploadTaskPanel;

  const getUploadTaskPhaseLabelBound = (task: Parameters<typeof getUploadTaskPhaseLabel>[0]) =>
    getUploadTaskPhaseLabel(task, store.pendingRemoteResumeTaskIds, store.localDispatchingTaskIds);

  const getUploadTaskDriverBadgeBound = (
    task: Parameters<typeof getUploadTaskDriverBadge>[0],
  ) => getUploadTaskDriverBadge(task, deps.accounts.value);

  const getRelayTaskDriverBadge = (task: {
    target_driver_type?: string;
    target_account_id?: number;
    target_account_name?: string;
  }) =>
    getUploadTaskDriverBadge(
      {
        driver_type: task.target_driver_type,
        account_id: task.target_account_id ?? 0,
        account_name: task.target_account_name,
      },
      deps.accounts.value,
    );

  return {
    uploadTaskPanelOpen: store.uploadTaskPanelOpen,
    taskPanelCategory: store.taskPanelCategory,
    uploadTaskView: store.uploadTaskView,
    uploadTaskPanelLoading: store.uploadTaskPanelLoading,
    uploadTaskPanelLoadingText: store.uploadTaskPanelLoadingText,
    uploadTaskServerConcurrency: store.uploadTaskServerConcurrency,
    displayUploadTasks: store.displayUploadTasks,
    activeUploadTasks: store.activeUploadTasks,
    runningUploadTasks: store.runningUploadTasks,
    completedUploadTasks: store.completedUploadTasks,
    filteredUploadTasks: store.filteredUploadTasks,
    canBatchToggle: store.canBatchToggle,
    canBatchDelete: store.canBatchDelete,
    batchToggleMode: store.batchToggleMode,
    batchToggleLabel: store.batchToggleLabel,
    batchToggleTitle: store.batchToggleTitle,
    batchDeleteLabel: store.batchDeleteLabel,
    uploadTaskEmptyText: store.uploadTaskEmptyText,
    uploadTaskTitle: store.uploadTaskTitle,
    uploadTaskLabel: store.uploadTaskLabel,
    getUploadTaskStatusText,
    formatUploadPart,
    getUploadTaskSpeedText,
    getUploadTaskDriverBadge: getUploadTaskDriverBadgeBound,
    isUploadTaskActive,
    getUploadTaskPhaseLabel: getUploadTaskPhaseLabelBound,
    shouldShowUploadTaskMetaPercent,
    shouldShowUploadTaskHairline,
    canHandleUploadTaskPrimaryAction,
    getUploadTaskPrimaryActionTitle,
    getUploadTaskPrimaryActionIcon,
    canDeleteUploadTask,
    handleDeleteUploadTask: actions.handleDeleteUploadTask,
    handleUploadTaskPrimaryAction: actions.handleUploadTaskPrimaryAction,
    handleBatchToggle: actions.handleBatchToggle,
    handleBatchDelete: actions.handleBatchDelete,
    openUploadTaskPanel: actions.openUploadTaskPanel,
    closeUploadTaskPanel: actions.closeUploadTaskPanel,
    openUploadNoticeFromPanel: actions.openUploadNoticeFromPanel,
    handleUploadFile: actions.handleUploadFile,
    handleUploadFolder: actions.handleUploadFolder,
    handleUploadFileChange: actions.handleUploadFileChange,
    handleUploadFolderChange: actions.handleUploadFolderChange,
    fetchUploadTasks: stream.fetchUploadTasks,
    refreshUploadTaskServerConcurrency: stream.refreshUploadTaskServerConcurrency,
    getRelayTaskDriverBadge,
    handleDeleteRelayTask: actions.handleDeleteRelayTask,
    handleBatchDeleteRelayTasks: actions.handleBatchDeleteRelayTasks,
    getRelayStatusText,
    getRelayPhaseLabel,
    shouldShowRelayTaskMetaPercent,
    shouldShowRelayTaskHairline,
    isRelayTaskActive,
    formatRelaySpeed,
    formatRelayPart,
    filteredRelayTasks: store.filteredRelayTasks,
    runningRelayTasks: store.runningRelayTasks,
    completedRelayTasks: store.completedRelayTasks,
    activeRelayCount: store.activeRelayCount,
    cleanupUploadTasks: stream.cleanupUploadTasks,
  };
}
