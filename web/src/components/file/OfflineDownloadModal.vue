<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { offlineDownloadApi } from "@/api/offlineDownload";
import { getApiErrorMessage } from "@/api/client";
import { toast } from "@/composables/useToast";
import { formatSize } from "@/utils/format";
import type { OfflineDownloadCapabilities, OfflineDownloadTask, OfflineTorrentPreparation } from "@/types/offline-download";
import type { Crumb } from "@/stores/browser";
import AppModal from "@/components/base/AppModal.vue";
import AppButton from "@/components/base/AppButton.vue";
import AppInput from "@/components/base/AppInput.vue";
import SvgIcon from "@/components/icons/SvgIcon.vue";
import FolderPickerModal from "./FolderPickerModal.vue";

const props = defineProps<{
  open: boolean;
  accountId: number | null;
  accountName: string;
  capability: OfflineDownloadCapabilities | null;
  currentParentId: string;
  currentDisplayPath: string;
  breadcrumb: Crumb[];
}>();

const emit = defineEmits<{
  close: [];
  created: [tasks: OfflineDownloadTask[]];
}>();

const sourceMode = ref<"url" | "bt">("url");
const urlText = ref("");
const fileName = ref("");
const targetParentId = ref("");
const targetDisplayPath = ref("/");
const folderPickerOpen = ref(false);
const submitting = ref(false);
const parsingTorrent = ref(false);
const torrentPreparation = ref<OfflineTorrentPreparation | null>(null);
const selectedTorrentIndexes = ref<number[]>([]);
const torrentInput = ref<HTMLInputElement | null>(null);

const supportsTorrent = computed(() => Boolean(props.capability?.supports_torrent));
const urlLines = computed(() =>
  urlText.value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean),
);
const selectedTorrentFiles = computed(() => {
  const selected = new Set(selectedTorrentIndexes.value);
  return torrentPreparation.value?.files.filter((file) => selected.has(file.index)) ?? [];
});
const selectedTorrentSize = computed(() =>
  selectedTorrentFiles.value.reduce((sum, file) => sum + file.size, 0),
);
const allTorrentSelected = computed(
  () => Boolean(torrentPreparation.value?.files.length) && selectedTorrentFiles.value.length === torrentPreparation.value?.files.length,
);
const submitDisabled = computed(() => {
  if (submitting.value || !props.accountId) return true;
  if (sourceMode.value === "bt") return !torrentPreparation.value || selectedTorrentIndexes.value.length === 0;
  return urlLines.value.length === 0;
});

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    sourceMode.value = "url";
    urlText.value = "";
    fileName.value = "";
    torrentPreparation.value = null;
    selectedTorrentIndexes.value = [];
    initTarget();
  },
  { immediate: true },
);

function initTarget() {
  const currentIsRoot = !props.currentParentId || props.currentParentId === "0";
  if (currentIsRoot && props.capability && !props.capability.root_target_allowed) {
    targetParentId.value = "";
    targetDisplayPath.value = "来自:离线下载（网盘默认目录）";
    return;
  }
  targetParentId.value = props.currentParentId;
  targetDisplayPath.value = props.currentDisplayPath || "/";
}

function targetResolved(payload: { parentId: string; path: string }) {
  folderPickerOpen.value = false;
  if ((payload.parentId === "" || payload.parentId === "0") && props.capability && !props.capability.root_target_allowed) {
    targetParentId.value = "";
    targetDisplayPath.value = "来自:离线下载（网盘默认目录）";
    return;
  }
  targetParentId.value = payload.parentId;
  targetDisplayPath.value = payload.path || "/";
}

async function prepareTorrent(file: File | undefined) {
  if (!file || !props.accountId) return;
  if (!file.name.toLowerCase().endsWith(".torrent")) {
    toast.info("请选择 .torrent 种子文件");
    return;
  }
  parsingTorrent.value = true;
  try {
    const result = await offlineDownloadApi.prepareTorrent(props.accountId, file);
    torrentPreparation.value = result;
    selectedTorrentIndexes.value = result.files.filter((item) => item.wanted).map((item) => item.index);
    if (!selectedTorrentIndexes.value.length) {
      selectedTorrentIndexes.value = result.files.map((item) => item.index);
    }
  } catch (error) {
    toast.error(getApiErrorMessage(error, "BT 种子解析失败"));
  } finally {
    parsingTorrent.value = false;
    if (torrentInput.value) torrentInput.value.value = "";
  }
}

function onTorrentInput(event: Event) {
  void prepareTorrent((event.target as HTMLInputElement).files?.[0]);
}

function onTorrentDrop(event: DragEvent) {
  void prepareTorrent(event.dataTransfer?.files?.[0]);
}

function toggleTorrentFile(index: number) {
  const selected = new Set(selectedTorrentIndexes.value);
  if (selected.has(index)) selected.delete(index);
  else selected.add(index);
  selectedTorrentIndexes.value = [...selected];
}

function toggleAllTorrentFiles() {
  if (!torrentPreparation.value) return;
  selectedTorrentIndexes.value = allTorrentSelected.value
    ? []
    : torrentPreparation.value.files.map((item) => item.index);
}

async function submit() {
  if (!props.accountId || submitDisabled.value) return;
  submitting.value = true;
  try {
    if (sourceMode.value === "bt") {
      const prep = torrentPreparation.value;
      if (!prep) return;
      const task = await offlineDownloadApi.addTorrent({
        account_id: props.accountId,
        preparation_id: prep.preparation_id,
        wanted: selectedTorrentIndexes.value,
        target_parent_id: targetParentId.value,
        target_display_path: targetDisplayPath.value,
      });
      emit("created", [task]);
      toast.success("BT 离线下载任务已提交");
    } else {
      const tasks = await offlineDownloadApi.addURLs({
        account_id: props.accountId,
        urls: urlLines.value,
        file_name: fileName.value.trim() || undefined,
        target_parent_id: targetParentId.value,
        target_display_path: targetDisplayPath.value,
      });
      emit("created", tasks);
      const failed = tasks.filter((task) => task.status === "failed").length;
      if (failed) toast.warning(`${tasks.length - failed} 个任务提交成功，${failed} 个失败`);
      else toast.success(`${tasks.length} 个离线下载任务已提交`);
    }
    emit("close");
  } catch (error) {
    toast.error(getApiErrorMessage(error, "离线下载任务提交失败"));
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <AppModal :open="open" title="新建离线下载" size="lg" @close="emit('close')">
    <div class="offline-download-form">
      <div class="offline-capability">
        <span class="offline-capability__icon"><SvgIcon name="cloud" :size="24" /></span>
        <span class="offline-capability__body">
          <strong v-if="supportsTorrent">{{ accountName }}支持链接和 BT 种子任务</strong>
          <strong v-else>{{ accountName }}支持 HTTP/HTTPS 离线下载</strong>
          <small v-if="supportsTorrent">链接可以批量提交；BT 种子解析后可选择下载内容。</small>
          <small v-else>官方接口一次创建一个任务，根目录会使用网盘默认的“来自:离线下载”。</small>
        </span>
      </div>

      <div v-if="supportsTorrent" class="offline-source-tabs">
        <button type="button" :class="{ active: sourceMode === 'url' }" @click="sourceMode = 'url'">
          <SvgIcon name="fa-link" :size="15" /> 链接任务
        </button>
        <button type="button" :class="{ active: sourceMode === 'bt' }" @click="sourceMode = 'bt'">
          <SvgIcon name="file" :size="15" /> BT 种子
        </button>
      </div>

      <template v-if="sourceMode === 'url'">
        <label class="offline-field">
          <span class="offline-field__label">下载链接</span>
          <textarea
            v-model="urlText"
            class="offline-textarea"
            :placeholder="capability?.supports_batch_urls ? '一行一个链接，可批量提交' : '请输入一个 HTTP/HTTPS 下载链接'"
            :rows="capability?.supports_batch_urls ? 5 : 3"
          />
          <small>
            支持 {{ capability?.url_schemes.map((item) => item.toUpperCase()).join(" / ") }}
            <template v-if="capability?.supports_batch_urls">，当前 {{ urlLines.length }} 条</template>
          </small>
        </label>
        <label v-if="!capability?.supports_batch_urls" class="offline-field">
          <span class="offline-field__label">自定义文件名 <em>可选</em></span>
          <AppInput v-model="fileName" placeholder="留空时由 123 云盘识别文件名" />
        </label>
      </template>

      <template v-else>
        <div
          v-if="!torrentPreparation"
          class="offline-torrent-drop"
          :class="{ loading: parsingTorrent }"
          @dragover.prevent
          @drop.prevent="onTorrentDrop"
          @click="!parsingTorrent && torrentInput?.click()"
        >
          <SvgIcon name="file" :size="30" />
          <strong>{{ parsingTorrent ? "正在上传并解析种子…" : "选择 .torrent 种子文件" }}</strong>
          <small>也可以把种子文件拖到这里，最大 16 MiB</small>
          <input ref="torrentInput" type="file" accept=".torrent,application/x-bittorrent" hidden @change="onTorrentInput" />
        </div>

        <div v-else class="offline-torrent-result">
          <div class="offline-torrent-summary">
            <span class="offline-torrent-summary__body">
              <strong :title="torrentPreparation.torrent_name">{{ torrentPreparation.torrent_name }}</strong>
              <small>{{ torrentPreparation.files.length }} 个文件 · {{ formatSize(torrentPreparation.total_size) }}</small>
            </span>
            <button type="button" @click="torrentPreparation = null; selectedTorrentIndexes = []">重新选择</button>
          </div>
          <div class="offline-torrent-files">
            <label class="offline-torrent-file offline-torrent-file--head">
              <input type="checkbox" :checked="allTorrentSelected" @change="toggleAllTorrentFiles" />
              <span>选择下载内容</span><span>大小</span>
            </label>
            <label v-for="file in torrentPreparation.files" :key="file.index" class="offline-torrent-file">
              <input
                type="checkbox"
                :checked="selectedTorrentIndexes.includes(file.index)"
                @change="toggleTorrentFile(file.index)"
              />
              <span :title="file.path">{{ file.path }}</span><span>{{ formatSize(file.size) }}</span>
            </label>
          </div>
          <div class="offline-torrent-selected">
            已选择 {{ selectedTorrentFiles.length }} 个文件，共 {{ formatSize(selectedTorrentSize) }}
          </div>
        </div>
      </template>

      <div class="offline-target">
        <span class="offline-target__icon"><SvgIcon name="folder" :size="22" /></span>
        <span class="offline-target__body"><small>保存位置</small><strong :title="targetDisplayPath">{{ targetDisplayPath }}</strong></span>
        <AppButton size="sm" @click="folderPickerOpen = true">更改目录</AppButton>
      </div>
      <div class="offline-target-help">任务完成后会自动刷新这个目录的文件缓存。</div>
    </div>

    <template #footer>
      <AppButton variant="cancel" @click="emit('close')">取消</AppButton>
      <AppButton variant="primary" :disabled="submitDisabled" @click="submit">
        <SvgIcon name="cloud" :size="17" />
        {{ submitting ? "正在提交…" : "开始离线下载" }}
      </AppButton>
    </template>
  </AppModal>

  <FolderPickerModal
    :open="folderPickerOpen"
    title="选择离线下载目录"
    confirm-text="保存到当前目录"
    :account-id="accountId"
    :allow-create-folder="true"
    :show-refresh="false"
    :initial-breadcrumb="breadcrumb"
    @resolve="targetResolved"
    @close="folderPickerOpen = false"
  />
</template>

<style scoped>
.offline-download-form { display: flex; flex-direction: column; gap: 16px; }
.offline-capability { display: flex; align-items: center; gap: 12px; padding: 13px 14px; border: 1px solid var(--tab-active-border); border-radius: 10px; background: var(--info-soft); }
.offline-capability__icon { width: 42px; height: 42px; display: inline-flex; align-items: center; justify-content: center; border-radius: 10px; background: var(--surface); color: var(--brand); }
.offline-capability__body { min-width: 0; display: flex; flex-direction: column; gap: 4px; }
.offline-capability__body strong { color: var(--text); font-size: 14px; }
.offline-capability__body small, .offline-field small, .offline-target-help { color: var(--text-muted); font-size: 12px; }
.offline-source-tabs { display: flex; gap: 8px; border-bottom: 1px solid var(--border-soft); }
.offline-source-tabs button { display: inline-flex; align-items: center; gap: 6px; padding: 9px 12px; border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--text-muted); font-weight: 600; cursor: pointer; }
.offline-source-tabs button.active { border-bottom-color: var(--brand); color: var(--brand); }
.offline-field { display: flex; flex-direction: column; gap: 8px; }
.offline-field__label { color: var(--text); font-size: 13px; font-weight: 600; }
.offline-field__label em { color: var(--text-muted); font-size: 11px; font-style: normal; font-weight: 400; }
.offline-textarea { width: 100%; resize: vertical; min-height: 92px; padding: 11px 12px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); color: var(--text); font: inherit; line-height: 1.6; box-sizing: border-box; }
.offline-textarea:focus { outline: none; border-color: var(--brand); }
.offline-torrent-drop { min-height: 148px; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 9px; border: 1px dashed var(--brand); border-radius: 11px; background: var(--surface-sunken); color: var(--brand); cursor: pointer; }
.offline-torrent-drop strong { color: var(--text); }
.offline-torrent-drop small { color: var(--text-muted); }
.offline-torrent-drop.loading { cursor: wait; opacity: .75; }
.offline-torrent-result { display: flex; flex-direction: column; gap: 9px; }
.offline-torrent-summary { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 11px 12px; border: 1px solid var(--border); border-radius: 9px; background: var(--surface-sunken); }
.offline-torrent-summary__body { min-width: 0; display: flex; flex-direction: column; gap: 3px; }
.offline-torrent-summary__body strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text); }
.offline-torrent-summary__body small { color: var(--text-muted); }
.offline-torrent-summary button { flex-shrink: 0; border: 0; background: transparent; color: var(--brand); cursor: pointer; }
.offline-torrent-files { max-height: 220px; overflow: auto; border: 1px solid var(--border); border-radius: 9px; }
.offline-torrent-file { display: grid; grid-template-columns: 24px minmax(0, 1fr) 88px; align-items: center; gap: 8px; min-height: 40px; padding: 0 11px; border-top: 1px solid var(--border-soft); color: var(--text-regular); font-size: 12px; }
.offline-torrent-file:first-child { border-top: 0; }
.offline-torrent-file > span:nth-child(2) { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.offline-torrent-file > span:last-child { text-align: right; color: var(--text-muted); }
.offline-torrent-file--head { background: var(--surface-sunken); color: var(--text-muted); }
.offline-torrent-selected { text-align: right; color: var(--text-muted); font-size: 12px; }
.offline-target { display: flex; align-items: center; gap: 10px; padding: 11px 12px; border: 1px solid var(--border); border-radius: 10px; background: var(--surface-sunken); }
.offline-target__icon { color: var(--brand); }
.offline-target__body { min-width: 0; flex: 1; display: flex; flex-direction: column; gap: 2px; }
.offline-target__body small { color: var(--text-muted); }
.offline-target__body strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text); font-size: 13px; }
.offline-target-help { margin-top: -10px; padding-left: 4px; }
@media (max-width: 640px) {
  .offline-target { align-items: flex-start; flex-wrap: wrap; }
  .offline-target .btn { width: 100%; }
  .offline-torrent-file { grid-template-columns: 22px minmax(0, 1fr) 70px; }
}
</style>
