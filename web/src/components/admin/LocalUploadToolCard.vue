<script setup lang="ts">
import { onMounted, ref } from "vue";
import { getApiErrorMessage } from "@/api/client";
import { localUploadApi, type LocalUploadMapping } from "@/api/cloudTools";
import { toast } from "@/composables/useToast";
import AppButton from "@/components/base/AppButton.vue";
import AppModal from "@/components/base/AppModal.vue";
import CloudToolCard from "@/components/admin/CloudToolCard.vue";

const props = withDefaults(defineProps<{ searchQuery?: string }>(), { searchQuery: "" });

const localEnabled = ref(false);
const localMappings = ref<LocalUploadMapping[]>([]);
const localSaving = ref(false);
const mappingOpen = ref(false);
const newMappingName = ref("");
const newMappingPath = ref("");

function matches(title: string) {
  const q = props.searchQuery.trim().toLowerCase();
  return !q || title.toLowerCase().includes(q);
}

async function load() {
  try {
    const res = await localUploadApi.getConfig();
    localEnabled.value = res.enabled;
    localMappings.value = res.mappings;
  } catch {
    localEnabled.value = false;
    localMappings.value = [];
  }
}

onMounted(load);

async function toggleEnabled() {
  localSaving.value = true;
  const next = !localEnabled.value;
  try {
    const res = await localUploadApi.saveConfig({ enabled: next, mappings: localMappings.value });
    localEnabled.value = res.enabled;
    toast.success(
      res.enabled
        ? "已启用：前台「新建 → 上传」将提供从服务器上传"
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
</script>

<template>
  <div v-show="matches('从服务器上传')">
    <CloudToolCard
      :enabled="localEnabled"
      name="从服务器上传"
      driver="作用于全部网盘账号 · 上传面板双来源"
      logo-src="/logos/local.png"
      logo-alt="本机"
      :tags="[{ label: '通用' }]"
      :stat-value="localMappings.length"
      stat-label="映射目录"
    >
      <template #toggle>
        <button
          class="check-toggle"
          type="button"
          :class="{ on: localEnabled }"
          :aria-label="localEnabled ? '停用从服务器上传' : '启用从服务器上传'"
          :disabled="localSaving"
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
      </template>
      开启后，前台「新建 → 上传」面板提供<strong>从服务器上传</strong>：
      选择服务器映射目录中的文件或文件夹，上传到当前网盘目录并保留文件夹结构；从访问机上传保持不变。
      <template #actions>
        <AppButton variant="secondary" :disabled="localSaving" @click="openMappings">
          目录映射设置
        </AppButton>
      </template>
    </CloudToolCard>

    <AppModal :open="mappingOpen" title="从服务器上传 · 目录映射设置" size="md" @close="closeMappings">
      <p class="local-mapping-tip">
        在 docker-compose 中先映射宿主机目录，再按容器内路径添加标签。
        前台从服务器上传时按标签浏览，不会暴露服务器其他路径。
      </p>
      <div class="local-mapping-list">
        <div v-for="m in localMappings" :key="m.name" class="local-mapping-item">
          <span class="local-mapping-item__name">{{ m.name }}</span>
          <span class="local-mapping-item__path">{{ m.path }}</span>
          <button class="local-mapping-item__del" type="button" title="删除" @click="removeMapping(m.name)">✕</button>
        </div>
        <div v-if="localMappings.length === 0" class="local-mapping-empty">还没有映射目录</div>
      </div>
      <div class="local-mapping-add">
        <input v-model="newMappingName" type="text" placeholder="标签名，如：媒体库" />
        <input v-model="newMappingPath" type="text" placeholder="容器内路径，如：/app/data/updatefiles" />
        <AppButton variant="primary" @click="addMapping">添加</AppButton>
      </div>
      <p class="local-mapping-hint">示例：<code>- /vol1/1000/updatefiles:/app/data/updatefiles</code></p>
      <template #footer>
        <AppButton variant="secondary" @click="closeMappings">取消</AppButton>
        <AppButton variant="primary" :disabled="localSaving" @click="saveMappings">
          {{ localSaving ? "保存中…" : "保存" }}
        </AppButton>
      </template>
    </AppModal>
  </div>
</template>

<style scoped>
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

.local-mapping-hint :deep(code) {
  font-family: ui-monospace, Menlo, monospace;
  background: var(--surface-sunken);
  border: 1px solid var(--border-soft);
  border-radius: 5px;
  padding: 1px 5px;
}
</style>
