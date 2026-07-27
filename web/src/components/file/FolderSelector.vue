<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { filesApi } from "@/api/files";
import { getApiErrorMessage } from "@/api/client";
import { toast } from "@/composables/useToast";
import { FileNameError, validateFileName } from "@/utils/fileName";
import type { FileItem } from "@/api/types";
import type { Crumb } from "@/stores/browser";
import type { SortKey, SortOrder } from "@/types/file-browser";
import { naturalSort } from "@/utils/naturalSort";
import { formatTime } from "@/utils/format";
import BreadcrumbNav from "./BreadcrumbNav.vue";
import SvgIcon from "@/components/icons/SvgIcon.vue";
import AppButton from "@/components/base/AppButton.vue";
import "@/styles/file-list.css";

type FolderSortKey = Extract<SortKey, "name" | "modified">;
type FolderLoader = (parentId: string, opts?: { forceRefresh?: boolean }) => Promise<FileItem[]>;
export type FolderSelection = { id: string; name: string };

const ROOT: Crumb = { id: "", name: "根目录" };

const props = withDefaults(
  defineProps<{
    // 网盘浏览必填；提供 loader 时可为 0（仅用于 resolve 回传）。
    accountId?: number;
    // 自定义目录加载（如 STRM 任务本地目录）；提供后不再走 filesApi。
    loader?: FolderLoader;
    title?: string;
    confirmText?: string;
    // 不可作为目标/不可进入的目录（如正在移动的文件夹自身）。
    excludedFolderIds?: string[];
    showRefresh?: boolean;
    // 允许内联新建文件夹（移动/复制场景为 true）；loader 模式下忽略。
    allowCreateFolder?: boolean;
    // 初始定位路径（含 ID 的面包屑）；缺省从根目录开始。
    initialBreadcrumb?: Crumb[];
    // 按路径逐级定位（目录不存在时回退根目录）。
    initialPath?: string;
    // 锁定浏览根为任务目录，不可回到更上层。
    rootAnchor?: { parentId: string; path: string; label?: string };
    // 同层多选子目录。
    multiSelect?: boolean;
    // 受控已选目录；传入后跨目录浏览保留，并由父组件汇总。
    selectedItems?: FolderSelection[];
    // 是否显示右上角关闭按钮（嵌套在自带关闭栏的容器里时可关）。
    showClose?: boolean;
    // 是否显示当前目录筛选搜索框。
    showSearch?: boolean;
  }>(),
  {
    accountId: 0,
    title: "选择目录",
    confirmText: "选择当前目录",
    excludedFolderIds: () => [],
    showRefresh: true,
    allowCreateFolder: false,
    initialBreadcrumb: () => [],
    initialPath: "",
    rootAnchor: undefined,
    multiSelect: false,
    selectedItems: undefined,
    showClose: true,
    showSearch: true,
  },
);

const emit = defineEmits<{
  resolve: [payload: {
    accountId: number;
    parentId: string;
    path: string;
    selections?: FolderSelection[];
  }];
  cancel: [];
  "update:selectedItems": [items: FolderSelection[]];
}>();

const loading = ref(false);
const error = ref("");
const dirs = ref<FileItem[]>([]);
const breadcrumb = ref<Crumb[]>([ROOT]);
const filterKeyword = ref("");
const sortKey = ref<FolderSortKey>("name");
const sortOrder = ref<SortOrder>("asc");
const creating = ref(false);
const showCreateInput = ref(false);
const newFolderName = ref("");
const createInputRef = ref<HTMLInputElement | null>(null);
const selectedMap = ref<Record<string, FolderSelection>>({});

const columns: { key: FolderSortKey; label: string }[] = [
  { key: "name", label: "名称" },
  { key: "modified", label: "修改时间" },
];

const currentParentId = computed(() => breadcrumb.value[breadcrumb.value.length - 1]?.id ?? "");
const currentPath = computed(() => {
  if (props.rootAnchor) {
    const base = props.rootAnchor.path.replace(/\/+$/, "") || "/";
    const extra = breadcrumb.value.slice(1).map((c) => c.name);
    if (!extra.length) return base;
    return `${base}/${extra.join("/")}`;
  }
  const names = breadcrumb.value.slice(1).map((c) => c.name);
  return names.length ? `/${names.join("/")}` : "/";
});

function anchorLabel(anchor: { path: string; label?: string }) {
  if (anchor.label?.trim()) return anchor.label.trim();
  const segs = anchor.path.split("/").filter(Boolean);
  return segs[segs.length - 1] || "任务目录";
}

const controlled = computed(() => props.selectedItems !== undefined);
const activeSelections = computed(() =>
  controlled.value ? (props.selectedItems ?? []) : Object.values(selectedMap.value),
);
const canCreateFolder = computed(() => props.allowCreateFolder && !props.loader);
const selectedCount = computed(() => activeSelections.value.length);
const primaryActionText = computed(() => {
  if (!props.multiSelect) return props.confirmText;
  // 受控模式由父组件决定文案（可汇总跨任务已选数量）。
  if (controlled.value) return props.confirmText;
  if (selectedCount.value > 0) return `添加所选 (${selectedCount.value})`;
  return props.confirmText;
});
const tableClass = computed(() => ({
  "folder-selector__table--multi": props.multiSelect,
}));

type SelectState = "none" | "checked" | "partial" | "covered";

function clearSelection() {
  if (controlled.value) return;
  selectedMap.value = {};
}

function normalizeSelectPath(id: string) {
  return String(id || "").replace(/^\/+|\/+$/g, "");
}

function isAncestorPath(ancestor: string, child: string) {
  const a = normalizeSelectPath(ancestor);
  const c = normalizeSelectPath(child);
  if (!a || !c || a === c) return false;
  return c.startsWith(`${a}/`);
}

function selectionState(id: string): SelectState {
  const key = normalizeSelectPath(id);
  if (!key && !id) {
    // 根目录自身一般不出现在行内；无 key 时只看是否有任意已选
    return activeSelections.value.length ? "partial" : "none";
  }
  const items = activeSelections.value;
  if (items.some((item) => normalizeSelectPath(item.id) === key)) return "checked";
  if (items.some((item) => isAncestorPath(item.id, key))) return "covered";
  if (items.some((item) => isAncestorPath(key, item.id))) return "partial";
  return "none";
}

function isSelected(id: string) {
  const state = selectionState(id);
  return state === "checked" || state === "covered";
}

function isPartialSelected(id: string) {
  return selectionState(id) === "partial";
}

function commitSelections(next: FolderSelection[]) {
  if (controlled.value) {
    emit("update:selectedItems", next);
    return;
  }
  const map: Record<string, FolderSelection> = {};
  for (const item of next) map[item.id] = item;
  selectedMap.value = map;
}

/** 勾选/取消时父子互斥：选父清子孙，选子清祖先。 */
function withPathExclusive(current: FolderSelection[], item: FolderSelection, selected: boolean) {
  const id = normalizeSelectPath(item.id);
  const next = current.filter((entry) => {
    const entryId = normalizeSelectPath(entry.id);
    if (entryId === id) return false;
    if (isAncestorPath(entryId, id) || isAncestorPath(id, entryId)) return false;
    return true;
  });
  if (selected) next.push({ id: item.id, name: item.name || item.id });
  return next;
}

function toggleSelect(dir: FileItem, checked?: boolean) {
  if (!props.multiSelect) return;
  const id = String(dir.id);
  const state = selectionState(id);
  if (state === "covered") {
    // 取消覆盖它的上级全选
    const key = normalizeSelectPath(id);
    commitSelections(
      activeSelections.value.filter((item) => !isAncestorPath(item.id, key)),
    );
    return;
  }
  if (state === "partial") {
    // 半选 → 全选当前目录（吸收已选子孙）
    commitSelections(withPathExclusive(activeSelections.value, { id, name: dir.name || id }, true));
    return;
  }
  const selected = checked ?? state !== "checked";
  commitSelections(withPathExclusive(activeSelections.value, { id, name: dir.name || id }, selected));
}

async function listDirs(parentId: string, opts?: { forceRefresh?: boolean }): Promise<FileItem[]> {
  if (props.loader) {
    return props.loader(parentId, opts);
  }
  const res = await filesApi.list(props.accountId, parentId, opts);
  return res.items.filter((it) => it.is_dir);
}

async function resolveInitialBreadcrumb(): Promise<Crumb[]> {
  if (props.rootAnchor) {
    return [{ id: props.rootAnchor.parentId, name: anchorLabel(props.rootAnchor) }];
  }
  const init = props.initialBreadcrumb;
  if (init.length && init[0]?.id === "") return [...init];

  const segments = (props.initialPath || "")
    .split("/")
    .map((item) => item.trim())
    .filter(Boolean);
  if (!segments.length) return [ROOT];

  let parentId = "";
  const crumbs: Crumb[] = [ROOT];
  try {
    for (const segment of segments) {
      const items = await listDirs(parentId);
      const matched = items.find(
        (item) => item.is_dir && String(item.name || "").trim() === segment,
      );
      if (!matched) throw new Error(`目录不存在: ${segment}`);
      parentId = String(matched.id);
      crumbs.push({ id: parentId, name: String(matched.name || segment) });
    }
    return crumbs;
  } catch {
    toast.info("已保存目录不存在或无法定位，已打开根目录");
    return [ROOT];
  }
}

async function resetAndLoad() {
  breadcrumb.value = await resolveInitialBreadcrumb();
  filterKeyword.value = "";
  resetCreateState();
  clearSelection();
  await load(currentParentId.value);
}

const excludedSet = computed(() => new Set(props.excludedFolderIds.map(String)));

const visibleDirs = computed(() =>
  dirs.value.filter((d) => d.is_dir && !excludedSet.value.has(String(d.id))),
);

const filteredDirs = computed(() => {
  const kw = filterKeyword.value.trim().toLowerCase();
  if (!kw) return visibleDirs.value;
  return visibleDirs.value.filter((d) => (d.name || "").toLowerCase().includes(kw));
});

const sortedDirs = computed(() => {
  const order = sortOrder.value === "desc" ? -1 : 1;
  return [...filteredDirs.value].sort((a, b) => {
    if (sortKey.value === "modified") {
      const ta = Date.parse(a.mod_time || "") || 0;
      const tb = Date.parse(b.mod_time || "") || 0;
      return (ta - tb) * order;
    }
    return naturalSort(a.name || "", b.name || "") * order;
  });
});

const headerSelectState = computed<SelectState>(() => {
  if (!sortedDirs.value.length) return "none";
  let checkedOrCovered = 0;
  let partial = 0;
  for (const dir of sortedDirs.value) {
    const state = selectionState(String(dir.id));
    if (state === "checked" || state === "covered") checkedOrCovered += 1;
    else if (state === "partial") partial += 1;
  }
  if (checkedOrCovered === sortedDirs.value.length) return "checked";
  if (checkedOrCovered > 0 || partial > 0) return "partial";
  return "none";
});

function toggleSelectAllVisible() {
  if (!props.multiSelect || !sortedDirs.value.length) return;
  if (headerSelectState.value === "checked") {
    let next = activeSelections.value;
    for (const dir of sortedDirs.value) {
      const id = String(dir.id);
      const key = normalizeSelectPath(id);
      next = next.filter(
        (item) =>
          normalizeSelectPath(item.id) !== key
          && !isAncestorPath(item.id, key)
          && !isAncestorPath(key, item.id),
      );
    }
    commitSelections(next);
    return;
  }
  let next = activeSelections.value;
  for (const dir of sortedDirs.value) {
    next = withPathExclusive(next, { id: String(dir.id), name: dir.name || String(dir.id) }, true);
  }
  commitSelections(next);
}

function sortClass(key: FolderSortKey): SortOrder | "" {
  return sortKey.value === key ? sortOrder.value : "";
}

function toggleSort(key: FolderSortKey) {
  if (sortKey.value === key) {
    sortOrder.value = sortOrder.value === "asc" ? "desc" : "asc";
    return;
  }
  sortKey.value = key;
  sortOrder.value = key === "name" ? "asc" : "desc";
}

async function load(parentId: string, opts?: { forceRefresh?: boolean }) {
  loading.value = true;
  error.value = "";
  try {
    dirs.value = await listDirs(parentId, opts);
  } catch (e) {
    dirs.value = [];
    error.value = getApiErrorMessage(e, "加载失败");
  } finally {
    loading.value = false;
  }
}

function resetCreateState() {
  showCreateInput.value = false;
  newFolderName.value = "";
}

function openDir(dir: FileItem) {
  breadcrumb.value = [...breadcrumb.value, { id: dir.id, name: dir.name }];
  filterKeyword.value = "";
  resetCreateState();
  clearSelection();
  void load(dir.id);
}

function goTo(index: number) {
  const minIndex = props.rootAnchor ? 0 : 0;
  if (index < minIndex) return;
  if (index >= breadcrumb.value.length - 1) return;
  breadcrumb.value = breadcrumb.value.slice(0, index + 1);
  filterKeyword.value = "";
  resetCreateState();
  clearSelection();
  void load(currentParentId.value);
}

function refresh() {
  resetCreateState();
  void load(currentParentId.value, { forceRefresh: true });
}

function startCreateFolder() {
  if (!canCreateFolder.value || creating.value) return;
  filterKeyword.value = "";
  showCreateInput.value = true;
  void nextTick(() => {
    createInputRef.value?.focus();
  });
}

function cancelCreateFolder() {
  if (creating.value) return;
  resetCreateState();
}

async function submitCreateFolder() {
  if (!canCreateFolder.value) return;
  let name: string;
  try {
    name = validateFileName(newFolderName.value);
  } catch (e) {
    toast.info(e instanceof FileNameError ? e.message : "文件夹名称无效");
    return;
  }
  if (visibleDirs.value.some((d) => (d.name || "").toLowerCase() === name.toLowerCase())) {
    toast.info("当前目录已存在同名文件夹");
    return;
  }

  creating.value = true;
  try {
    const res = await filesApi.createFolder({
      account_id: props.accountId,
      parent_id: currentParentId.value,
      name,
    });
    resetCreateState();
    await load(currentParentId.value);
    const created = dirs.value.find(
      (d) => String(d.id) === String(res.folder_id) || (d.name || "").toLowerCase() === name.toLowerCase(),
    );
    toast.success(`文件夹 "${name}" 创建成功`);
    if (created) openDir(created);
  } catch (e) {
    toast.error(getApiErrorMessage(e, "创建文件夹失败"));
  } finally {
    creating.value = false;
  }
}

function selectCurrent() {
  const payload: {
    accountId: number;
    parentId: string;
    path: string;
    selections?: FolderSelection[];
  } = {
    accountId: props.accountId,
    parentId: currentParentId.value,
    path: currentPath.value,
  };
  if (props.multiSelect && selectedCount.value > 0) {
    payload.selections = activeSelections.value;
  }
  emit("resolve", payload);
}

watch(
  () => [props.accountId, props.initialPath, props.initialBreadcrumb, props.rootAnchor] as const,
  () => {
    void resetAndLoad();
  },
  { immediate: true },
);
</script>

<template>
  <div class="folder-selector">
    <div v-if="title || showSearch || showClose" class="folder-selector__header">
      <h3 v-if="title" class="folder-selector__title">{{ title }}</h3>
      <label
        v-if="showSearch"
        class="folder-selector__search"
        title="仅筛选当前目录下已加载的文件夹"
      >
        <span class="folder-selector__search-icon"><SvgIcon name="search" :size="15" /></span>
        <input
          v-model.trim="filterKeyword"
          type="search"
          placeholder="筛选当前目录文件夹"
          :disabled="loading"
          maxlength="100"
        />
        <button
          v-if="filterKeyword"
          type="button"
          class="folder-selector__search-clear"
          aria-label="清空筛选"
          @click="filterKeyword = ''"
        >
          ×
        </button>
      </label>
      <button
        v-if="showClose"
        type="button"
        class="folder-selector__close"
        aria-label="关闭"
        @click="emit('cancel')"
      >
        ×
      </button>
    </div>

    <div class="folder-selector__content">
      <BreadcrumbNav :items="breadcrumb" compact @navigate="goTo" />
      <div v-if="filterKeyword" class="folder-selector__filter-tip">
        仅筛选当前目录，匹配 {{ sortedDirs.length }} 项
      </div>

      <div class="file-list folder-selector__list" :class="tableClass">
        <div class="folder-table-header" role="row">
          <label
            v-if="multiSelect"
            class="checkbox-col"
            :title="headerSelectState === 'checked' ? '取消全选' : '全选当前层'"
          >
            <input
              type="checkbox"
              :checked="headerSelectState === 'checked'"
              :indeterminate="headerSelectState === 'partial'"
              :disabled="loading || !sortedDirs.length"
              @change="toggleSelectAllVisible"
            />
          </label>
          <button
            v-for="col in columns"
            :key="col.key"
            type="button"
            class="folder-table-heading"
            :class="[`col-${col.key}`, { active: sortKey === col.key }]"
            @click="toggleSort(col.key)"
          >
            <span>{{ col.label }}</span>
            <span class="sort-indicator" :class="sortClass(col.key)" />
          </button>
        </div>

        <div class="folder-table-body">
          <div v-if="showCreateInput" class="folder-create-row">
            <span class="folder-name-icon"><SvgIcon name="folder" :size="18" /></span>
            <input
              ref="createInputRef"
              v-model.trim="newFolderName"
              type="text"
              class="inline-rename-input"
              placeholder="输入文件夹名称"
              maxlength="100"
              :disabled="creating"
              @keyup.enter="submitCreateFolder"
              @keyup.esc="cancelCreateFolder"
            />
            <button
              type="button"
              class="folder-inline-btn confirm"
              title="确认"
              :disabled="creating"
              @click="submitCreateFolder"
            >
              ✓
            </button>
            <button
              type="button"
              class="folder-inline-btn cancel"
              title="取消"
              :disabled="creating"
              @click="cancelCreateFolder"
            >
              ×
            </button>
          </div>

          <div v-if="loading" class="folder-state">加载中…</div>
          <div v-else-if="error" class="folder-state error">{{ error }}</div>
          <div v-else-if="!sortedDirs.length && !showCreateInput" class="folder-state">
            {{ filterKeyword ? "当前目录没有匹配的文件夹" : "没有子目录" }}
          </div>
          <template v-else>
            <div
              v-for="dir in sortedDirs"
              :key="dir.id"
              class="folder-table-row"
              :class="{
                selected: multiSelect && (isSelected(String(dir.id)) || isPartialSelected(String(dir.id))),
              }"
              @click="openDir(dir)"
            >
              <label
                v-if="multiSelect"
                class="checkbox-col"
                :title="selectionState(String(dir.id)) === 'covered'
                  ? '已包含在上级全选中，点击取消上级全选'
                  : selectionState(String(dir.id)) === 'partial'
                    ? '已选部分子目录，点击改为全选此目录'
                    : `选择 ${dir.name}`"
                @click.stop
              >
                <input
                  type="checkbox"
                  :checked="isSelected(String(dir.id))"
                  :indeterminate="isPartialSelected(String(dir.id))"
                  :aria-label="`选择 ${dir.name}`"
                  @change="toggleSelect(dir, ($event.target as HTMLInputElement).checked)"
                />
              </label>
              <div class="folder-name-cell">
                <span class="folder-name-icon"><SvgIcon name="folder" :size="18" /></span>
                <span class="folder-name-text" :title="dir.name">{{ dir.name }}</span>
              </div>
              <span class="folder-time-cell">{{ formatTime(dir.mod_time) }}</span>
            </div>
          </template>
        </div>
      </div>
    </div>

    <div class="folder-selector__footer">
      <button
        v-if="canCreateFolder"
        type="button"
        class="folder-selector__secondary"
        :disabled="loading || creating"
        @click="startCreateFolder"
      >
        <span class="folder-selector__btn-icon"><SvgIcon name="folder-plus" :size="16" /></span>
        新建文件夹
      </button>
      <button
        v-if="showRefresh"
        type="button"
        class="folder-selector__secondary"
        :disabled="loading"
        @click="refresh"
      >
        <span class="folder-selector__btn-icon" :class="{ spin: loading }">
          <SvgIcon name="refresh" :size="16" />
        </span>
        刷新
      </button>
      <div class="folder-selector__spacer" aria-hidden="true" />
      <span v-if="multiSelect && selectedCount" class="folder-selector__count">已选 {{ selectedCount }} 项</span>
      <AppButton
        variant="primary"
        class="folder-selector__confirm"
        :disabled="loading"
        @click="selectCurrent"
      >
        {{ primaryActionText }}
      </AppButton>
    </div>
  </div>
</template>

<style scoped>
.folder-selector {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  min-height: 0;
  box-sizing: border-box;
}

.folder-selector__header {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 20px 24px 0;
}
.folder-selector__title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text);
  flex-shrink: 0;
}
.folder-selector__search {
  width: 220px;
  height: 36px;
  margin-left: auto;
  padding: 0 10px;
  border-radius: var(--radius-pill);
  background: var(--surface-sunken);
  color: var(--text-muted);
  display: flex;
  align-items: center;
  gap: 8px;
  box-sizing: border-box;
}
.folder-selector__search-icon {
  display: inline-flex;
  flex-shrink: 0;
  line-height: 0;
}
.folder-selector__search input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  color: var(--text);
  font-size: 13px;
}
.folder-selector__search input::placeholder {
  color: var(--text-muted);
}
.folder-selector__search-clear {
  width: 18px;
  height: 18px;
  padding: 0;
  border: none;
  border-radius: 50%;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  font-size: 16px;
  line-height: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.folder-selector__search-clear:hover {
  color: var(--text);
}
.folder-selector__close {
  background: none;
  border: none;
  color: var(--text-muted);
  font-size: 20px;
  line-height: 1;
  width: 24px;
  height: 24px;
  padding: 0;
  cursor: pointer;
}
.folder-selector__close:hover {
  color: var(--text);
}

.folder-selector__content {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  padding: 14px 24px 12px;
  box-sizing: border-box;
  overflow: visible;
}
.folder-selector__content :deep(.breadcrumb) {
  flex: none;
}
.folder-selector__filter-tip {
  margin-top: 8px;
  color: var(--text-muted);
  font-size: 12px;
}

.folder-selector__list {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  margin-top: 6px;
  overflow: hidden;
}

.folder-table-header,
.folder-table-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 200px;
  align-items: center;
  column-gap: 16px;
}
.folder-selector__list.folder-selector__table--multi .folder-table-header,
.folder-selector__list.folder-selector__table--multi .folder-table-row {
  grid-template-columns: 48px minmax(0, 1fr) 200px;
}
.folder-table-header {
  flex-shrink: 0;
  min-height: 46px;
  margin: 0 0 6px;
  padding: 0 12px;
  background: var(--surface-muted);
  border-radius: var(--radius-md);
}
.folder-selector__list .checkbox-col {
  box-sizing: border-box;
  width: 48px;
  align-self: stretch;
  display: grid;
  place-items: center;
  margin: 0;
  cursor: pointer;
}
.folder-selector__list .folder-table-row .checkbox-col {
  min-height: 100%;
}
.folder-table-row.selected {
  background: color-mix(in srgb, var(--brand) 8%, var(--surface));
}
.folder-selector__count {
  margin-right: 8px;
  color: var(--text-muted);
  font-size: 12px;
}
.folder-table-heading {
  min-width: 0;
  height: 100%;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  font-weight: 500;
  text-align: left;
}
.folder-table-heading.active {
  color: var(--text-regular);
}

.folder-table-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding-right: 2px;
}
.folder-table-row {
  padding: 12px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background 0.15s ease;
}
.folder-table-row:hover {
  background: var(--surface-sunken);
}
.folder-name-cell {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 10px;
}
.folder-name-icon {
  flex-shrink: 0;
  line-height: 0;
  display: inline-flex;
}
.folder-name-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 14px;
  color: var(--text-regular);
}
.folder-time-cell {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-muted);
  font-size: 13px;
  text-align: right;
}

.folder-create-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
}
.folder-create-row .inline-rename-input {
  flex: 1;
  min-width: 0;
  height: 32px;
}

.folder-state {
  padding: 28px 20px;
  text-align: center;
  color: var(--text-muted);
  font-style: italic;
}
.folder-state.error {
  color: var(--danger);
}

.folder-selector__footer {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
  padding: 0 24px 24px;
}
.folder-selector__spacer {
  flex: 1;
}
/* 内联白色按钮（新建文件夹/刷新）：刻意的无 hover、字重 400 变体，保留本地样式。 */
.folder-selector__secondary {
  height: 38px;
  border-radius: var(--radius-sm);
  padding: 0 16px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  cursor: pointer;
  font-size: 14px;
  font-weight: 400;
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--text-regular);
}
.folder-selector__secondary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
/* 确认按钮：仅布局，配色/hover 走全局 .btn--primary。 */
.folder-selector__confirm {
  height: 38px;
  min-width: 140px;
  padding: 0 16px;
  font-size: 14px;
}
.folder-selector__btn-icon {
  display: inline-flex;
  line-height: 0;
}
@media (max-width: 640px) {
  .folder-selector__header {
    padding: 18px 18px 0;
    flex-wrap: wrap;
    gap: 10px;
  }
  .folder-selector__search {
    order: 3;
    width: 100%;
    margin-left: 0;
  }
  .folder-selector__content {
    padding: 12px 18px 10px;
  }
  .folder-table-header,
  .folder-table-row {
    grid-template-columns: minmax(0, 1fr) 140px;
    column-gap: 10px;
  }
  .folder-selector__footer {
    padding: 0 18px 20px;
  }
}
</style>
