<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { getApiErrorMessage } from "@/api/client";
import {
  aiOrganizeApi,
  cloudToolsApi,
  localUploadApi,
  quarkTVApi,
  type AIOrganizeConfig,
  type CloudTool115Status,
  type LocalUploadMapping,
  type QuarkTVAccount,
  type QuarkTVBinding,
  type QuarkTVStatus,
} from "@/api/cloudTools";
import { confirm } from "@/composables/useConfirm";
import { toast } from "@/composables/useToast";
import { useSettingsLoad } from "@/composables/useSettingsLoad";
import AppButton from "@/components/base/AppButton.vue";
import AppModal from "@/components/base/AppModal.vue";
import ProxyToolsPanel from "@/components/admin/ProxyToolsPanel.vue";
import QuarkTVBindModal from "@/components/admin/QuarkTVBindModal.vue";
import "@/styles/admin-shared.css";

const props = withDefaults(defineProps<{ searchOpen?: boolean }>(), { searchOpen: false });
const emit = defineEmits<{ "update:searchOpen": [boolean] }>();

const { runLoad } = useSettingsLoad();

const searchQuery = ref("");
const cardTitles = ["Emby 反代", "飞牛影视反代", "115 网盘 STRM 增强方案", "从服务器上传", "AI 辅助增强工具", "夸克 STRM 播放接管"];

function matches(title: string) {
  const q = searchQuery.value.trim().toLowerCase();
  return !q || title.toLowerCase().includes(q);
}

const hasMatch = computed(() => {
  const q = searchQuery.value.trim().toLowerCase();
  return !q || cardTitles.some((t) => t.toLowerCase().includes(q));
});

function closeSearch() {
  searchQuery.value = "";
  emit("update:searchOpen", false);
}

const status = ref<CloudTool115Status>({ enabled: false, cache_count: 0, available: false });
const saving = ref(false);
const clearing = ref(false);

const localEnabled = ref(false);
const localMappings = ref<LocalUploadMapping[]>([]);
const localSaving = ref(false);
const mappingOpen = ref(false);
const newMappingName = ref("");
const newMappingPath = ref("");

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

const qtvStatus = ref<QuarkTVStatus>({ enabled: false, available: false, bindings: [] });
const qtvSaving = ref(false);
const qtvAccounts = ref<QuarkTVAccount[]>([]);
const qtvBindOpen = ref(false);
const qtvManageOpen = ref(false);
const qtvUnbindingID = ref<number | null>(null);

async function load() {
  await runLoad(async () => {
    const [st, lu, ai, qtv] = await Promise.all([
      cloudToolsApi.status115(),
      localUploadApi.getConfig().catch(() => ({ enabled: false, mappings: [] })),
      aiOrganizeApi.getConfig(),
      quarkTVApi.status().catch(() => ({ enabled: false, available: false, bindings: [] })),
    ]);
    status.value = st;
    localEnabled.value = lu.enabled;
    localMappings.value = lu.mappings;
    aiConfig.value = ai;
    qtvStatus.value = qtv;
  }, "加载网盘工具状态失败");
}

function openAISettings(pendingEnable = false) {
  enableAfterSave.value = pendingEnable;
  aiDraft.value = { ...aiConfig.value };
  aiSettingsOpen.value = true;
}

function closeAISettings() {
  aiSettingsOpen.value = false;
  enableAfterSave.value = false;
}

function aiConfigComplete(config: AIOrganizeConfig) {
  return Boolean(config.base_url.trim() && config.api_key.trim() && config.model.trim());
}

async function toggleAIEnabled() {
  if (!aiConfig.value.enabled && !aiConfigComplete(aiConfig.value)) {
    openAISettings(true);
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

async function testAIConfig() {
  if (!aiConfigComplete(aiDraft.value)) {
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

async function saveAIConfig() {
  if (!aiConfigComplete(aiDraft.value)) {
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
    title: "清空映射数据？",
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
    toast.error(getApiErrorMessage(e, "清空映射数据失败"));
  } finally {
    clearing.value = false;
  }
}

async function toggleQuarkTV() {
  if (!qtvStatus.value.enabled && qtvStatus.value.bindings.length === 0) {
    await openQuarkTVBind();
    toast.info("请先选择夸克账号并扫码绑定 TV 账号");
    return;
  }
  qtvSaving.value = true;
  const next = !qtvStatus.value.enabled;
  try {
    await quarkTVApi.setEnabled(next);
    qtvStatus.value.enabled = next;
    toast.success(next ? "已启用：夸克播放请求改走 TV 302 直链" : "已停用：夸克播放恢复网页代理");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "修改开关失败"));
  } finally {
    qtvSaving.value = false;
  }
}

function openQuarkTVManage() {
  qtvManageOpen.value = true;
}

function closeQuarkTVManage() {
  qtvManageOpen.value = false;
}

async function openQuarkTVBind() {
  try {
    const res = await quarkTVApi.accounts();
    const boundIDs = new Set(qtvStatus.value.bindings.map((b) => b.account_id));
    qtvAccounts.value = res.accounts.filter((a) => !boundIDs.has(a.id));
    if (qtvAccounts.value.length === 0) {
      toast.error(res.accounts.length === 0 ? "请先添加并启用夸克网盘账号" : "所有夸克账号均已绑定");
      return;
    }
    qtvManageOpen.value = false;
    qtvBindOpen.value = true;
  } catch (e) {
    toast.error(getApiErrorMessage(e, "加载夸克账号失败"));
  }
}

function closeQuarkTVBind() {
  qtvBindOpen.value = false;
}

async function onQuarkTVBound() {
  qtvBindOpen.value = false;
  const st = await quarkTVApi.status().catch(() => ({ enabled: false, available: false, bindings: [] }));
  qtvStatus.value = st;
  if (!st.enabled) {
    qtvSaving.value = true;
    try {
      await quarkTVApi.setEnabled(true);
      qtvStatus.value.enabled = true;
      toast.success("已启用夸克 STRM 播放接管");
    } catch (e) {
      toast.error(getApiErrorMessage(e, "绑定成功但启用失败，请手动开启"));
    } finally {
      qtvSaving.value = false;
    }
  }
}

async function unbindQuarkTV(binding: QuarkTVBinding) {
  const ok = await confirm({
    title: "解绑夸克 TV？",
    message: `将解绑「${binding.account_name}」的夸克 TV 绑定，该账号播放恢复网页代理。`,
    confirmText: "确认解绑",
    cancelText: "取消",
    danger: true,
  }).catch(() => false);
  if (!ok) return;
  qtvUnbindingID.value = binding.account_id;
  try {
    await quarkTVApi.unbind(binding.account_id);
    qtvStatus.value = await quarkTVApi.status();
    toast.success("已解绑");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "解绑失败"));
  } finally {
    qtvUnbindingID.value = null;
  }
}
</script>

<template>
  <div class="cloud-tools">
    <div v-if="searchOpen" class="tool-search">
      <div class="tool-search__mask" @click="closeSearch" />
      <div class="tool-search__box">
        <input v-model="searchQuery" autofocus placeholder="搜索工具，如：飞牛、Emby、反代" @keydown.esc="closeSearch" />
        <button type="button" @click="closeSearch">×</button>
      </div>
    </div>
    <div class="cloud-tools__grid">
      <ProxyToolsPanel :search-query="searchQuery" />
      <article v-show="matches('115 网盘 STRM 增强方案')" class="tool-card" :class="status.enabled ? 'is-enabled' : 'is-disabled'">
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
              {{ clearing ? "清空中…" : "清空映射数据" }}
            </AppButton>
          </div>
        </div>

      </article>

      <!-- 本机上传卡片 -->
      <article v-show="matches('从服务器上传')" class="tool-card" :class="localEnabled ? 'is-enabled' : 'is-disabled'">
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

      <article v-show="matches('AI 辅助增强工具')" class="tool-card" :class="aiConfig.enabled ? 'is-enabled' : 'is-disabled'">
        <span class="tool-card__bar" :class="aiConfig.enabled ? 'is-enabled' : 'is-disabled'" />
        <div class="tool-card__head">
          <div class="tool-card__logo tool-card__ai-logo" aria-hidden="true">AI</div>
          <div class="tool-card__meta">
            <h3 class="tool-card__name">
              AI 辅助增强工具
              <span class="tool-card__tag">通用</span>
            </h3>
            <p class="tool-card__driver">已接入目录整理 · 低置信作品批量补判</p>
          </div>
          <button
            class="check-toggle"
            type="button"
            :class="{ on: aiConfig.enabled }"
            :aria-label="aiConfig.enabled ? '停用 AI 辅助增强工具' : '启用 AI 辅助增强工具'"
            :disabled="aiSaving"
            title="启用 / 停用"
            @click="toggleAIEnabled"
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
          提供可复用的 AI 识别能力，目前已用于目录整理。模型只返回识别结果，
          仍由原功能生成计划并执行，后续可供其他功能接入。
        </p>
        <div class="tool-card__row">
          <div class="tool-card__stat tool-card__stat--model">
            <span class="tool-card__num">{{ aiConfig.model || "待配置" }}</span>
          </div>
          <div class="tool-card__actions">
            <AppButton variant="secondary" :disabled="aiSaving" @click="openAISettings(false)">
              配置模型参数
            </AppButton>
          </div>
        </div>
      </article>

      <!-- 夸克 STRM 播放接管卡片 -->
      <article v-show="matches('夸克 STRM 播放接管')" class="tool-card" :class="qtvStatus.enabled ? 'is-enabled' : 'is-disabled'">
        <span class="tool-card__bar" :class="qtvStatus.enabled ? 'is-enabled' : 'is-disabled'" />
        <div class="tool-card__head">
          <img class="tool-card__logo" src="/logos/quark.png" alt="夸克" />
          <div class="tool-card__meta">
            <h3 class="tool-card__name">
              夸克 STRM 播放接管
              <span class="tool-card__tag">夸克网盘</span>
              <span class="tool-card__tag tool-card__tag--warn">实验性</span>
            </h3>
            <p class="tool-card__driver">作用于夸克网盘 · STRM 播放请求走 TV 302 直链</p>
          </div>
          <button
            class="check-toggle"
            type="button"
            :class="{ on: qtvStatus.enabled }"
            :aria-label="qtvStatus.enabled ? '停用夸克 STRM 播放接管' : '启用夸克 STRM 播放接管'"
            :disabled="qtvSaving || !qtvStatus.available"
            title="启用 / 停用"
            @click="toggleQuarkTV"
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
          开启后，夸克网盘账号的 STRM 播放请求改走夸克 TV 的 302 直链，由夸克转码播放，画质会明显下降，且存在部分字幕不可用问题，请根据需要开启或关闭。
        </p>
        <div class="tool-card__row">
          <div class="tool-card__stat">
            <span class="tool-card__num">{{ qtvStatus.bindings.length }}</span>
            <span class="tool-card__label">个绑定账号</span>
          </div>
          <AppButton variant="secondary" :disabled="qtvSaving" @click="openQuarkTVManage">
            账号绑定设置
          </AppButton>
        </div>
      </article>
    </div>
    <div v-if="searchOpen && !hasMatch" class="tool-search__empty">没有找到相关工具</div>

    <AppModal :open="mappingOpen" title="本机上传 · 目录映射设置" size="md" @close="closeMappings">
      <p class="local-mapping-tip">
        在 docker-compose 中先映射宿主机目录，再按容器内路径添加标签。
        前台从本机上传时按标签浏览，不会暴露服务器其他路径。
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

    <AppModal :open="aiSettingsOpen" title="AI 辅助增强工具 · 模型设置" size="md" @close="closeAISettings">
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
      <p class="local-mapping-hint">同一目录树在 24 小时内重新生成计划时会复用识别结果。</p>
      <template #footer>
        <AppButton class="ai-settings-test" variant="secondary" :disabled="aiTesting || aiSaving" @click="testAIConfig">
          {{ aiTesting ? "测试中…" : "测试连接" }}
        </AppButton>
        <AppButton variant="secondary" :disabled="aiSaving" @click="closeAISettings">取消</AppButton>
        <AppButton variant="primary" :disabled="aiSaving" @click="saveAIConfig">
          {{ aiSaving ? "保存中…" : enableAfterSave ? "保存并启用" : "保存" }}
        </AppButton>
      </template>
    </AppModal>

    <AppModal :open="qtvManageOpen" title="夸克 STRM 播放接管 · 账号绑定" size="md" @close="closeQuarkTVManage">
      <div v-if="qtvStatus.bindings.length" class="qtv-list">
        <div v-for="b in qtvStatus.bindings" :key="b.account_id" class="qtv-item">
          <div class="qtv-item__main">
            <strong>{{ b.account_name }}</strong>
            <span>TV 账号：{{ b.tv_nickname || "未知" }}</span>
          </div>
          <AppButton variant="danger" :disabled="qtvUnbindingID === b.account_id" @click="unbindQuarkTV(b)">
            {{ qtvUnbindingID === b.account_id ? "解绑中…" : "解绑" }}
          </AppButton>
        </div>
      </div>
      <div v-else class="qtv-empty">还没有绑定账号</div>
      <template #footer>
        <div class="modal-footer-center">
          <AppButton variant="primary" @click="openQuarkTVBind">添加绑定</AppButton>
        </div>
      </template>
    </AppModal>

    <QuarkTVBindModal
      :open="qtvBindOpen"
      :accounts="qtvAccounts"
      @close="closeQuarkTVBind"
      @bound="onQuarkTVBound"
    />
  </div>
</template>

<style scoped>
.tool-search__mask {
  position: fixed;
  inset: 0;
  z-index: var(--z-modal);
  background: rgba(15, 23, 42, 0.35);
}
.tool-search__box {
  position: fixed;
  top: 140px;
  left: 50%;
  transform: translateX(-50%);
  z-index: calc(var(--z-modal) + 1);
  display: flex;
  align-items: center;
  gap: 8px;
  width: min(520px, calc(100vw - 40px));
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-pop);
  padding: 12px 16px;
}
.tool-search__box input {
  flex: 1;
  border: none;
  outline: none;
  background: transparent;
  font-size: 15px;
  color: var(--text);
}
.tool-search__box button {
  border: none;
  background: transparent;
  color: var(--text-muted);
  font-size: 16px;
  padding: 2px 6px;
  border-radius: var(--radius-sm);
}
.tool-search__box button:hover {
  background: var(--border-soft);
  color: var(--text);
}
.tool-search__empty {
  margin-top: 16px;
  text-align: center;
  color: var(--text-muted);
  font-size: 14px;
  padding: 40px 0;
}

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
.tool-card__ai-logo {
  display: grid;
  place-items: center;
  color: #fff;
  font-size: 17px;
  font-weight: 700;
  letter-spacing: -0.5px;
  background: linear-gradient(145deg, #7167e8, #3f8eea);
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
.tool-card__stat--model {
  min-width: 0;
}
.tool-card__stat--model .tool-card__num {
  max-width: 230px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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
.ai-settings-test {
  margin-right: auto;
}

.qtv-list {
  display: grid;
  gap: 8px;
  max-height: 340px;
  overflow-y: auto;
}
.qtv-item {
  display: flex;
  align-items: center;
  gap: 12px;
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-md);
  padding: 11px 13px;
  background: var(--surface-sunken);
}
.qtv-item__main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.qtv-item__main strong {
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.qtv-item__main span {
  font-size: 12px;
  color: var(--text-muted);
}
.qtv-empty {
  padding: 28px 0;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}
.modal-footer-center {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
}

</style>
