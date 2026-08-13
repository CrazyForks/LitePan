<script setup lang="ts">
import { onMounted, ref } from "vue";
import { getApiErrorMessage } from "@/api/client";
import {
  cloudToolsApi,
  localUploadApi,
  type CloudTool115Status,
  type LocalUploadMapping,
} from "@/api/cloudTools";
import { confirm } from "@/composables/useConfirm";
import { toast } from "@/composables/useToast";
import { useSettingsLoad } from "@/composables/useSettingsLoad";
import AppButton from "@/components/base/AppButton.vue";
import "@/styles/admin-shared.css";

const { runLoad } = useSettingsLoad();

const status = ref<CloudTool115Status>({ enabled: false, cache_count: 0, available: false });
const saving = ref(false);
const clearing = ref(false);

const localEnabled = ref(false);
const localMappings = ref<LocalUploadMapping[]>([]);
const localSaving = ref(false);
const mappingOpen = ref(false);
const newMappingName = ref("");
const newMappingPath = ref("");

async function load() {
  await runLoad(async () => {
    const [st, lu] = await Promise.all([
      cloudToolsApi.status115(),
      localUploadApi.getConfig().catch(() => ({ enabled: false, mappings: [] })),
    ]);
    status.value = st;
    localEnabled.value = lu.enabled;
    localMappings.value = lu.mappings;
  }, "加载网盘工具状态失败");
}

onMounted(load);

async function toggleEnabled() {
  saving.value = true;
  const next = !status.value.enabled;
  try {
    const res = await cloudToolsApi.set115Enabled(next);
    status.value.enabled = res.enabled;
    toast.success(
      res.enabled
        ? "已启用：115Open 账号的 STRM 任务将改用全量清单模式执行"
        : "已停用：115Open 账号的 STRM 任务恢复逐目录递归",
    );
  } catch (e) {
    toast.error(getApiErrorMessage(e, "修改开关失败"));
  } finally {
    saving.value = false;
  }
}

async function toggleLocalEnabled() {
  localSaving.value = true;
  const next = !localEnabled.value;
  try {
    const res = await localUploadApi.saveConfig({ enabled: next, mappings: localMappings.value });
    localEnabled.value = res.enabled;
    toast.success(
      res.enabled
        ? "已启用：前台「新建 → 上传」将提供从本机上传"
        : "已停用：前台上传恢复原有方式",
    );
  } catch (e) {
    toast.error(getApiErrorMessage(e, "修改开关失败"));
  } finally {
    localSaving.value = false;
  }
}

function openMappings() {
  mappingOpen.value = true;
}

function closeMappings() {
  mappingOpen.value = false;
}

function addMapping() {
  const name = newMappingName.value.trim();
  const path = newMappingPath.value.trim();
  if (!name || !path.startsWith("/")) {
    toast.error("请填写标签名和以 / 开头的容器内路径");
    return;
  }
  if (localMappings.value.some((m) => m.name === name)) {
    toast.error(`标签「${name}」已存在`);
    return;
  }
  localMappings.value.push({ name, path });
  newMappingName.value = "";
  newMappingPath.value = "";
}

function removeMapping(name: string) {
  localMappings.value = localMappings.value.filter((m) => m.name !== name);
}

async function saveMappings() {
  localSaving.value = true;
  try {
    const res = await localUploadApi.saveConfig({ enabled: localEnabled.value, mappings: localMappings.value });
    localMappings.value = res.mappings;
    mappingOpen.value = false;
    toast.success("映射目录已保存");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存映射目录失败"));
  } finally {
    localSaving.value = false;
  }
}

async function clearCache() {
  const ok = await confirm({
    title: "清空路径映射表？",
    message:
      `将删除 ${status.value.cache_count.toLocaleString("zh-CN")} 条目录路径映射记录，` +
      "下次该账号执行 STRM 任务时会重新解析目录路径，用于纠正目录被移动 / 重命名后的路径漂移。此操作不影响网盘文件与已生成的 STRM 文件。",
    confirmText: "确认清空",
    cancelText: "取消",
    danger: true,
  }).catch(() => false);
  if (!ok) return;

  clearing.value = true;
  try {
    const res = await cloudToolsApi.clear115Cache(0);
    toast.success(`已清空 ${res.removed.toLocaleString("zh-CN")} 条路径映射记录`);
    await load();
  } catch (e) {
    toast.error(getApiErrorMessage(e, "清空路径映射表失败"));
  } finally {
    clearing.value = false;
  }
}
</script>

<template>
  <div class="cloud-tools">
    <div class="cloud-tools__grid">
      <article class="tool-card" :class="status.enabled ? 'is-enabled' : 'is-disabled'">
        <span class="tool-card__bar" :class="status.enabled ? 'is-enabled' : 'is-disabled'" />

        <div class="tool-card__head">
          <img class="tool-card__logo" src="/logos/115.png" alt="115" />
          <div class="tool-card__meta">
            <h3 class="tool-card__name">
              115 网盘 STRM 增强方案
              <span class="tool-card__tag">115Open</span>
              <span class="tool-card__tag tool-card__tag--warn">实验性</span>
            </h3>
            <p class="tool-card__driver">作用于 STRM 任务 · 全量清单 + 增量对账</p>
          </div>
          <button
            class="check-toggle"
            type="button"
            :class="{ on: status.enabled }"
            :aria-label="status.enabled ? '停用 115 网盘 STRM 增强' : '启用 115 网盘 STRM 增强'"
            :disabled="saving || !status.available"
            title="启用 / 停用"
            @click="toggleEnabled"
          >
            <svg viewBox="0 0 16 16" aria-hidden="true">
              <path
                d="M3.5 8.5 6.5 11.5 12.5 4.5"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
          </button>
        </div>

        <p class="tool-card__desc">
          开启后，<code>115Open</code> 账号的 STRM 任务扫描更快：一次性获取整个目录的文件清单，
          不用一层层翻目录，请求更少、更不容易触发限制。分支的任务仍按分支的方式执行。
        </p>

        <div class="tool-card__row">
          <div class="tool-card__stat">
            <span class="tool-card__num">{{ status.cache_count.toLocaleString("zh-CN") }}</span>
            <span class="tool-card__label">条路径映射关系</span>
          </div>
          <div class="tool-card__actions">
            <AppButton variant="danger" :disabled="clearing" @click="clearCache">
              {{ clearing ? "清空中…" : "清空路径映射表" }}
            </AppButton>
          </div>
        </div>

      </article>

      <!-- 本机上传卡片 -->
      <article class="tool-card" :class="localEnabled ? 'is-enabled' : 'is-disabled'">
        <span class="tool-card__bar" :class="localEnabled ? 'is-enabled' : 'is-disabled'" />
        <div class="tool-card__head">
          <img class="tool-card__logo" src="/logos/local.png" alt="本机" />
          <div class="tool-card__meta">
            <h3 class="tool-card__name">
              从服务器上传
              <span class="tool-card__tag">通用</span>
            </h3>
            <p class="tool-card__driver">作用于全部网盘账号 · 上传面板双来源</p>
          </div>
          <button
            class="check-toggle"
            type="button"
            :class="{ on: localEnabled }"
            :aria-label="localEnabled ? '停用本机上传' : '启用本机上传'"
            :disabled="localSaving"
            title="启用 / 停用"
            @click="toggleLocalEnabled"
          >
            <svg viewBox="0 0 16 16" aria-hidden="true">
              <path
                d="M3.5 8.5 6.5 11.5 12.5 4.5"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
          </button>
        </div>
        <p class="tool-card__desc">
          开启后，前台「新建 → 上传」面板提供<strong>从服务器上传</strong>：
          选择服务器映射目录中的文件或文件夹，上传到当前网盘目录并保留文件夹结构；从访问机上传保持不变。
        </p>
        <div class="tool-card__row">
          <div class="tool-card__stat">
            <span class="tool-card__num">{{ localMappings.length }}</span>
            <span class="tool-card__label">映射目录</span>
          </div>
          <div class="tool-card__actions">
            <AppButton variant="secondary" :disabled="localSaving" @click="openMappings">
              目录映射设置
            </AppButton>
          </div>
        </div>
      </article>
    </div>

    <!-- 映射设置弹窗 -->
    <div class="local-mapping-overlay" :class="{ 'is-open': mappingOpen }" @click.self="closeMappings">
      <div class="local-mapping-modal">
        <h3 class="local-mapping-modal__title">本机上传 · 目录映射设置</h3>
        <div class="local-mapping-modal__body">
          <p class="local-mapping-tip">
            在 docker-compose 中先映射宿主机目录，再按容器内路径添加标签。
            前台从本机上传时按标签浏览，不会暴露服务器其他路径。
          </p>
          <div class="local-mapping-list">
            <div v-for="m in localMappings" :key="m.name" class="local-mapping-item">
              <span class="local-mapping-item__name">{{ m.name }}</span>
              <span class="local-mapping-item__path">{{ m.path }}</span>
              <button class="local-mapping-item__del" type="button" title="删除" @click="removeMapping(m.name)">
                ✕
              </button>
            </div>
            <div v-if="localMappings.length === 0" class="local-mapping-empty">还没有映射目录</div>
          </div>
          <div class="local-mapping-add">
            <input v-model="newMappingName" type="text" placeholder="标签名，如：媒体库" />
            <input v-model="newMappingPath" type="text" placeholder="容器内路径，如：/app/data/updatefiles" />
            <AppButton variant="primary" @click="addMapping">添加</AppButton>
          </div>
          <p class="local-mapping-hint">示例：<code>- /vol1/1000/updatefiles:/app/data/updatefiles</code></p>
        </div>
        <div class="local-mapping-modal__actions">
          <AppButton variant="secondary" @click="closeMappings">取消</AppButton>
          <AppButton variant="primary" :disabled="localSaving" @click="saveMappings">
            {{ localSaving ? "保存中…" : "保存" }}
          </AppButton>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.cloud-tools__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(420px, 1fr));
  gap: 18px;
}

.tool-card {
  position: relative;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-xl);
  padding: 20px;
  overflow: hidden;
  transition: var(--transition);
}
.tool-card:hover {
  box-shadow: var(--shadow-card);
}
.tool-card.is-enabled {
  border-color: color-mix(in srgb, var(--success) 40%, var(--border));
}
.tool-card__bar {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  width: 4px;
}
.tool-card__bar.is-enabled {
  background: linear-gradient(180deg, var(--success), #059669);
}
.tool-card__bar.is-disabled {
  background: linear-gradient(180deg, #9ca3af, #6b7280);
}

.tool-card__head {
  display: flex;
  align-items: center;
  gap: 14px;
}
.tool-card__meta {
  flex: 1;
  min-width: 0;
}
.tool-card__logo {
  width: 48px;
  height: 48px;
  border-radius: var(--radius-md);
  flex-shrink: 0;
  object-fit: cover;
}
.tool-card__name {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.tool-card__tag {
  font-size: 11px;
  font-weight: 500;
  padding: 1px 8px;
  border-radius: var(--radius-pill);
  background: var(--info-soft);
  color: var(--info);
}
.tool-card__tag--warn {
  background: color-mix(in srgb, var(--warning) 14%, var(--surface));
  color: #b45309;
}
.tool-card__driver {
  margin: 2px 0 0;
  font-size: 12px;
  color: var(--text-muted);
}

.tool-card__desc {
  margin: 14px 0 0;
  font-size: 13px;
  color: var(--text-regular);
}
.tool-card__desc code {
  background: var(--surface-sunken);
  border: 1px solid var(--border-soft);
  border-radius: 5px;
  padding: 1px 5px;
  font-size: 12px;
}

.tool-card__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px dashed var(--border);
}
.tool-card__stat {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.tool-card__num {
  font-size: 16px;
  font-weight: 700;
  color: var(--text);
}
.tool-card__label {
  font-size: 13px;
  color: var(--text-muted);
}
.tool-card__actions {
  display: flex;
  align-items: center;
  gap: 14px;
}

.check-toggle {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 0;
  padding: 0;
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  background: var(--border);
  color: var(--text-muted);
  transition: background 0.18s ease, color 0.18s ease, box-shadow 0.18s ease;
}
.check-toggle svg {
  width: 14px;
  height: 14px;
}
.check-toggle:hover {
  background: var(--surface-hover);
}
.check-toggle.on {
  background: var(--success);
  color: #fff;
  box-shadow: 0 0 0 4px rgba(16, 185, 129, 0.16);
}
.check-toggle.on:hover {
  background: color-mix(in srgb, var(--success) 88%, #000);
}
.check-toggle:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.local-mapping-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.4);
  display: none;
  align-items: center;
  justify-content: center;
  z-index: var(--z-modal);
}
.local-mapping-overlay.is-open {
  display: flex;
}
.local-mapping-modal {
  width: 520px;
  max-width: calc(100vw - 40px);
  background: var(--surface);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-pop);
  padding: 22px;
}
.local-mapping-modal__title {
  margin: 0 0 12px;
  font-size: 16px;
  font-weight: 700;
}
.local-mapping-tip {
  font-size: 12px;
  color: var(--text-muted);
  background: var(--surface-sunken);
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-md);
  padding: 10px 12px;
  margin: 0 0 12px;
}
.local-mapping-list {
  display: grid;
  gap: 8px;
  margin-bottom: 12px;
  max-height: 240px;
  overflow-y: auto;
}
.local-mapping-item {
  display: flex;
  align-items: center;
  gap: 10px;
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-md);
  padding: 10px 12px;
  background: var(--surface-sunken);
}
.local-mapping-item__name {
  font-weight: 600;
  font-size: 13px;
  flex-shrink: 0;
}
.local-mapping-item__path {
  font-size: 12px;
  color: var(--text-muted);
  font-family: ui-monospace, Menlo, monospace;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.local-mapping-item__del {
  border: none;
  background: transparent;
  color: var(--text-muted);
  font-size: 13px;
  padding: 4px 6px;
  border-radius: var(--radius-sm);
}
.local-mapping-item__del:hover {
  background: var(--border-soft);
  color: var(--danger);
}
.local-mapping-empty {
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
  padding: 18px 0;
}
.local-mapping-add {
  display: flex;
  gap: 8px;
}
.local-mapping-add input {
  flex: 1;
  min-width: 0;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 7px 10px;
  font-size: 13px;
  background: var(--surface);
  color: var(--text);
}
.local-mapping-hint {
  margin: 10px 0 0;
  font-size: 12px;
  color: var(--text-muted);
}
.local-mapping-hint code {
  font-family: ui-monospace, Menlo, monospace;
  background: var(--surface-sunken);
  border: 1px solid var(--border-soft);
  border-radius: 5px;
  padding: 1px 5px;
}
.local-mapping-modal__actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 18px;
}

</style>
