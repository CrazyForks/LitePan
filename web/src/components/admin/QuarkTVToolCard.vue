<script setup lang="ts">
import { onMounted, ref } from "vue";
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
import CloudToolCard from "@/components/admin/CloudToolCard.vue";
import QuarkTVBindModal from "@/components/admin/QuarkTVBindModal.vue";

const props = withDefaults(defineProps<{ searchQuery?: string }>(), { searchQuery: "" });

const qtvStatus = ref<QuarkTVStatus>({ enabled: false, available: false, bindings: [] });
const qtvSaving = ref(false);
const qtvAccounts = ref<QuarkTVAccount[]>([]);
const qtvBindOpen = ref(false);
const qtvManageOpen = ref(false);
const qtvUnbindingID = ref<number | null>(null);

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
            <span>TV 账号：{{ b.tv_nickname || "未知" }}</span>
          </div>
          <AppButton variant="danger" :disabled="qtvUnbindingID === b.account_id" @click="unbind(b)">
            {{ qtvUnbindingID === b.account_id ? "解绑中…" : "解绑" }}
          </AppButton>
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
