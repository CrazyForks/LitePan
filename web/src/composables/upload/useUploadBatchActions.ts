import { uploadApi } from "@/api/upload";
import { getApiErrorMessage } from "@/api/client";
import { toast } from "@/composables/useToast";
import {
  confirmBatchUploadTaskDelete,
  confirmUploadTaskDelete,
} from "@/composables/confirmUpload";
import { getUploadTaskStableKey, isLocalUploadTask, buildUploadTaskBreadcrumb } from "@/composables/upload/uploadTaskFormatters";
import type { UploadActionsCtx } from "@/composables/upload/useUploadPanelActions";
import type { UploadTask } from "@/types/upload";

export function useUploadBatchActions(ctx: UploadActionsCtx, closePanel: () => void) {
  const { deps, store, stream, dispatcher } = ctx;

  async function pauseUploadTask(task: UploadTask, silent = false) {
    if (isLocalUploadTask(task)) {
      store.pausedLocalUploadTaskIds.add(task.task_id);
      store.localUploadTaskControllers.get(task.task_id)?.abort();
      store.updateLocalUploadTask(task.task_id, { status: "paused", message: "上传已暂停", error: "" });
      return;
    }
    try {
      store.patchRemoteUploadTask(task.task_id, { status: "paused", message: "上传已暂停", error: "" });
      await uploadApi.pauseTask(task.task_id);
      await stream.fetchUploadTasks();
    } catch (e) {
      await stream.fetchUploadTasks();
      if (!silent) toast.error(getApiErrorMessage(e, "暂停上传任务失败"));
    }
  }

  async function resumeUploadTask(task: UploadTask, silent = false) {
    if (isLocalUploadTask(task)) {
      const payload = store.localUploadTaskPayloads.get(task.task_id);
      if (!payload?.file) {
        store.updateLocalUploadTask(task.task_id, {
          status: "failed",
          error: "缺少本地上传数据，无法继续",
        });
        if (!silent) toast.error("缺少本地上传数据，无法继续");
        return;
      }
      store.pausedLocalUploadTaskIds.delete(task.task_id);
      store.canceledLocalUploadTaskIds.delete(task.task_id);
      store.updateLocalUploadTask(task.task_id, { status: "pending", message: "等待上传", error: "" });
      store.uploadTaskView.value = "running";
      void dispatcher.startUploadTaskScheduler();
      return;
    }
    store.pendingRemoteResumeTaskIds.add(String(task.task_id));
    store.uploadTaskView.value = "running";
    void dispatcher.startUploadTaskScheduler();
    if (!silent) void stream.fetchUploadTasks();
  }

  async function handleDeleteUploadTask(
    task: UploadTask,
    opts: { silent?: boolean; skipDialog?: boolean; deleteUploadedFile?: boolean } = {},
  ) {
    if (!task.task_id) return;
    if (isLocalUploadTask(task)) {
      if (!opts.skipDialog) {
        const r = await confirmUploadTaskDelete(task.file_name, task.status === "success");
        if (!r) return;
      }
      store.canceledLocalUploadTaskIds.add(task.task_id);
      store.localUploadTaskControllers.get(task.task_id)?.abort();
      store.removeLocalUploadTask(task.task_id);
      store.localUploadTaskPayloads.delete(task.task_id);
      return;
    }
    const allowDeleteFile = task.status === "success";
    const r = opts.skipDialog
      ? { action: "confirm", checked: Boolean(opts.deleteUploadedFile) }
      : await confirmUploadTaskDelete(task.file_name, allowDeleteFile);
    if (!r) return;
    const deleteUploadedFile = allowDeleteFile && r.checked;
    store.hiddenUploadTaskKeys.add(getUploadTaskStableKey(task));
    store.removeRemoteUploadTask(task.task_id);
    try {
      await uploadApi.deleteTask(task.task_id, deleteUploadedFile);
      await stream.fetchUploadTasks();
      if (deleteUploadedFile && task.target_path === deps.currentPath.value) {
        await deps.loadFiles({ forceRefresh: true, silent: true });
      }
    } catch (e) {
      store.hiddenUploadTaskKeys.delete(getUploadTaskStableKey(task));
      await stream.fetchUploadTasks();
      if (!opts.silent) toast.error(getApiErrorMessage(e, "删除上传任务失败"));
    }
  }

  async function handleUploadTaskPrimaryAction(task: UploadTask) {
    if (["pending", "running"].includes(task.status)) {
      await pauseUploadTask(task);
      return;
    }
    if (["failed", "paused", "canceled"].includes(task.status)) {
      await resumeUploadTask(task);
      return;
    }
    if (!["success", "skipped"].includes(task.status)) return;
    const account = deps.accounts.value.find((a) => String(a.id) === String(task.account_id));
    if (!account) {
      toast.warning("未找到对应账号");
      return;
    }
    const crumbs = await buildUploadTaskBreadcrumb(account, task, deps.getRootId);
    deps.selectedFilesList.value = [];
    await deps.openDirectory(account.id, crumbs, { forceRefresh: true });
    closePanel();
  }

  async function handleBatchToggle() {
    if (store.batchToggleMode.value === "pause") {
      store.batchPauseInProgress.value = true;
      try {
        for (const task of store.filteredUploadTasks.value.filter((t) =>
          ["pending", "running"].includes(t.status),
        )) {
          await pauseUploadTask(task, true);
        }
      } finally {
        store.batchPauseInProgress.value = false;
      }
      return;
    }
    for (const task of store.filteredUploadTasks.value.filter((t) =>
      ["failed", "paused", "canceled"].includes(t.status),
    )) {
      await resumeUploadTask(task, true);
    }
  }

  async function handleBatchDelete() {
    const tasks = [...store.filteredUploadTasks.value];
    if (!tasks.length) return;
    const hasRemote = tasks.some((t) => !isLocalUploadTask(t));
    let deleteUploadedFile = false;
    if (hasRemote) {
      const successCount = tasks.filter((t) => t.status === "success").length;
      const r = await confirmBatchUploadTaskDelete(tasks.length, successCount);
      if (!r) return;
      deleteUploadedFile = r.checked;
    }
    for (const task of tasks.filter(isLocalUploadTask)) {
      await handleDeleteUploadTask(task, { silent: true, skipDialog: true });
    }
    const remote = tasks.filter((t) => !isLocalUploadTask(t));
    if (remote.length) {
      remote.forEach((t) => store.hiddenUploadTaskKeys.add(getUploadTaskStableKey(t)));
      try {
        await uploadApi.batchDelete(
          remote.map((t) => t.task_id),
          deleteUploadedFile,
        );
        await stream.fetchUploadTasks();
        if (deleteUploadedFile) await deps.loadFiles({ forceRefresh: true, silent: true });
      } catch (e) {
        remote.forEach((t) => store.hiddenUploadTaskKeys.delete(getUploadTaskStableKey(t)));
        toast.error(getApiErrorMessage(e, "批量删除失败"));
      }
    }
  }

  return {
    pauseUploadTask,
    resumeUploadTask,
    handleDeleteUploadTask,
    handleUploadTaskPrimaryAction,
    handleBatchToggle,
    handleBatchDelete,
  };
}

export function useUploadRelayActions(ctx: UploadActionsCtx) {
  const { batchDeleteRelayTasks } = ctx.deps.relay;

  async function handleDeleteRelayTask(task: { task_id: string }) {
    try {
      await batchDeleteRelayTasks([task.task_id]);
      toast.success("跨盘任务已删除");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "删除跨盘任务失败");
    }
  }

  async function handleBatchDeleteRelayTasks() {
    const ids = (ctx.store.filteredRelayTasks.value as Array<{ task_id: string }>).map((t) => t.task_id);
    if (!ids.length) return;
    try {
      await batchDeleteRelayTasks(ids);
      toast.success("跨盘任务已删除");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "删除跨盘任务失败");
    }
  }

  return { handleDeleteRelayTask, handleBatchDeleteRelayTasks };
}
