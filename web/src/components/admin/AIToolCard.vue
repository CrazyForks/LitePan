<script setup lang="ts">
import { onMounted, ref } from "vue";
import { getApiErrorMessage } from "@/api/client";
import { aiOrganizeApi, type AIOrganizeConfig } from "@/api/cloudTools";
import { toast } from "@/composables/useToast";
import AppButton from "@/components/base/AppButton.vue";
import AppModal from "@/components/base/AppModal.vue";
import CloudToolCard from "@/components/admin/CloudToolCard.vue";

const props = withDefaults(defineProps<{ searchQuery?: string }>(), { searchQuery: "" });

const aiConfig = ref<AIOrganizeConfig>({
  enabled: false,
  base_url: "https://api.deepseek.com",
  api_key: "",
  model: "deepseek-chat",
});
const aiDraft = ref<AIOrganizeConfig>({ ...aiConfig.value });
const aiSaving = ref(false);
const aiTesting = ref(false);
const aiSettingsOpen = ref(false);
const enableAfterSave = ref(false);

function matches(title: string) {
  const q = props.searchQuery.trim().toLowerCase();
  return !q || title.toLowerCase().includes(q);
}

async function load() {
  try {
    aiConfig.value = await aiOrganizeApi.getConfig();
  } catch (e) {
    toast.error(getApiErrorMessage(e, "加载 AI 配置失败"));
  }
}

onMounted(load);

function openSettings(pendingEnable = false) {
  enableAfterSave.value = pendingEnable;
  aiDraft.value = { ...aiConfig.value };
  aiSettingsOpen.value = true;
}

function closeSettings() {
  aiSettingsOpen.value = false;
  enableAfterSave.value = false;
}

function configComplete(config: AIOrganizeConfig) {
  return Boolean(config.base_url.trim() && config.api_key.trim() && config.model.trim());
}

async function toggleEnabled() {
  if (!aiConfig.value.enabled && !configComplete(aiConfig.value)) {
    openSettings(true);
    toast.info("先填写 API 地址、API Key 和模型名称");
    return;
  }
  aiSaving.value = true;
  try {
    aiConfig.value = await aiOrganizeApi.saveConfig({
      ...aiConfig.value,
      enabled: !aiConfig.value.enabled,
    });
    toast.success(aiConfig.value.enabled ? "AI 辅助增强已启用" : "AI 辅助增强已停用");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "修改开关失败"));
  } finally {
    aiSaving.value = false;
  }
}

async function testConfig() {
  if (!configComplete(aiDraft.value)) {
    toast.error("请先填完模型配置");
    return;
  }
  aiTesting.value = true;
  try {
    await aiOrganizeApi.testConfig(aiDraft.value);
    toast.success("连接成功，模型已正确返回 JSON");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "连接测试失败"));
  } finally {
    aiTesting.value = false;
  }
}

async function saveConfig() {
  if (!configComplete(aiDraft.value)) {
    toast.error("请填完 API 地址、API Key 和模型名称");
    return;
  }
  aiSaving.value = true;
  try {
    aiConfig.value = await aiOrganizeApi.saveConfig({
      ...aiDraft.value,
      enabled: enableAfterSave.value || aiDraft.value.enabled,
    });
    aiSettingsOpen.value = false;
    enableAfterSave.value = false;
    toast.success(aiConfig.value.enabled ? "模型配置已保存并启用" : "模型配置已保存");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存模型配置失败"));
  } finally {
    aiSaving.value = false;
  }
}
</script>

<template>
  <div v-show="matches('AI 辅助增强工具')">
    <CloudToolCard
      :enabled="aiConfig.enabled"
      name="AI 辅助增强工具"
      driver="已接入目录整理 · 低置信作品批量补判"
      logo-text="AI"
      :tags="[{ label: '通用' }]"
      :stat-value="aiConfig.model || '待配置'"
      :compact-stat="true"
    >
      <template #toggle>
        <button
          class="check-toggle"
          type="button"
          :class="{ on: aiConfig.enabled }"
          :aria-label="aiConfig.enabled ? '停用 AI 辅助增强工具' : '启用 AI 辅助增强工具'"
          :disabled="aiSaving"
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
      开启后，目录整理中内置规则识别不出或低置信的作品，将批量交给 AI 模型补判标题与季集，
      仍由原流程校验并生成计划；未配置或调用失败时自动回落内置识别，不影响整理。
      <template #actions>
        <AppButton variant="secondary" :disabled="aiSaving" @click="openSettings(false)">
          配置模型参数
        </AppButton>
      </template>
    </CloudToolCard>

    <AppModal :open="aiSettingsOpen" title="AI 辅助增强工具 · 模型设置" size="md" @close="closeSettings">
      <label class="ai-settings-field">
        <span>API 地址</span>
        <input v-model.trim="aiDraft.base_url" type="url" placeholder="https://api.deepseek.com" />
      </label>
      <label class="ai-settings-field">
        <span>模型名称</span>
        <input v-model.trim="aiDraft.model" type="text" placeholder="例如 deepseek-chat" />
      </label>
      <label class="ai-settings-field">
        <span>API Key</span>
        <input v-model.trim="aiDraft.api_key" type="password" autocomplete="new-password" placeholder="sk-..." />
      </label>
      <p class="ai-settings-hint">同一目录树在 24 小时内重新生成计划时会复用识别结果。</p>
      <template #footer>
        <AppButton class="ai-settings-test" variant="secondary" :disabled="aiTesting || aiSaving" @click="testConfig">
          {{ aiTesting ? "测试中…" : "测试连接" }}
        </AppButton>
        <AppButton variant="secondary" :disabled="aiSaving" @click="closeSettings">取消</AppButton>
        <AppButton variant="primary" :disabled="aiSaving" @click="saveConfig">
          {{ aiSaving ? "保存中…" : enableAfterSave ? "保存并启用" : "保存" }}
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

.ai-settings-field {
  display: grid;
  gap: 6px;
  margin-top: 12px;
  color: var(--text-regular);
  font-size: 13px;
  font-weight: 600;
}

.ai-settings-field input {
  width: 100%;
  box-sizing: border-box;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 9px 11px;
  font-size: 13px;
  background: var(--surface);
  color: var(--text);
}

.ai-settings-field input:focus {
  outline: none;
  border-color: var(--primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--primary) 14%, transparent);
}

.ai-settings-hint {
  margin: 10px 0 0;
  font-size: 12px;
  color: var(--text-muted);
}

.ai-settings-test {
  margin-right: auto;
}
</style>
