<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { uploadApi } from "@/api/upload";
import { toast } from "@/composables/useToast";

const props = defineProps<{
  open: boolean;
  serverConcurrency: number;
}>();

const emit = defineEmits<{
  "update:serverConcurrency": [number];
  close: [];
}>();

const loading = ref(true);
const saving = ref(false);
const concurrency = ref(3);
const minLimit = ref(1);
const maxLimit = ref(5);

async function loadRuntime() {
  loading.value = true;
  try {
    const data = await uploadApi.getRuntime();
    concurrency.value = data.concurrency;
    minLimit.value = data.concurrency_min ?? 1;
    maxLimit.value = data.concurrency_max ?? 5;
    emit("update:serverConcurrency", data.concurrency);
  } catch {
    concurrency.value = props.serverConcurrency || 3;
  } finally {
    loading.value = false;
  }
}

async function applyConcurrency(next: number) {
  if (next < minLimit.value || next > maxLimit.value || saving.value) return;
  const prev = concurrency.value;
  concurrency.value = next;
  saving.value = true;
  try {
    const data = await uploadApi.updateRuntime(next);
    concurrency.value = data.concurrency;
    emit("update:serverConcurrency", data.concurrency);
    toast.success("上传并发已更新");
  } catch (e) {
    concurrency.value = prev;
    toast.error(e instanceof Error ? e.message : "更新上传并发失败");
  } finally {
    saving.value = false;
  }
}

function step(delta: number) {
  void applyConcurrency(concurrency.value + delta);
}

watch(
  () => props.open,
  (open) => {
    if (open) void loadRuntime();
  },
);

onMounted(() => {
  if (props.open) void loadRuntime();
});
</script>

<template>
  <div class="upload-settings-panel" role="dialog" aria-label="上传设置" @click.stop>
    <div class="upload-settings-panel__head">
      <span class="upload-settings-panel__title">上传设置</span>
      <button type="button" class="upload-settings-panel__close" aria-label="关闭" @click="emit('close')">
        ×
      </button>
    </div>

    <div v-if="loading" class="upload-settings-panel__loading">加载中…</div>
    <section v-else class="upload-settings-panel__body">
      <p class="upload-settings-panel__hint">同一时间最多进行几个上传，超出的任务会在面板中排队等待。</p>
      <div class="upload-settings-panel__stepper">
        <button
          type="button"
          class="upload-settings-panel__step"
          :disabled="saving || concurrency <= minLimit"
          aria-label="减少并发"
          @click="step(-1)"
        >
          −
        </button>
        <div class="upload-settings-panel__value-wrap">
          <span class="upload-settings-panel__value">{{ concurrency }}</span>
          <span class="upload-settings-panel__unit">个任务</span>
        </div>
        <button
          type="button"
          class="upload-settings-panel__step"
          :disabled="saving || concurrency >= maxLimit"
          aria-label="增加并发"
          @click="step(1)"
        >
          +
        </button>
      </div>
      <p class="upload-settings-panel__range">可调范围 {{ minLimit }}–{{ maxLimit }}，修改后立即生效</p>
    </section>
  </div>
</template>

<style scoped>
.upload-settings-panel {
  position: absolute;
  bottom: calc(100% + 10px);
  left: 0;
  width: 248px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 12px;
  box-shadow: var(--shadow-pop);
  z-index: 130;
  overflow: hidden;
}

.upload-settings-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border-bottom: 1px solid var(--border-soft);
  background: var(--panel-head-bg, var(--surface-sunken));
}

.upload-settings-panel__title {
  font-size: 13px;
  font-weight: 700;
  color: var(--text);
}

.upload-settings-panel__close {
  border: none;
  background: transparent;
  color: var(--text-muted);
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 6px;
}

.upload-settings-panel__close:hover {
  background: var(--surface-hover);
  color: var(--text);
}

.upload-settings-panel__loading {
  padding: 20px 12px;
  font-size: 13px;
  color: var(--text-muted);
  text-align: center;
}

.upload-settings-panel__body {
  padding: 12px;
}

.upload-settings-panel__hint {
  margin: 0 0 12px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-muted);
}

.upload-settings-panel__stepper {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
}

.upload-settings-panel__step {
  width: 34px;
  height: 34px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface-sunken);
  color: var(--text);
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
  transition: background 0.15s ease, border-color 0.15s ease;
}

.upload-settings-panel__step:hover:not(:disabled) {
  background: var(--surface-hover);
  border-color: color-mix(in srgb, var(--brand) 35%, var(--border));
}

.upload-settings-panel__step:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.upload-settings-panel__value-wrap {
  min-width: 72px;
  text-align: center;
}

.upload-settings-panel__value {
  display: block;
  font-size: 22px;
  font-weight: 700;
  color: var(--text);
  line-height: 1.1;
}

.upload-settings-panel__unit {
  display: block;
  margin-top: 2px;
  font-size: 11px;
  color: var(--text-muted);
}

.upload-settings-panel__range {
  margin: 10px 0 0;
  font-size: 11px;
  color: var(--text-muted);
  text-align: center;
}
</style>
