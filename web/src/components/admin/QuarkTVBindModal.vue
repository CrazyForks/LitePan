<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { quarkTVApi, type QuarkTVAccount } from "@/api/cloudTools";
import { getApiErrorMessage } from "@/api/client";
import { toast } from "@/composables/useToast";
import AppButton from "@/components/base/AppButton.vue";
import AppModal from "@/components/base/AppModal.vue";
import AppSelect from "@/components/base/AppSelect.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";

const props = defineProps<{
  open: boolean;
  accounts: QuarkTVAccount[];
}>();
const emit = defineEmits<{ close: []; bound: [] }>();

type Phase = "loading" | "waiting" | "success" | "failed" | "expired" | "error";

const phase = ref<Phase>("loading");
const qrImage = ref("");
const message = ref("");
const token = ref("");
const expiresIn = ref(0);
const accountId = ref<number | null>(null);
const binding = ref(false);

let pollTimer: ReturnType<typeof setTimeout> | null = null;
let countdownTimer: ReturnType<typeof setInterval> | null = null;
let errorStreak = 0;

const canStart = computed(() => accountId.value !== null && !binding.value);
const accountOptions = computed(() => props.accounts.map((a) => ({ value: a.id, label: a.name })));

const expireText = computed(() => {
  if (phase.value === "expired") return "二维码已过期";
  if (expiresIn.value <= 0) return "";
  return `二维码剩余有效时间：${expiresIn.value} 秒`;
});

function clearTimers() {
  if (pollTimer) {
    clearTimeout(pollTimer);
    pollTimer = null;
  }
  if (countdownTimer) {
    clearInterval(countdownTimer);
    countdownTimer = null;
  }
}

function startCountdown(seconds: number) {
  if (countdownTimer) clearInterval(countdownTimer);
  expiresIn.value = Math.max(0, seconds || 0);
  if (expiresIn.value <= 0) return;
  countdownTimer = setInterval(() => {
    expiresIn.value = Math.max(0, expiresIn.value - 1);
    if (expiresIn.value <= 0 && phase.value === "waiting") {
      phase.value = "expired";
      message.value = "二维码已过期，请重新获取";
      clearTimers();
    }
  }, 1000);
}

function schedulePoll(delay = 2000) {
  if (pollTimer) clearTimeout(pollTimer);
  pollTimer = setTimeout(() => void poll(), delay);
}

function reset() {
  clearTimers();
  phase.value = "loading";
  qrImage.value = "";
  message.value = "";
  token.value = "";
  expiresIn.value = 0;
  binding.value = false;
  errorStreak = 0;
  accountId.value = props.accounts[0]?.id ?? null;
}

async function start() {
  if (accountId.value === null) {
    toast.error("请先选择夸克账号");
    return;
  }
  clearTimers();
  phase.value = "loading";
  qrImage.value = "";
  message.value = "";
  token.value = "";
  expiresIn.value = 0;
  errorStreak = 0;
  try {
    const res = await quarkTVApi.bindStart(accountId.value);
    if (!res.token || !res.qr_image) throw new Error("获取二维码失败");
    token.value = res.token;
    qrImage.value = res.qr_image.startsWith("data:")
      ? res.qr_image
      : `data:image/jpeg;base64,${res.qr_image}`;
    phase.value = "waiting";
    startCountdown(res.expires_in || 300);
    schedulePoll(2000);
  } catch (e) {
    phase.value = "error";
    message.value = getApiErrorMessage(e, "获取二维码失败");
  }
}

async function poll() {
  if (!token.value) return;
  try {
    const res = await quarkTVApi.bindPoll(token.value);
    errorStreak = 0;
    if (res.status === "success") {
      binding.value = true;
      phase.value = "success";
      clearTimers();
      toast.success("绑定成功，STRM 播放请求将改走夸克 TV 直链");
      emit("bound");
      return;
    }
    if (res.status === "failed" || res.status === "expired") {
      phase.value = res.status === "expired" ? "expired" : "failed";
      message.value = res.message || (res.status === "expired" ? "二维码已过期" : "绑定失败");
      clearTimers();
      return;
    }
    schedulePoll(2000);
  } catch {
    errorStreak += 1;
    if (errorStreak >= 5) {
      phase.value = "error";
      message.value = "网络异常，请重试";
      clearTimers();
      return;
    }
    schedulePoll(3000);
  }
}

function handleClose() {
  clearTimers();
  emit("close");
}

watch(
  () => props.open,
  (open) => {
    if (open) {
      reset();
      void start();
    } else {
      clearTimers();
    }
  },
);

onUnmounted(clearTimers);
</script>

<template>
  <AppModal :open="open" title="夸克 STRM 播放接管 · 账号绑定" size="md" @close="handleClose">
    <label class="qtv-field">
      <span>选择夸克账号</span>
      <AppSelect
        v-model="accountId"
        :options="accountOptions"
        :disabled="binding"
        @update:modelValue="phase === 'waiting' ? start() : undefined"
      />
    </label>

    <div class="qtv-body">
      <div v-if="phase === 'loading'" class="qtv-state qtv-state--loading">
        <BusySpinner :size="28" color="var(--brand)" />
        <span>正在生成二维码...</span>
      </div>

      <div v-else-if="phase === 'waiting' || phase === 'success'" class="qtv-state">
        <img v-if="qrImage" class="qtv-image" :src="qrImage" alt="夸克 TV 扫码二维码" />
        <div class="qtv-hint">请使用夸克 App 扫码，并在手机端确认登录。TV 端与所选夸克账号需为同一账号。</div>
        <div v-if="phase === 'waiting' && expireText" class="qtv-countdown">{{ expireText }}</div>
        <div v-else class="qtv-success">绑定成功</div>
      </div>

      <div v-else class="qtv-state qtv-state--failed">
        <i class="fas fa-circle-exclamation"></i>
        <div class="qtv-result-title">{{ phase === "expired" ? "二维码已过期" : phase === "failed" ? "绑定失败" : "获取二维码失败" }}</div>
        <div class="qtv-hint">{{ message || "请关闭后重试" }}</div>
        <AppButton variant="secondary" @click="start">重新获取</AppButton>
      </div>
    </div>

    <template #footer>
      <AppButton variant="secondary" :disabled="binding" @click="handleClose">取消</AppButton>
      <AppButton variant="primary" :disabled="!canStart" @click="start">
        {{ binding ? "绑定中…" : "获取二维码" }}
      </AppButton>
    </template>
  </AppModal>
</template>

<style scoped>
.qtv-field {
  display: grid;
  gap: 6px;
  color: var(--text-regular);
  font-size: 13px;
  font-weight: 600;
}
.qtv-body {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 300px;
  margin-top: 14px;
}
.qtv-state {
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  text-align: center;
  color: var(--text);
  font-size: 13px;
}
.qtv-state--loading {
  color: var(--text-muted);
}
.qtv-state--failed {
  color: #b91c1c;
}
.qtv-state i {
  font-size: 40px;
}
.qtv-image {
  width: 220px;
  height: 220px;
  border-radius: 8px;
  border: 1px solid var(--border);
  background: #fff;
  padding: 8px;
  box-sizing: border-box;
}
.qtv-hint {
  margin: 0;
  color: var(--text-muted);
  font-size: 13px;
  line-height: 1.5;
}
.qtv-countdown {
  margin: 0;
  color: var(--text-muted);
  font-size: 12px;
}
.qtv-success {
  color: #16a34a;
  font-size: 13px;
  font-weight: 600;
}
.qtv-result-title {
  color: var(--text);
  font-size: 15px;
  font-weight: 600;
}
</style>
