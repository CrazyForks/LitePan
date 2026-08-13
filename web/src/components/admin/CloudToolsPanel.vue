<script setup lang="ts">
import { onMounted, ref } from "vue";
import { getApiErrorMessage } from "@/api/client";
import { cloudToolsApi, type CloudTool115Status } from "@/api/cloudTools";
import { confirm } from "@/composables/useConfirm";
import { toast } from "@/composables/useToast";
import { useSettingsLoad } from "@/composables/useSettingsLoad";
import AppButton from "@/components/base/AppButton.vue";
import "@/styles/admin-shared.css";

const { runLoad } = useSettingsLoad();

const status = ref<CloudTool115Status>({ enabled: false, cache_count: 0, available: false });
const saving = ref(false);
const clearing = ref(false);

async function load() {
  await runLoad(async () => {
    status.value = await cloudToolsApi.status115();
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
          不用一层层翻目录，请求更少、更不容易触发限制。
          生成的 STRM、元数据同步、排除规则和原来完全一致；配了分支的任务不受影响，仍按分支的方式执行。
        </p>

        <div class="tool-card__row">
          <div class="tool-card__stat">
            <span class="tool-card__num">{{ status.cache_count.toLocaleString("zh-CN") }}</span>
            <span class="tool-card__label">路径缓存目录</span>
          </div>
          <div class="tool-card__actions">
            <AppButton variant="danger" :disabled="clearing" @click="clearCache">
              {{ clearing ? "清空中…" : "清空路径映射表" }}
            </AppButton>
          </div>
        </div>

      </article>

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

</style>
