<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { coverExtractApi, type CoverFile, type CoverFrame, type CoverRuntime } from "@/api/coverExtract";
import { getApiErrorMessage } from "@/api/client";
import { toast } from "@/composables/useToast";
import { canvasToJPEG, createCoverPoster, type CoverPosterFocus } from "@/utils/coverPoster";
import AppButton from "@/components/base/AppButton.vue";
import AppModal from "@/components/base/AppModal.vue";
import AccountFolderField from "@/components/admin/AccountFolderField.vue";
import CloudToolCard from "@/components/admin/CloudToolCard.vue";
import FolderPickerModal from "@/components/file/FolderPickerModal.vue";
import AppSelect from "@/components/base/AppSelect.vue";

type CaptureMode = "uniform" | "head_tail" | "timestamp";
const panelShapeOptions = [
  { value: "slant", label: "斜边" },
  { value: "straight", label: "直边" },
];

const props = withDefaults(defineProps<{ searchQuery?: string }>(), { searchQuery: "" });
const open = ref(false);
const loading = ref(false);
const downloading = ref(false);
const saving = ref(false);
const previewing = ref(false);
const toggleSaving = ref(false);
const targetPickerOpen = ref(false);
const captureMode = ref<CaptureMode>("uniform");
const files = ref<CoverFile[]>([]);
const runtime = ref<CoverRuntime | null>(null);
const activeID = ref("");
const selectedFrame = ref("");
const timeHour = ref(0);
const timeMinute = ref(0);
const timeSecond = ref(0);
const statusText = ref("");
const previewError = ref("");
const posterCanvas = ref<HTMLCanvasElement | null>(null);
const titles = ref<Record<string, string>>({});
const packaged = ref<Record<string, boolean>>({});
const panelColors = ref<Record<string, string>>({});
const panelOpacities = ref<Record<string, number>>({});
const textColors = ref<Record<string, string>>({});
const panelShapes = ref<Record<string, "slant" | "straight">>({});
const panelHeights = ref<Record<string, number>>({});
const imageZooms = ref<Record<string, number>>({});
const frameFocuses = ref<Record<string, CoverPosterFocus>>({});
const draggingPreview = ref(false);
let previewTicket = 0;
let previewAnimationFrame = 0;
let dragState: {
  pointerID: number;
  frameID: string;
  startX: number;
  startY: number;
  startFocus: CoverPosterFocus;
} | null = null;

const active = computed(() => files.value.find((file) => file.id === activeID.value) ?? files.value[0]);
const enabled = computed(() => runtime.value?.enabled ?? false);
const targetDisplay = computed(() => active.value ? `${active.value.target_path === "/" ? "" : active.value.target_path}/poster.jpg` : "/poster.jpg");
const visible = computed(() => !props.searchQuery.trim() || "视频海报生成封面提取".includes(props.searchQuery.trim()));
const selectedFrameInfo = computed(() => active.value?.frames.find((frame) => frame.id === selectedFrame.value));
const activeTitle = computed({
  get: () => active.value ? (titles.value[active.value.id] ?? "") : "",
  set: (value: string) => {
    if (active.value) titles.value[active.value.id] = value;
  },
});
const activePackaged = computed({
  get: () => active.value ? (packaged.value[active.value.id] ?? false) : false,
  set: (value: boolean) => {
    if (active.value) packaged.value[active.value.id] = value;
  },
});
const activePanelColor = computed({
  get: () => active.value ? (panelColors.value[active.value.id] ?? "#000000") : "#000000",
  set: (value: string) => {
    if (active.value) panelColors.value[active.value.id] = value;
  },
});
const activePanelOpacity = computed({
  get: () => active.value ? (panelOpacities.value[active.value.id] ?? 0.8) : 0.8,
  set: (value: number) => {
    if (active.value) panelOpacities.value[active.value.id] = Number(value);
  },
});
const activeTextColor = computed({
  get: () => active.value ? (textColors.value[active.value.id] ?? "#fffdf8") : "#fffdf8",
  set: (value: string) => {
    if (active.value) textColors.value[active.value.id] = value;
  },
});
const activePanelShape = computed({
  get: () => active.value ? (panelShapes.value[active.value.id] ?? "slant") : "slant",
  set: (value: "slant" | "straight") => {
    if (active.value) panelShapes.value[active.value.id] = value;
  },
});
const activePanelHeight = computed({
  get: () => active.value ? (panelHeights.value[active.value.id] ?? 0.22) : 0.22,
  set: (value: number) => {
    if (active.value) panelHeights.value[active.value.id] = Number(value);
  },
});
const activeImageZoom = computed({
  get: () => active.value ? (imageZooms.value[active.value.id] ?? 1) : 1,
  set: (value: number) => {
    if (active.value) imageZooms.value[active.value.id] = Number(value);
  },
});
const captureHint = computed(() => {
  if (captureMode.value === "uniform") return "按完整片长均匀选取 5 个时间点，适合快速挑选代表画面。";
  if (captureMode.value === "head_tail") return "从视频开头和结尾各取 1 张，共生成 2 张候选画面。";
  return "设置准确时间点，只提取该位置的 1 张候选画面。";
});
const captureActionLabel = computed(() => {
  if (captureMode.value === "uniform") return "提取五张";
  if (captureMode.value === "head_tail") return "提取首尾";
  return "按时间提取";
});

function fmtDuration(ms?: number) {
  if (!ms) return "待提取";
  const seconds = Math.floor(ms / 1000);
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remain = seconds % 60;
  return hours ? `${hours}:${String(minutes).padStart(2, "0")}:${String(remain).padStart(2, "0")}` : `${minutes}:${String(remain).padStart(2, "0")}`;
}

function fmtTimestamp(ms: number) {
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  return `${minutes}:${String(seconds % 60).padStart(2, "0")}`;
}

function inferTitle(file: CoverFile) {
  const targetName = file.target_path.split("/").filter(Boolean).at(-1)?.trim() ?? "";
  const genericDirectories = new Set(["电影", "电视剧", "剧集", "短剧", "视频", "movies", "movie", "tv"]);
  if (targetName && !genericDirectories.has(targetName.toLowerCase())) return targetName;
  const stem = file.name.replace(/\.[^.]+$/, "").replace(/[._]+/g, " ").trim();
  return stem
    .replace(/\s+(?:S\d{1,2}E\d{1,3}|E\d{1,3}|2160p|1080p|720p|WEB[- .]?DL|BluRay|HDTV)\b.*$/i, "")
    .trim() || stem;
}

function ensureFileOptions(file: CoverFile) {
  if (!(file.id in titles.value)) titles.value[file.id] = inferTitle(file);
  if (!(file.id in packaged.value)) packaged.value[file.id] = false;
  if (!(file.id in panelColors.value)) panelColors.value[file.id] = "#000000";
  if (!(file.id in panelOpacities.value)) panelOpacities.value[file.id] = 0.8;
  if (!(file.id in textColors.value)) textColors.value[file.id] = "#fffdf8";
  if (!(file.id in panelShapes.value)) panelShapes.value[file.id] = "slant";
  if (!(file.id in panelHeights.value)) panelHeights.value[file.id] = 0.22;
  if (!(file.id in imageZooms.value)) imageZooms.value[file.id] = 1;
}

function frameFocus(frameID = selectedFrame.value): CoverPosterFocus {
  return frameFocuses.value[frameID] ?? { x: 0.5, y: 0.5 };
}

function clampFocus(value: number) {
  return Math.min(1, Math.max(0, value));
}

function schedulePreview() {
  if (previewAnimationFrame) return;
  previewAnimationFrame = window.requestAnimationFrame(() => {
    previewAnimationFrame = 0;
    void refreshPreview(true);
  });
}

function setFrameFocus(frameID: string, focus: CoverPosterFocus, immediate = false) {
  frameFocuses.value = { ...frameFocuses.value, [frameID]: focus };
  if (immediate) void refreshPreview();
  else schedulePreview();
}

function normalizeFiles(list: CoverFile[]) {
  return (list ?? []).map((file) => {
    const normalized = { ...file, frames: file.frames ?? [] };
    ensureFileOptions(normalized);
    return normalized;
  });
}

function ensureActiveSelection() {
  const current = active.value;
  if (!current) {
    selectedFrame.value = "";
    return;
  }
  if (!current.frames.some((frame) => frame.id === selectedFrame.value)) {
    selectedFrame.value = current.frames[0]?.id ?? "";
  }
}

async function load() {
  try {
    const [list, rt] = await Promise.all([coverExtractApi.files(), coverExtractApi.runtime()]);
    files.value = normalizeFiles(list.files ?? []);
    runtime.value = rt;
    if (!files.value.some((file) => file.id === activeID.value)) activeID.value = files.value[0]?.id ?? "";
    ensureActiveSelection();
  } catch (error) {
    toast.error(getApiErrorMessage(error, "加载视频海报生成工具失败"));
  }
}

async function show() {
  open.value = true;
  await load();
}

async function toggleEnabled() {
  toggleSaving.value = true;
  try {
    runtime.value = await coverExtractApi.setEnabled(!enabled.value);
    toast.success(runtime.value.enabled ? "已启用视频海报生成，视频右键菜单已开放" : "已停用视频海报生成，视频右键菜单已隐藏");
  } catch (error) {
    toast.error(getApiErrorMessage(error, "修改开关失败"));
  } finally {
    toggleSaving.value = false;
  }
}

async function downloadTool() {
  downloading.value = true;
  statusText.value = "正在下载、校验并安装 FFmpeg…";
  try {
    runtime.value = await coverExtractApi.download();
    toast.success("FFmpeg 安装完成");
  } catch (error) {
    toast.error(getApiErrorMessage(error, "FFmpeg 安装失败"));
  } finally {
    downloading.value = false;
    statusText.value = "";
  }
}

async function extract(mode: CaptureMode) {
  if (!active.value) return;
  const sessionID = active.value.id;
  const previousFrames = new Set(active.value.frames.map((frame) => frame.id));
  loading.value = true;
  statusText.value = mode === "uniform" ? "正在读取视频信息并生成 5 张候选画面…" : mode === "head_tail" ? "正在提取片头与片尾 2 张候选画面…" : "正在提取指定时间的画面…";
  try {
    const timestampMs = (timeHour.value * 3600 + timeMinute.value * 60 + timeSecond.value) * 1000;
    const out = await coverExtractApi.extract({ session_file_id: sessionID, mode, count: mode === "uniform" ? 5 : undefined, timestamp_ms: timestampMs });
    ensureFileOptions(out);
    files.value = files.value.map((file) => file.id === out.id ? { ...out, frames: out.frames ?? [] } : file);
    const firstNewFrame = out.frames.find((frame) => !previousFrames.has(frame.id));
    selectedFrame.value = firstNewFrame?.id ?? out.frames[0]?.id ?? "";
    statusText.value = `当前视频已生成 ${out.frames.length} 张候选图`;
  } catch (error) {
    toast.error(getApiErrorMessage(error, "提取失败"));
    await load();
    statusText.value = "";
  } finally {
    loading.value = false;
  }
}

async function buildPoster() {
  if (!selectedFrameInfo.value) throw new Error("请先选择候选画面");
  return createCoverPoster({
    imageURL: coverExtractApi.imageURL(selectedFrameInfo.value.id),
    title: activeTitle.value,
    packaged: activePackaged.value,
    focus: frameFocus(selectedFrameInfo.value.id),
    panelColor: activePanelColor.value,
    panelOpacity: activePanelOpacity.value,
    textColor: activeTextColor.value,
    panelShape: activePanelShape.value,
    panelHeight: activePanelHeight.value,
    imageZoom: activeImageZoom.value,
  });
}

function startPreviewDrag(event: PointerEvent) {
  if (event.button !== 0 || previewing.value || !selectedFrameInfo.value) return;
  const element = event.currentTarget as HTMLElement;
  dragState = {
    pointerID: event.pointerId,
    frameID: selectedFrameInfo.value.id,
    startX: event.clientX,
    startY: event.clientY,
    startFocus: { ...frameFocus(selectedFrameInfo.value.id) },
  };
  draggingPreview.value = true;
  element.setPointerCapture(event.pointerId);
  event.preventDefault();
}

function movePreviewDrag(event: PointerEvent) {
  if (!dragState || dragState.pointerID !== event.pointerId) return;
  const element = event.currentTarget as HTMLElement;
  const bounds = element.getBoundingClientRect();
  if (!bounds.width || !bounds.height) return;
  setFrameFocus(dragState.frameID, {
    x: clampFocus(dragState.startFocus.x - (event.clientX - dragState.startX) / bounds.width),
    y: clampFocus(dragState.startFocus.y - (event.clientY - dragState.startY) / bounds.height),
  });
  event.preventDefault();
}

function finishPreviewDrag(event: PointerEvent) {
  if (!dragState || dragState.pointerID !== event.pointerId) return;
  const element = event.currentTarget as HTMLElement;
  if (element.hasPointerCapture(event.pointerId)) element.releasePointerCapture(event.pointerId);
  dragState = null;
  draggingPreview.value = false;
}

function resetPreviewFocus() {
  if (!selectedFrameInfo.value) return;
  setFrameFocus(selectedFrameInfo.value.id, { x: 0.5, y: 0.5 }, true);
}

async function refreshPreview(silent = false) {
  const ticket = ++previewTicket;
  previewError.value = "";
  if (!open.value || !selectedFrameInfo.value) return;
  if (!silent) previewing.value = true;
  try {
    const rendered = await buildPoster();
    if (ticket !== previewTicket) return;
    await nextTick();
    const canvas = posterCanvas.value;
    const ctx = canvas?.getContext("2d");
    if (!canvas || !ctx) return;
    canvas.width = rendered.width;
    canvas.height = rendered.height;
    ctx.drawImage(rendered, 0, 0);
  } catch (error) {
    if (ticket === previewTicket) previewError.value = error instanceof Error ? error.message : "海报预览生成失败";
  } finally {
    if (ticket === previewTicket && !silent) previewing.value = false;
  }
}

async function save() {
  if (!active.value || !selectedFrame.value) return;
  if (activePackaged.value && !activeTitle.value.trim()) {
    toast.error("请先填写海报片名");
    return;
  }
  saving.value = true;
  statusText.value = "正在合成 1000×1500 海报…";
  try {
    const canvas = await buildPoster();
    const blob = await canvasToJPEG(canvas);
    statusText.value = `正在保存到 ${targetDisplay.value}…`;
    const payload = { session_file_id: active.value.id, frame_id: selectedFrame.value, overwrite: false };
    let out = await coverExtractApi.saveComposed(payload, blob);
    if (out.conflict) {
      if (!window.confirm(`${out.filename} 已存在，确定覆盖吗？`)) {
        statusText.value = "已取消保存";
        return;
      }
      out = await coverExtractApi.saveComposed({ ...payload, overwrite: true }, blob);
    }
    if (out.ok) {
      toast.success(`已保存到 ${targetDisplay.value}`);
      statusText.value = "封面保存完成";
    }
  } catch (error) {
    toast.error(getApiErrorMessage(error, error instanceof Error ? error.message : "保存封面失败"));
    statusText.value = "";
  } finally {
    saving.value = false;
  }
}

async function remove(id: string) {
  try {
    const removedFrameIDs = files.value.find((file) => file.id === id)?.frames.map((frame) => frame.id) ?? [];
    await coverExtractApi.remove(id);
    files.value = files.value.filter((file) => file.id !== id);
    const nextTitles = { ...titles.value };
    const nextPackaged = { ...packaged.value };
    const nextPanelColors = { ...panelColors.value };
    const nextPanelOpacities = { ...panelOpacities.value };
    const nextTextColors = { ...textColors.value };
    const nextPanelShapes = { ...panelShapes.value };
    const nextPanelHeights = { ...panelHeights.value };
    const nextImageZooms = { ...imageZooms.value };
    delete nextTitles[id];
    delete nextPackaged[id];
    delete nextPanelColors[id];
    delete nextPanelOpacities[id];
    delete nextTextColors[id];
    delete nextPanelShapes[id];
    delete nextPanelHeights[id];
    delete nextImageZooms[id];
    titles.value = nextTitles;
    packaged.value = nextPackaged;
    panelColors.value = nextPanelColors;
    panelOpacities.value = nextPanelOpacities;
    textColors.value = nextTextColors;
    panelShapes.value = nextPanelShapes;
    panelHeights.value = nextPanelHeights;
    imageZooms.value = nextImageZooms;
    if (removedFrameIDs.length) {
      const nextFocuses = { ...frameFocuses.value };
      removedFrameIDs.forEach((frameID) => delete nextFocuses[frameID]);
      frameFocuses.value = nextFocuses;
    }
    if (activeID.value === id) activeID.value = files.value[0]?.id ?? "";
    ensureActiveSelection();
  } catch (error) {
    toast.error(getApiErrorMessage(error, "移除失败"));
  }
}

async function setTarget(payload: { parentId: string; path: string }) {
  if (!active.value) return;
  try {
    const out = await coverExtractApi.setTarget(active.value.id, { parent_id: payload.parentId, path: payload.path || "/" });
    files.value = files.value.map((file) => file.id === out.id ? { ...out, frames: out.frames ?? [] } : file);
    targetPickerOpen.value = false;
  } catch (error) {
    toast.error(getApiErrorMessage(error, "修改封面保存目录失败"));
  }
}

function openTargetPicker() {
  if (!active.value || loading.value || saving.value) return;
  targetPickerOpen.value = true;
}

function select(file: CoverFile) {
  activeID.value = file.id;
  selectedFrame.value = file.frames[0]?.id ?? "";
}

function choose(frame: CoverFrame) {
  selectedFrame.value = frame.id;
}

watch([selectedFrame, activeTitle, activePackaged, activePanelColor, activePanelOpacity, activeTextColor, activePanelShape, activePanelHeight, activeImageZoom, open], () => void refreshPreview(), { flush: "post" });
onMounted(load);
onUnmounted(() => {
  if (previewAnimationFrame) window.cancelAnimationFrame(previewAnimationFrame);
});
</script>

<template>
  <CloudToolCard v-show="visible" :enabled="enabled" name="视频海报生成" driver="视频截图· 保存到网盘" logo-src="/logos/CoverExtract.png" logo-alt="视频海报生成" :stat-value="files.length" stat-label="个待处理视频">
    启用后，通过首页文件列表右键[生成视频海报]将待处理文件发送至此，适用于无法刮削作品生成海报。
    <template #toggle><button class="check-toggle" type="button" :class="{ on: enabled }" :aria-label="enabled ? '停用视频海报生成' : '启用视频海报生成'" :disabled="toggleSaving" @click="toggleEnabled"><svg viewBox="0 0 16 16" aria-hidden="true"><path d="M3.5 8.5 6.5 11.5 12.5 4.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" /></svg></button></template>
    <template #actions><AppButton size="sm" @click="show">打开工具</AppButton></template>
  </CloudToolCard>

  <AppModal :open="open" title="视频海报生成" size="lg" @close="open = false">
    <div v-if="runtime && !runtime.ready" class="cover-warning">
      {{ runtime.error }}<br>请将组件放到 {{ runtime.manual_path }}
      <AppButton v-if="runtime.auto_download_available" size="sm" :disabled="downloading" @click="downloadTool">{{ downloading ? "安装中…" : "自动安装" }}</AppButton>
    </div>
    <div class="cover-layout">
      <aside class="cover-files">
        <div v-for="file in files" :key="file.id" class="cover-file" :class="{ active: file.id === active?.id }">
          <button type="button" class="cover-file__select" @click="select(file)"><span>{{ file.name }}</span><small>{{ fmtDuration(file.duration_ms) }} · {{ file.frames.length ? `${file.frames.length} 张候选图` : file.status }}</small></button>
          <button type="button" class="cover-file__remove" aria-label="移除视频及候选图" @click="remove(file.id)">×</button>
        </div>
        <p v-if="!files.length">请在文件管理中右键视频，发送到视频海报生成工具。</p>
      </aside>

      <section class="cover-main">
        <div v-if="active" class="cover-capture-panel">
          <div class="cover-capture-tabs" role="tablist" aria-label="选择取帧方式">
            <button type="button" role="tab" :aria-selected="captureMode === 'uniform'" :class="{ active: captureMode === 'uniform' }" @click="captureMode = 'uniform'">
              <strong>均匀取帧五张</strong>
            </button>
            <button type="button" role="tab" :aria-selected="captureMode === 'head_tail'" :class="{ active: captureMode === 'head_tail' }" @click="captureMode = 'head_tail'">
              <strong>片头片尾取帧</strong>
            </button>
            <button type="button" role="tab" :aria-selected="captureMode === 'timestamp'" :class="{ active: captureMode === 'timestamp' }" @click="captureMode = 'timestamp'">
              <strong>按时间取帧</strong>
            </button>
          </div>
          <div class="cover-capture-run">
            <span class="cover-capture-hint">{{ captureHint }}</span>
            <div v-if="captureMode === 'timestamp'" class="cover-time"><input v-model.number="timeHour" aria-label="小时" type="number" min="0"><span>时</span><input v-model.number="timeMinute" aria-label="分钟" type="number" min="0" max="59"><span>分</span><input v-model.number="timeSecond" aria-label="秒" type="number" min="0" max="59"><span>秒</span></div>
            <AppButton size="sm" :disabled="loading || !enabled || !runtime?.ready" @click="extract(captureMode)">{{ captureActionLabel }}</AppButton>
          </div>
        </div>
        <div v-if="statusText" class="cover-status"><span v-if="loading || downloading || saving" class="cover-spinner" />{{ statusText }}</div>

        <div v-if="active?.frames.length" class="cover-workspace">
          <div class="cover-candidates">
            <div class="cover-section-title"><strong>候选画面</strong><span>保留原视频比例</span></div>
            <div class="cover-grid">
              <button v-for="frame in active.frames" :key="frame.id" type="button" :class="{ selected: selectedFrame === frame.id }" @click="choose(frame)">
                <img :src="coverExtractApi.imageURL(frame.id)" loading="lazy">
                <span>{{ fmtTimestamp(frame.time_ms) }}</span>
              </button>
            </div>
          </div>

          <div class="cover-preview-pane">
            <div class="cover-section-title"><strong>海报预览</strong><span>拖动画面 · 双击复位</span></div>
            <div
              class="cover-preview-frame"
              :class="{ dragging: draggingPreview }"
              title="拖动调整画面位置，双击恢复居中"
              @pointerdown="startPreviewDrag"
              @pointermove="movePreviewDrag"
              @pointerup="finishPreviewDrag"
              @pointercancel="finishPreviewDrag"
              @dblclick.prevent="resetPreviewFocus"
            >
              <canvas ref="posterCanvas" :class="{ loading: previewing }" />
              <label class="cover-preview-zoom" title="放大画面后可拖动调整位置" @pointerdown.stop @dblclick.stop>
                <small>{{ Math.round(activeImageZoom * 100) }}%</small>
                <input v-model.number="activeImageZoom" aria-label="画面缩放" type="range" min="1" max="1.5" step="0.01" orient="vertical">
              </label>
              <span v-if="previewing" class="cover-preview-loading"><i class="cover-spinner" />正在合成预览…</span>
            </div>
            <p v-if="previewError" class="cover-error">{{ previewError }}</p>
          </div>

        </div>
        <div v-else-if="active" class="cover-empty">取帧后可在这里选择画面并实时预览海报。</div>
        <p v-if="active?.error" class="cover-error">{{ active.error }}</p>
      </section>
    </div>
    <div v-if="active?.frames.length" class="cover-package-controls">
      <div class="cover-package-head">
        <label class="cover-package-toggle"><input v-model="activePackaged" type="checkbox"><strong>包装海报</strong></label>
        <label v-if="activePackaged" class="cover-title-input"><span>片名</span><input v-model="activeTitle" maxlength="16" type="text" placeholder="最多 16 个字"></label>
      </div>
      <div v-if="activePackaged" class="cover-package-options">
        <div class="cover-style-controls">
          <label class="cover-edge"><span>边缘</span><AppSelect v-model="activePanelShape" :options="panelShapeOptions" /></label>
          <label class="cover-height" title="底部形状高度"><span>高度</span><input v-model.number="activePanelHeight" type="range" min="0.15" max="0.3" step="0.01"><small>{{ Math.round(activePanelHeight * 100) }}%</small></label>
          <label title="底部色块颜色"><span>底色</span><input v-model="activePanelColor" type="color"></label>
          <label class="cover-opacity" title="设为 0% 时仅保留片名"><span>透明度</span><input v-model.number="activePanelOpacity" type="range" min="0" max="1" step="0.05"><small>{{ Math.round(activePanelOpacity * 100) }}%</small></label>
          <label title="片名颜色"><span>字色</span><input v-model="activeTextColor" type="color"></label>
        </div>
      </div>
    </div>
    <template #footer>
      <div v-if="active" class="cover-footer">
        <AccountFolderField class="cover-footer__path" :display="targetDisplay" :title="`封面保存到 ${targetDisplay}`" browse-label="选择目录" @browse="openTargetPicker" />
        <AppButton variant="primary" :disabled="!enabled || !selectedFrame || saving || loading || previewing" @click="save">{{ saving ? "保存中…" : "保存封面" }}</AppButton>
      </div>
    </template>
  </AppModal>
  <FolderPickerModal :open="targetPickerOpen" nested :account-id="active?.account_id ?? null" :initial-path="active?.target_path ?? '/'" title="选择封面保存目录" confirm-text="保存到此目录" @close="targetPickerOpen = false" @resolve="setTarget" />
</template>

<style scoped>
.check-toggle{width:28px;height:28px;border-radius:50%;border:0;padding:0;flex-shrink:0;display:inline-flex;align-items:center;justify-content:center;cursor:pointer;background:var(--border);color:var(--text-muted);transition:background .18s ease,color .18s ease,box-shadow .18s ease}.check-toggle svg{width:14px;height:14px}.check-toggle:hover{background:var(--surface-hover)}.check-toggle.on{background:var(--success);color:#fff;box-shadow:0 0 0 4px rgba(16,185,129,.16)}.check-toggle:disabled{opacity:.5;cursor:not-allowed}
.cover-warning{padding:10px 12px;margin-bottom:12px;border:1px solid var(--warning);border-radius:10px;color:var(--warning)}.cover-layout{display:grid;grid-template-columns:220px minmax(0,1fr);gap:16px;min-height:480px}.cover-files{border-right:1px solid var(--border);padding-right:12px}.cover-file{position:relative;display:flex;border:1px solid transparent;border-radius:10px}.cover-file.active{border-color:var(--primary);background:var(--primary-soft)}.cover-file__select{min-width:0;flex:1;padding:10px;text-align:left;border:0;background:transparent;color:var(--text);cursor:pointer}.cover-file__remove{width:32px;border:0;background:transparent;color:var(--text-muted);font-size:18px;cursor:pointer}.cover-file__remove:hover{color:var(--danger)}.cover-files span,.cover-files small{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.cover-files small{color:var(--text-muted);margin-top:4px}.cover-main{min-width:0}.cover-capture-panel{padding:0}.cover-capture-tabs{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));border-bottom:1px solid var(--border)}.cover-capture-tabs>button{min-width:0;padding:4px 8px 10px;border:0;border-bottom:2px solid transparent;background:transparent;color:var(--text-muted);text-align:center;cursor:pointer;transition:border-color .16s ease,color .16s ease}.cover-capture-tabs>button:hover{color:var(--text)}.cover-capture-tabs>button.active{border-bottom-color:var(--primary);color:var(--primary)}.cover-capture-tabs strong{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:13px;font-weight:600}.cover-capture-run{display:flex;align-items:center;gap:10px;padding-top:10px}.cover-capture-hint{min-width:0;flex:1;font-size:12px;color:var(--text-muted)}.cover-time{display:flex;align-items:center;gap:4px}.cover-time input{width:48px;padding:7px;border:1px solid var(--border);border-radius:8px;background:var(--surface);color:var(--text)}.cover-time span{font-size:12px;color:var(--text-muted)}.cover-status{display:flex;align-items:center;gap:8px;margin-top:12px;color:var(--text-muted)}.cover-spinner{display:inline-block;width:15px;height:15px;border:2px solid var(--border);border-top-color:var(--primary);border-radius:50%;animation:cover-spin .8s linear infinite}@keyframes cover-spin{to{transform:rotate(360deg)}}
.cover-workspace{display:grid;grid-template-columns:minmax(0,1fr) 250px;gap:18px;margin-top:16px;align-items:start}.cover-section-title{display:flex;align-items:baseline;justify-content:space-between;gap:8px;margin-bottom:9px}.cover-section-title span{font-size:12px;color:var(--text-muted)}.cover-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(116px,1fr));gap:10px}.cover-grid button{min-width:0;padding:4px;border:2px solid transparent;border-radius:10px;background:var(--surface-soft);cursor:pointer}.cover-grid button.selected{border-color:var(--primary);background:var(--primary-soft)}.cover-grid img{display:block;width:100%;aspect-ratio:4/3;object-fit:contain;border-radius:6px;background:#06080c}.cover-grid span{display:block;padding:4px;color:var(--text-muted)}
.cover-package-controls{grid-column:1/-1;min-width:0;margin-top:2px;padding-top:14px;border-top:1px solid var(--border)}.cover-package-toggle{display:flex;align-items:center;gap:9px;width:max-content;cursor:pointer}.cover-package-toggle input{width:17px;height:17px;accent-color:var(--primary)}.cover-package-toggle span,.cover-package-toggle small{display:block}.cover-package-toggle small{margin-top:2px;color:var(--text-muted)}.cover-package-options{min-width:0;display:flex;align-items:center;flex-wrap:wrap;gap:10px 20px;margin-top:12px}.cover-title-input{min-width:240px;flex:1 1 320px;display:flex;align-items:center;gap:8px}.cover-title-input>span,.cover-style-controls span{flex:0 0 auto;font-size:12px;color:var(--text-muted)}.cover-title-input input{min-width:0;width:100%;padding:8px 10px;border:1px solid var(--border);border-radius:8px;background:var(--surface);color:var(--text)}.cover-style-controls{min-width:0;display:flex;align-items:center;flex:1 1 520px;flex-wrap:wrap;gap:8px 18px}.cover-style-controls label{display:flex;align-items:center;gap:6px;white-space:nowrap}.cover-style-controls select{height:30px;padding:0 24px 0 8px;border:1px solid var(--border);border-radius:8px;background:var(--surface);color:var(--text)}.cover-style-controls input[type=color]{width:30px;height:30px;padding:2px;border:1px solid var(--border);border-radius:8px;background:var(--surface);cursor:pointer}.cover-opacity input,.cover-height input{width:82px;accent-color:var(--primary)}.cover-opacity small,.cover-height small{width:34px;color:var(--text-muted);font-size:11px}
.cover-package-controls{grid-column:auto;margin-top:16px}
.cover-package-head{display:flex;align-items:center;gap:24px}.cover-package-head .cover-title-input{max-width:620px}
.cover-preview-pane{min-width:0}.cover-preview-frame{position:relative;width:100%;overflow:hidden;border-radius:12px;background:#08090e;box-shadow:0 10px 28px rgba(4,8,18,.16);cursor:grab;touch-action:none;user-select:none}.cover-preview-frame.dragging{cursor:grabbing}.cover-preview-frame canvas{display:block;width:100%;aspect-ratio:2/3;pointer-events:none;transition:opacity .15s ease}.cover-preview-frame canvas.loading{opacity:.5}.cover-preview-zoom{position:absolute;z-index:2;top:10px;right:10px;display:flex;flex-direction:column;align-items:center;gap:5px;padding:7px 5px;border:1px solid rgba(255,255,255,.22);border-radius:10px;background:rgba(7,12,20,.58);color:#fff;cursor:default;backdrop-filter:blur(8px)}.cover-preview-zoom small{font-size:10px;line-height:1}.cover-preview-zoom input{width:18px;height:86px;margin:0;accent-color:#fff;cursor:pointer;writing-mode:vertical-lr;direction:rtl}.cover-preview-loading{position:absolute;inset:0;display:flex;align-items:center;justify-content:center;gap:8px;pointer-events:none;color:#fff;background:rgba(5,7,12,.28);font-size:13px}.cover-empty{display:flex;align-items:center;justify-content:center;min-height:260px;margin-top:16px;border:1px dashed var(--border);border-radius:12px;color:var(--text-muted)}.cover-error{color:var(--danger)}.cover-footer{display:flex;align-items:stretch;gap:10px;width:100%}.cover-footer__path{min-width:0;flex:1}.cover-footer>:last-child{flex:0 0 auto}
.cover-style-controls input[type=range]{appearance:none;-webkit-appearance:none;width:88px;height:16px;margin:0;background:transparent;cursor:pointer}.cover-style-controls input[type=range]::-webkit-slider-runnable-track{height:2px;border:0;border-radius:999px;background:color-mix(in srgb,var(--text-muted) 34%,transparent)}.cover-style-controls input[type=range]::-webkit-slider-thumb{-webkit-appearance:none;width:12px;height:12px;margin-top:-5px;border:2px solid var(--surface);border-radius:50%;background:var(--primary);box-shadow:0 1px 5px rgba(31,78,150,.25);transition:transform .15s ease,box-shadow .15s ease}.cover-style-controls input[type=range]:hover::-webkit-slider-thumb{transform:scale(1.12);box-shadow:0 1px 7px rgba(31,78,150,.34)}.cover-style-controls input[type=range]:focus-visible::-webkit-slider-thumb{box-shadow:0 0 0 3px color-mix(in srgb,var(--primary) 22%,transparent)}.cover-style-controls input[type=range]::-moz-range-track{height:2px;border:0;border-radius:999px;background:color-mix(in srgb,var(--text-muted) 34%,transparent)}.cover-style-controls input[type=range]::-moz-range-thumb{width:9px;height:9px;border:2px solid var(--surface);border-radius:50%;background:var(--primary);box-shadow:0 1px 5px rgba(31,78,150,.25)}
.cover-preview-zoom{padding:8px 7px;border-color:rgba(255,255,255,.14);border-radius:12px;background:rgba(8,13,22,.48);box-shadow:0 6px 18px rgba(0,0,0,.16)}.cover-preview-zoom input{appearance:none;-webkit-appearance:none;width:3px;height:88px;margin:2px 7px;background:rgba(255,255,255,.32);border-radius:999px;outline:none}.cover-preview-zoom input::-webkit-slider-runnable-track{width:3px;border:0;border-radius:999px;background:rgba(255,255,255,.32)}.cover-preview-zoom input::-webkit-slider-thumb{-webkit-appearance:none;width:13px;height:13px;border:2px solid rgba(20,25,34,.48);border-radius:50%;background:#fff;box-shadow:0 2px 7px rgba(0,0,0,.28);transition:transform .15s ease}.cover-preview-zoom input:hover::-webkit-slider-thumb{transform:scale(1.12)}.cover-preview-zoom input::-moz-range-track{width:3px;border:0;border-radius:999px;background:rgba(255,255,255,.32)}.cover-preview-zoom input::-moz-range-thumb{width:10px;height:10px;border:2px solid rgba(20,25,34,.48);border-radius:50%;background:#fff;box-shadow:0 2px 7px rgba(0,0,0,.28)}
.cover-style-controls input[type=range]::-webkit-slider-thumb{border:1px solid rgba(255,255,255,.9);background:#2f6fed}.cover-style-controls input[type=range]::-moz-range-thumb{border:1px solid rgba(255,255,255,.9);background:#2f6fed}
.cover-preview-zoom input{width:88px;height:3px;margin:44px -35px;writing-mode:horizontal-tb;direction:ltr;transform:rotate(-90deg);transform-origin:center}.cover-preview-zoom input::-webkit-slider-runnable-track{width:100%;height:3px}.cover-preview-zoom input::-webkit-slider-thumb{width:13px;height:13px;margin-top:-5px;border:1px solid rgba(255,255,255,.9);background:#2f6fed}.cover-preview-zoom input::-moz-range-track{width:100%;height:3px}.cover-preview-zoom input::-moz-range-thumb{width:11px;height:11px;border:1px solid rgba(255,255,255,.9);background:#2f6fed}
.cover-package-head{display:grid;grid-template-columns:180px minmax(0,1fr);align-items:center;gap:20px}.cover-package-head .cover-title-input{max-width:none}.cover-package-toggle{display:flex;align-items:center;gap:10px;width:auto;min-height:38px}.cover-package-toggle strong{font-size:15px;white-space:nowrap}.cover-package-options{display:block;margin-top:14px}.cover-style-controls{display:grid;grid-template-columns:minmax(150px,180px) minmax(210px,1fr) 94px minmax(210px,1fr) 94px;align-items:center;gap:16px 24px}.cover-style-controls label{min-width:0}.cover-style-controls .cover-height,.cover-style-controls .cover-opacity{display:grid;grid-template-columns:auto minmax(76px,1fr) 36px}.cover-style-controls input[type=range]{width:100%}.cover-style-controls input[type=color]{width:28px;height:28px;border-radius:7px}.cover-style-controls select{min-width:92px}
.cover-style-controls input[type=color]{padding:0;border:0;background:transparent;box-shadow:none}.cover-style-controls input[type=color]::-webkit-color-swatch-wrapper{padding:0}.cover-style-controls input[type=color]::-webkit-color-swatch{border:0;border-radius:6px}.cover-style-controls input[type=color]::-moz-color-swatch{border:0;border-radius:6px}.cover-edge :deep(.select){width:92px}.cover-edge :deep(.select__trigger){min-height:30px;padding:5px 9px;border-radius:8px}
@media(max-width:760px){.cover-layout{grid-template-columns:1fr}.cover-files{border-right:0;border-bottom:1px solid var(--border);padding:0 0 10px;max-height:150px;overflow:auto}.cover-capture-tabs>button{padding-inline:3px}.cover-capture-run{align-items:stretch;flex-wrap:wrap}.cover-capture-hint{flex-basis:100%}.cover-workspace{grid-template-columns:1fr}.cover-preview-pane{width:min(280px,100%);margin:0 auto}.cover-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.cover-package-head{align-items:flex-start;flex-direction:column;gap:10px}.cover-package-options{display:grid;grid-template-columns:1fr}.cover-title-input{min-width:0;width:100%}.cover-style-controls{gap:9px 12px}.cover-opacity,.cover-height{flex:1 1 180px}.cover-opacity input,.cover-height input{min-width:70px;flex:1}.cover-time{flex:1}.cover-footer{flex-wrap:wrap}.cover-footer__path{flex-basis:100%}.cover-footer>:last-child{margin-left:auto}}
@media(max-width:1100px){.cover-style-controls{grid-template-columns:repeat(2,minmax(0,1fr))}.cover-style-controls>label:last-child{grid-column:auto}}
@media(max-width:760px){.cover-package-head{display:grid;grid-template-columns:1fr;gap:10px}.cover-style-controls{grid-template-columns:1fr}.cover-style-controls .cover-height,.cover-style-controls .cover-opacity{grid-template-columns:auto minmax(90px,1fr) 36px}}
</style>
