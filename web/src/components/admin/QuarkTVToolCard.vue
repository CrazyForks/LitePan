<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { getApiErrorMessage } from "@/api/client";
import {
  quarkTVApi,
  type QuarkTVAccount,
  type QuarkTVBinding,
  type QuarkTVStatus,
} from "@/api/cloudTools";
import { confirm } from "@/composables/useConfirm";
import { toast } from "@/composables/useToast";
import AppButton from "@/components/base/AppButton.vue";
import AppModal from "@/components/base/AppModal.vue";
import AppSelect from "@/components/base/AppSelect.vue";
import SettingsBoolSegment from "@/components/admin/SettingsBoolSegment.vue";
import CloudToolCard from "@/components/admin/CloudToolCard.vue";
import QuarkTVBindModal from "@/components/admin/QuarkTVBindModal.vue";

const props = withDefaults(defineProps<{ searchQuery?: string }>(), { searchQuery: "" });

const qtvStatus = ref<QuarkTVStatus>({ enabled: false, available: false, bindings: [] });
const qtvSaving = ref(false);
const qtvAccounts = ref<QuarkTVAccount[]>([]);
const qtvBindOpen = ref(false);
const qtvManageOpen = ref(false);
const qtvUnbindingID = ref<number | null>(null);
const qtvSettingsOpen = ref(false);
const qtvSettingsSaving = ref(false);
const qtvEditingBinding = ref<QuarkTVBinding | null>(null);
const qtvResolution = ref("4k");
const qtvAllowDolby = ref(false);

const settingsChanged = computed(
  () =>
    !!qtvEditingBinding.value &&
    (qtvResolution.value !== normalizeResolutionForUI(qtvEditingBinding.value.preferred_resolution || "auto") ||
      qtvAllowDolby.value !== !!qtvEditingBinding.value.allow_dolby),
);

const settingsResolutionOptions = computed(() => {
  const binding = qtvEditingBinding.value;
  const advancedDisabled = binding ? !supportsAdvancedQuality(binding.membership) : false;
  return [
    { value: "4k", label: "4K", disabled: advancedDisabled, tag: "SVIP" },
    { value: "super", label: "超清", disabled: advancedDisabled, tag: "SVIP" },
    { value: "high", label: "高清", disabled: advancedDisabled, tag: "SVIP" },
    { value: "low", label: "流畅" },
  ];
});

const dolbyControlDisabled = computed(() => {
  const binding = qtvEditingBinding.value;
  if (!binding) return false;
  return !supportsAdvancedQuality(binding.membership) && !qtvAllowDolby.value;
});

function matches(title: string) {
  const q = props.searchQuery.trim().toLowerCase();
  return !q || title.toLowerCase().includes(q);
}

async function load() {
  qtvStatus.value = await quarkTVApi.status().catch(() => ({ enabled: false, available: false, bindings: [] }));
}

onMounted(() => {
  void load();
});

async function toggleEnabled() {
  if (!qtvStatus.value.enabled && qtvStatus.value.bindings.length === 0) {
    await openBind();
    toast.info("请先选择夸克账号并扫码绑定 TV 账号");
    return;
  }
  qtvSaving.value = true;
  const next = !qtvStatus.value.enabled;
  try {
    await quarkTVApi.setEnabled(next);
    qtvStatus.value.enabled = next;
    toast.success(next ? "已启用：夸克播放请求改走 TV 302 直链" : "已停用：夸克播放恢复夸克驱动本机代理");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "修改开关失败"));
  } finally {
    qtvSaving.value = false;
  }
}

function openManage() {
  qtvManageOpen.value = true;
}

function closeManage() {
  qtvManageOpen.value = false;
}

function openSettings(binding: QuarkTVBinding) {
  qtvEditingBinding.value = binding;
  qtvResolution.value = normalizeResolutionForUI(binding.preferred_resolution || "auto");
  qtvAllowDolby.value = !!binding.allow_dolby;
  qtvSettingsOpen.value = true;
}

function closeSettings() {
  if (qtvSettingsSaving.value) return;
  qtvSettingsOpen.value = false;
  qtvEditingBinding.value = null;
}

function resolutionLabel(value: string) {
  const normalized = normalizeResolutionForUI(value);
  return settingsResolutionOptions.value.find((item) => item.value === normalized)?.label || "4K";
}

function displayMembership(binding: QuarkTVBinding | null) {
  if (!binding) return "未知";
  return binding.membership?.trim() || "未知";
}

function supportsAdvancedQuality(membership: string) {
  const value = membership.trim().toUpperCase();
  return value === "SVIP" || value === "SVIP+" || value === "88VIP";
}

function normalizeResolutionForUI(value: string) {
  switch ((value || "").trim().toLowerCase()) {
    case "4k":
    case "auto":
      return "4k";
    case "2k":
    case "super":
      return "super";
    case "high":
      return "high";
    case "normal":
    case "low":
    default:
      return "low";
  }
}

async function openBind() {
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

function closeBind() {
  qtvBindOpen.value = false;
}

async function onBound() {
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

async function unbind(binding: QuarkTVBinding) {
  const ok = await confirm({
    title: "解绑夸克 TV？",
    message: `将解绑「${binding.account_name}」的夸克 TV 绑定，该账号播放恢复夸克驱动本机代理。`,
    confirmText: "确认解绑",
    cancelText: "取消",
    danger: true,
  }).catch(() => false);
  if (!ok) return;
  qtvUnbindingID.value = binding.account_id;
  try {
    await quarkTVApi.unbind(binding.account_id);
    await load();
    toast.success("已解绑");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "解绑失败"));
  } finally {
    qtvUnbindingID.value = null;
  }
}

async function saveSettings() {
  if (!qtvEditingBinding.value) return;
  qtvSettingsSaving.value = true;
  try {
    const updated = await quarkTVApi.updateBindingSettings({
      account_id: qtvEditingBinding.value.account_id,
      preferred_resolution: qtvResolution.value,
      allow_dolby: qtvAllowDolby.value,
    });
    qtvStatus.value.bindings = qtvStatus.value.bindings.map((item) =>
      item.account_id === updated.account_id ? updated : item,
    );
    qtvEditingBinding.value = updated;
    qtvSettingsOpen.value = false;
    toast.success("播放设置已保存");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存播放设置失败"));
  } finally {
    qtvSettingsSaving.value = false;
  }
}
</script>

<template>
  <div v-show="matches('夸克 STRM 播放接管')">
    <CloudToolCard
      :enabled="qtvStatus.enabled"
      name="夸克 STRM 播放接管"
      driver="作用于夸克网盘 · STRM 播放请求走 TV 302 直链"
      logo-src="/logos/quark.png"
      logo-alt="夸克"
      :tags="[
        { label: '夸克网盘' },
        { label: '实验性', variant: 'warn' },
      ]"
      :stat-value="qtvStatus.bindings.length"
      stat-label="个绑定账号"
    >
      <template #toggle>
        <button
          class="check-toggle"
          type="button"
          :class="{ on: qtvStatus.enabled }"
          :aria-label="qtvStatus.enabled ? '停用夸克 STRM 播放接管' : '启用夸克 STRM 播放接管'"
          :disabled="qtvSaving || !qtvStatus.available"
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
      开启后，夸克网盘账号的 STRM 播放请求改走夸克 TV 的 302 直链，由夸克转码播放，画质会明显下降，且存在部分字幕不可用问题，请根据需要开启或关闭。
      <template #actions>
        <AppButton variant="secondary" :disabled="qtvSaving" @click="openManage">
          账号绑定设置
        </AppButton>
      </template>
    </CloudToolCard>

    <AppModal :open="qtvManageOpen" title="夸克 STRM 播放接管 · 账号绑定" size="md" @close="closeManage">
      <div v-if="qtvStatus.bindings.length" class="qtv-list">
        <div v-for="b in qtvStatus.bindings" :key="b.account_id" class="qtv-item">
          <div class="qtv-item__main">
            <strong>{{ b.account_name }}</strong>
            <div class="qtv-item__meta">
              <span class="qtv-item__meta-text">
                TV · {{ b.tv_nickname || "未知" }} · {{ resolutionLabel(b.preferred_resolution || "auto") }} ·
                {{ b.allow_dolby ? "杜比开启" : "杜比关闭" }}
              </span>
            </div>
          </div>
          <div class="qtv-item__actions">
            <AppButton variant="secondary" @click="openSettings(b)">播放设置</AppButton>
            <AppButton variant="danger" :disabled="qtvUnbindingID === b.account_id" @click="unbind(b)">
              {{ qtvUnbindingID === b.account_id ? "解绑中…" : "解绑" }}
            </AppButton>
          </div>
        </div>
      </div>
      <div v-else class="qtv-empty">还没有绑定账号</div>
      <template #footer>
        <div class="modal-footer-center">
          <AppButton variant="primary" @click="openBind">添加绑定</AppButton>
        </div>
      </template>
    </AppModal>

    <QuarkTVBindModal
      :open="qtvBindOpen"
      :accounts="qtvAccounts"
      @close="closeBind"
      @bound="onBound"
    />

    <AppModal :open="qtvSettingsOpen" title="夸克 TV 播放设置" size="md" @close="closeSettings">
      <div v-if="qtvEditingBinding" class="qtv-settings">
        <div class="qtv-settings__hint">
          <strong>{{ qtvEditingBinding.account_name }}</strong>
          <span>TV 账号：{{ qtvEditingBinding.tv_nickname || "未知" }} · 当前会员：{{ displayMembership(qtvEditingBinding) }}</span>
        </div>
        <label class="qtv-settings__field">
          <span>清晰度偏好</span>
          <AppSelect v-model="qtvResolution" :options="settingsResolutionOptions" />
        </label>
        <label class="qtv-settings__field">
          <div class="qtv-settings__field-head">
            <span>杜比视界</span>
            <span class="qtv-settings__tag">SVIP 限额</span>
          </div>
          <SettingsBoolSegment
            v-model="qtvAllowDolby"
            label="杜比视界"
            off-label="关闭"
            on-label="开启"
            :disabled="dolbyControlDisabled"
          />
          <small>开启后优先尝试杜比视界；不可用时会自动降级到上面的清晰度偏好。</small>
        </label>
      </div>
      <template #footer>
        <AppButton variant="secondary" :disabled="qtvSettingsSaving" @click="closeSettings">取消</AppButton>
        <AppButton variant="primary" :disabled="qtvSettingsSaving || !settingsChanged" @click="saveSettings">
          {{ qtvSettingsSaving ? "保存中…" : "保存设置" }}
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
  gap: 6px;
}

.qtv-item__main strong {
  font-size: 14px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.qtv-item__meta {
  display: block;
  min-width: 0;
}

.qtv-item__meta-text {
  display: block;
  font-size: 12px;
  color: var(--text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.qtv-item__actions {
  display: flex;
  align-items: center;
  gap: 8px;
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

.qtv-settings {
  display: grid;
  gap: 16px;
}

.qtv-settings__hint {
  display: grid;
  gap: 4px;
  padding: 12px 14px;
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-md);
  background: var(--surface-sunken);
}

.qtv-settings__hint strong {
  font-size: 14px;
  font-weight: 600;
}

.qtv-settings__hint span {
  font-size: 12px;
  color: var(--text-muted);
}

.qtv-settings__field {
  display: grid;
  gap: 8px;
}

.qtv-settings__field > span {
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
}

.qtv-settings__field-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.qtv-settings__tag {
  display: inline-flex;
  align-items: center;
  height: 18px;
  padding: 0 8px;
  border-radius: var(--radius-pill);
  background: color-mix(in srgb, var(--warning) 14%, var(--surface));
  color: #b45309;
  font-size: 11px;
  font-weight: 500;
}

.qtv-settings__field small {
  font-size: 12px;
  color: var(--text-muted);
  line-height: 1.5;
}
</style>
