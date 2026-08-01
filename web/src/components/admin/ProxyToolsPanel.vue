<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { getApiErrorMessage } from "@/api/client";
import {
  fetchEmbyConfig,
  refreshEmbyLibrary,
  saveEmbyConfig,
  testEmbyConfig,
  type EmbyConfig,
} from "@/api/emby";
import {
  fetchFnosConfig,
  saveFnosConfig,
  testFnosConfig,
  type FnosConfig,
} from "@/api/fnos";
import { toast, copyTextToClipboard } from "@/composables/useToast";
import { useSettingsLoad } from "@/composables/useSettingsLoad";
import AppButton from "@/components/base/AppButton.vue";
import AppInput from "@/components/base/AppInput.vue";
import SettingsHelpTooltip from "@/components/admin/SettingsHelpTooltip.vue";
import embyLogo from "@/assets/proxy/embylogo.png";
import fnosLogo from "@/assets/proxy/fnmovielogo.png";
import "@/styles/admin-shared.css";

const { loading, runLoad } = useSettingsLoad();

const embyForm = reactive({
  enabled: false,
  emby_url: "",
  api_key: "",
  proxy_port: "",
  proxy_url: "",
  running: false,
  last_error: "",
});
const embyOriginal = reactive({
  enabled: false,
  emby_url: "",
  api_key: "",
  proxy_port: "",
});

const fnosForm = reactive({
  enabled: false,
  fnos_url: "",
  proxy_port: "",
  strm_path_maps: "",
  strm_dir: "/app/strm",
  proxy_url: "",
  running: false,
  last_error: "",
});
const fnosOriginal = reactive({
  enabled: false,
  fnos_url: "",
  proxy_port: "",
  strm_path_maps: "",
});

const savingEmby = ref(false);
const savingFnos = ref(false);
const testingEmby = ref(false);
const testingFnos = ref(false);
const refreshingEmby = ref(false);

const canTestEmby = computed(() => Boolean(embyForm.emby_url.trim() && embyForm.api_key.trim()));
const canTestFnos = computed(() => Boolean(fnosForm.fnos_url.trim()));
const canRefreshEmby = computed(() => Boolean(embyForm.emby_url.trim() && embyForm.api_key.trim()));

function applyEmbyConfig(config: EmbyConfig) {
  embyForm.enabled = Boolean(config.enabled);
  embyForm.emby_url = config.emby_url || "";
  embyForm.api_key = config.api_key || "";
  embyForm.proxy_port = config.proxy_port || "";
  embyForm.proxy_url = config.proxy_url || "";
  embyForm.running = Boolean(config.running);
  embyForm.last_error = config.last_error || "";
  embyOriginal.enabled = embyForm.enabled;
  embyOriginal.emby_url = embyForm.emby_url;
  embyOriginal.api_key = embyForm.api_key;
  embyOriginal.proxy_port = embyForm.proxy_port;
}

function applyFnosConfig(config: FnosConfig) {
  fnosForm.enabled = Boolean(config.enabled);
  fnosForm.fnos_url = config.fnos_url || "";
  fnosForm.proxy_port = config.proxy_port || "";
  fnosForm.strm_path_maps = config.strm_path_maps || "";
  fnosForm.strm_dir = config.strm_dir || "/app/strm";
  fnosForm.proxy_url = config.proxy_url || "";
  fnosForm.running = Boolean(config.running);
  fnosForm.last_error = config.last_error || "";
  fnosOriginal.enabled = fnosForm.enabled;
  fnosOriginal.fnos_url = fnosForm.fnos_url;
  fnosOriginal.proxy_port = fnosForm.proxy_port;
  fnosOriginal.strm_path_maps = fnosForm.strm_path_maps;
}

async function load() {
  await runLoad(async () => {
    const [emby, fnos] = await Promise.all([fetchEmbyConfig(), fetchFnosConfig()]);
    applyEmbyConfig(emby);
    applyFnosConfig(fnos);
  }, "加载反代配置失败");
}

onMounted(load);

function resolveProxyURL(proxyURL: string, port: string) {
  const p = port.trim();
  if (!p) return proxyURL;
  try {
    const u = new URL(proxyURL || `http://127.0.0.1:${p}`);
    const pageHost = window.location.hostname;
    if (
      pageHost &&
      pageHost !== "127.0.0.1" &&
      pageHost !== "localhost" &&
      (u.hostname === "127.0.0.1" || u.hostname === "localhost")
    ) {
      return `${window.location.protocol}//${pageHost}:${p}`;
    }
    if (!proxyURL) {
      return `${u.protocol}//${u.hostname}:${p}`;
    }
  } catch {}
  return proxyURL;
}

function endpointText(enabled: boolean, running: boolean, proxyURL: string, port: string) {
  const resolved = resolveProxyURL(proxyURL, port);
  if (enabled && running && resolved) return resolved;
  if (enabled && port.trim()) return "已启用，保存后生成入口";
  if (enabled) return "已启用，填写端口后生成入口";
  return "启用并填写端口后生成";
}

async function copyText(text: string) {
  if (!text || text.includes("生成")) {
    toast.error("暂无可复制的反代地址");
    return;
  }
  await copyTextToClipboard(text, {
    successMessage: "已复制反代地址",
    errorMessage: "复制失败，请手动选择地址",
  });
}

async function saveEmby() {
  savingEmby.value = true;
  try {
    applyEmbyConfig(
      await saveEmbyConfig({
        enabled: embyForm.enabled,
        emby_url: embyForm.emby_url,
        api_key: embyForm.api_key,
        proxy_port: embyForm.proxy_port,
      }),
    );
    toast.success("Emby 反代配置已保存");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存失败"));
  } finally {
    savingEmby.value = false;
  }
}

async function saveFnos() {
  savingFnos.value = true;
  try {
    applyFnosConfig(
      await saveFnosConfig({
        enabled: fnosForm.enabled,
        fnos_url: fnosForm.fnos_url,
        proxy_port: fnosForm.proxy_port,
        strm_path_maps: fnosForm.strm_path_maps,
      }),
    );
    toast.success("飞牛影视反代配置已保存");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存失败"));
  } finally {
    savingFnos.value = false;
  }
}

async function testEmby() {
  if (!embyForm.emby_url.trim()) {
    toast.error("请先填写 Emby 地址");
    return;
  }
  if (!embyForm.api_key.trim()) {
    toast.error("请先填写 Emby API Key");
    return;
  }
  testingEmby.value = true;
  try {
    await testEmbyConfig({
      enabled: embyForm.enabled,
      emby_url: embyForm.emby_url,
      api_key: embyForm.api_key,
      proxy_port: embyForm.proxy_port,
    });
    toast.success("Emby 地址与 API Key 验证通过");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "Emby 验证失败"));
  } finally {
    testingEmby.value = false;
  }
}

async function testFnos() {
  if (!fnosForm.fnos_url.trim()) {
    toast.error("请先填写飞牛影视地址");
    return;
  }
  testingFnos.value = true;
  try {
    await testFnosConfig({
      enabled: fnosForm.enabled,
      fnos_url: fnosForm.fnos_url,
      proxy_port: fnosForm.proxy_port,
      strm_path_maps: fnosForm.strm_path_maps,
    });
    toast.success("飞牛影视地址可访问");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "飞牛影视验证失败"));
  } finally {
    testingFnos.value = false;
  }
}

async function refreshEmby() {
  refreshingEmby.value = true;
  try {
    await refreshEmbyLibrary();
    toast.success("已触发 Emby 刷库");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "刷库失败"));
  } finally {
    refreshingEmby.value = false;
  }
}

const embyEndpoint = computed(() =>
  endpointText(embyForm.enabled, embyForm.running, embyForm.proxy_url, embyForm.proxy_port),
);
const fnosEndpoint = computed(() =>
  endpointText(fnosForm.enabled, fnosForm.running, fnosForm.proxy_url, fnosForm.proxy_port),
);
</script>

<template>
  <div class="proxy-tools" :class="{ 'is-loading': loading }">
    <div class="proxy-grid">
      <article class="proxy-card" :class="{ 'is-on': embyForm.enabled }">
        <div class="card-head">
          <div class="brand-block">
            <img class="brand-logo" :src="embyLogo" alt="" width="44" height="44" />
            <div class="brand-meta">
              <h2>
                Emby 反代
                <SettingsHelpTooltip title="Emby 反代说明">
                  <p>在播放器和 Emby 之间加一层：播放 STRM 时，把真实的网盘播放地址直接交给播放器。</p>
                  <p>Infuse 等播放器连下方的「反代入口」，就能正常播放 STRM 影片。</p>
                </SettingsHelpTooltip>
              </h2>
              <span class="pill" :class="{ live: embyForm.running }">
                <span class="dot" />
                <span v-if="embyForm.running">运行中 · :{{ embyForm.proxy_port }}</span>
                <span v-else-if="embyForm.enabled">已启用 · 未监听</span>
                <span v-else>未启用</span>
              </span>
            </div>
          </div>
          <button
            class="check-toggle"
            type="button"
            :class="{ on: embyForm.enabled }"
            :aria-label="embyForm.enabled ? '关闭 Emby 反代' : '启用 Emby 反代'"
            @click="embyForm.enabled = !embyForm.enabled"
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

        <div class="endpoint">
          <div class="endpoint-label">
            反代入口（给客户端）
            <SettingsHelpTooltip title="反代入口说明">
              <p>在播放器里添加服务器时，填这个地址。</p>
              <p>注意不是下面的「Emby 地址」，别填混了。</p>
            </SettingsHelpTooltip>
          </div>
          <div class="endpoint-row">
            <div class="endpoint-url" :class="{ muted: !embyForm.running || !embyEndpoint.startsWith('http') }">
              {{ embyEndpoint }}
            </div>
            <button
              class="ghost-btn"
              type="button"
              :disabled="!embyForm.running || !embyEndpoint.startsWith('http')"
              @click="copyText(embyEndpoint)"
            >
              复制
            </button>
          </div>
        </div>

        <div class="params">
          <div class="field">
            <div class="field-label">
              Emby 地址<span class="req">*</span>
              <SettingsHelpTooltip title="Emby 地址说明">
                <p>你的 Emby 服务器地址，例如 http://192.168.1.10:8096。</p>
                <p>给 LitePan 连 Emby 用的，播放器里不要填这个。</p>
              </SettingsHelpTooltip>
            </div>
            <div class="control">
              <AppInput
                v-model="embyForm.emby_url"
                placeholder="http://192.168.1.10:8096"
                autocomplete="off"
                ignore-autofill
              />
              <AppButton
                type="button"
                class="test-btn"
                variant="secondary"
                :disabled="testingEmby || !canTestEmby"
                @click="testEmby"
              >
                {{ testingEmby ? "…" : "测试" }}
              </AppButton>
            </div>
          </div>

          <div class="field">
            <div class="field-label">
              API Key<span class="req">*</span>
              <SettingsHelpTooltip title="API Key 说明">
                <p>在 Emby 后台「API 密钥」里生成一个，粘贴到这里，用来连接 Emby 和刷库。</p>
              </SettingsHelpTooltip>
            </div>
            <div class="control">
              <AppInput
                v-model="embyForm.api_key"
                placeholder="Emby API Key"
                autocomplete="off"
                ignore-autofill
              />
            </div>
          </div>

          <div class="field">
            <div class="field-label">
              反代端口
              <SettingsHelpTooltip title="反代端口说明">
                <p>反代用的端口，随便选一个没被占用的数字就行。</p>
                <p>留空则不启动反代。</p>
              </SettingsHelpTooltip>
            </div>
            <div class="control">
              <div class="mini">
                <AppInput
                  v-model="embyForm.proxy_port"
                  type="number"
                  placeholder="留空不启用"
                />
              </div>
            </div>
          </div>
          <p v-if="embyForm.last_error" class="card-error">{{ embyForm.last_error }}</p>
        </div>

        <div class="card-foot">
          <AppButton
            type="button"
            variant="secondary"
            :disabled="refreshingEmby || !canRefreshEmby"
            @click="refreshEmby"
          >
            {{ refreshingEmby ? "刷库中…" : "刷库" }}
          </AppButton>
          <AppButton type="button" variant="primary" :disabled="savingEmby" @click="saveEmby">
            {{ savingEmby ? "保存中…" : "保存配置" }}
          </AppButton>
        </div>
      </article>

      <article class="proxy-card" :class="{ 'is-on': fnosForm.enabled }">
        <div class="card-head">
          <div class="brand-block">
            <img class="brand-logo" :src="fnosLogo" alt="" width="44" height="44" />
            <div class="brand-meta">
              <h2>
                飞牛影视反代
                <SettingsHelpTooltip title="飞牛影视反代说明">
                  <p>在播放器和飞牛影视之间加一层：播放 STRM 时，把真实的网盘播放地址直接交给播放器。</p>
                  <p>爆米花 / VidHub / SenPlayer 等连下方的「反代入口」，就能正常播放 STRM 影片。</p>
                </SettingsHelpTooltip>
              </h2>
              <span class="pill" :class="{ live: fnosForm.running }">
                <span class="dot" />
                <span v-if="fnosForm.running">运行中 · :{{ fnosForm.proxy_port }}</span>
                <span v-else-if="fnosForm.enabled">已启用 · 未监听</span>
                <span v-else>未启用</span>
              </span>
            </div>
          </div>
          <button
            class="check-toggle"
            type="button"
            :class="{ on: fnosForm.enabled }"
            :aria-label="fnosForm.enabled ? '关闭飞牛反代' : '启用飞牛反代'"
            @click="fnosForm.enabled = !fnosForm.enabled"
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

        <div class="endpoint">
          <div class="endpoint-label">
            反代入口（给客户端）
            <SettingsHelpTooltip title="反代入口说明">
              <p>在播放器里添加飞牛服务器时，填这个地址。</p>
              <p>注意不是下面的「飞牛影视地址」，别填混了。</p>
            </SettingsHelpTooltip>
          </div>
          <div class="endpoint-row">
            <div class="endpoint-url" :class="{ muted: !fnosForm.running || !fnosEndpoint.startsWith('http') }">
              {{ fnosEndpoint }}
            </div>
            <button
              class="ghost-btn"
              type="button"
              :disabled="!fnosForm.running || !fnosEndpoint.startsWith('http')"
              @click="copyText(fnosEndpoint)"
            >
              复制
            </button>
          </div>
        </div>

        <div class="params">
          <div class="field">
            <div class="field-label">
              飞牛影视地址<span class="req">*</span>
              <SettingsHelpTooltip title="飞牛影视地址说明">
                <p>你的飞牛影视地址，端口一般是 8005（不是 NAS 管理页的 5666）。</p>
                <p>给 LitePan 连飞牛用的，播放器里不要填这个。</p>
              </SettingsHelpTooltip>
            </div>
            <div class="control">
              <AppInput
                v-model="fnosForm.fnos_url"
                placeholder="http://192.168.1.50:8005"
                autocomplete="off"
                ignore-autofill
              />
              <AppButton
                type="button"
                class="test-btn"
                variant="secondary"
                :disabled="testingFnos || !canTestFnos"
                @click="testFnos"
              >
                {{ testingFnos ? "…" : "测试" }}
              </AppButton>
            </div>
          </div>

          <div class="field">
            <div class="field-label">
              飞牛 STRM 目录
              <SettingsHelpTooltip title="飞牛 STRM 目录说明">
                <p>把 Docker 里映射到 <code>/app/strm</code> 的左边路径填到这里。</p>
                <p>例：<code>/vol1/1000/Strm/LitePanGO:/app/strm</code> → 填 <code>/vol1/1000/Strm/LitePanGO</code>。</p>
                <p>两边路径相同则可留空。</p>
              </SettingsHelpTooltip>
            </div>
            <div class="control">
              <AppInput
                v-model="fnosForm.strm_path_maps"
                placeholder="/vol1/1000/Strm/LitePanGO"
                autocomplete="off"
                ignore-autofill
              />
            </div>
          </div>

          <div class="field">
            <div class="field-label">
              反代端口
              <SettingsHelpTooltip title="反代端口说明">
                <p>反代用的端口，随便选一个没被占用的数字就行，别和 Emby 反代用同一个。</p>
                <p>留空则不启动反代。</p>
              </SettingsHelpTooltip>
            </div>
            <div class="control">
              <div class="mini">
                <AppInput
                  v-model="fnosForm.proxy_port"
                  type="number"
                  placeholder="如 18997"
                />
              </div>
            </div>
          </div>
          <p v-if="fnosForm.last_error" class="card-error">{{ fnosForm.last_error }}</p>
        </div>

        <div class="card-foot">
          <AppButton type="button" variant="primary" :disabled="savingFnos" @click="saveFnos">
            {{ savingFnos ? "保存中…" : "保存配置" }}
          </AppButton>
        </div>
      </article>
    </div>
  </div>
</template>

<style scoped>
.proxy-tools {
  padding-bottom: 8px;
  color: var(--text);
}
.proxy-tools.is-loading {
  opacity: 0.72;
  pointer-events: none;
}

.proxy-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 18px;
}

.proxy-card {
  position: relative;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 18px;
  box-shadow: var(--shadow-card);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  transition: box-shadow 0.2s ease, border-color 0.2s ease;
}
.proxy-card:hover {
  box-shadow: var(--shadow-pop);
  border-color: color-mix(in srgb, var(--brand) 28%, var(--border));
}
.proxy-card.is-on {
  border-color: color-mix(in srgb, var(--brand) 35%, var(--border));
}
.proxy-card::before {
  content: "";
  position: absolute;
  inset: 0 0 auto 0;
  height: 3px;
  background: var(--brand-gradient);
  opacity: 0.85;
}

.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 18px 20px 12px;
  min-height: 72px;
}
.brand-block {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex: 1;
}
.brand-logo {
  width: 44px;
  height: 44px;
  object-fit: contain;
  flex-shrink: 0;
  display: block;
}
.brand-meta {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 6px;
  min-width: 0;
}
.brand-meta h2 {
  margin: 0;
  font-size: 16px;
  font-weight: 650;
  line-height: 1.2;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.pill {
  display: inline-flex;
  align-items: center;
  align-self: flex-start;
  gap: 6px;
  font-size: 12px;
  font-weight: 550;
  line-height: 1;
  height: 22px;
  padding: 0 9px;
  border-radius: 999px;
  background: var(--surface-muted);
  color: var(--text-muted);
}
.pill .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--text-muted);
  flex-shrink: 0;
}
.pill.live {
  background: color-mix(in srgb, var(--success) 14%, var(--surface));
  color: color-mix(in srgb, var(--success) 78%, var(--text));
}
.pill.live .dot {
  background: var(--success);
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.18);
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

.endpoint {
  margin: 0 20px 14px;
  padding: 11px 12px;
  border-radius: 12px;
  background:
    radial-gradient(120% 80% at 0% 0%, rgba(76, 116, 223, 0.08), transparent 55%),
    var(--surface-sunken);
  border: 1px solid var(--border-soft);
}
.endpoint-label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--text-muted);
  margin-bottom: 6px;
}
.endpoint-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.endpoint-url {
  flex: 1;
  min-width: 0;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12.5px;
  color: var(--text-regular);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.endpoint-url.muted {
  color: var(--text-muted);
}
.ghost-btn {
  appearance: none;
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--text-regular);
  font: inherit;
  font-size: 12px;
  padding: 5px 10px;
  border-radius: 8px;
  cursor: pointer;
  white-space: nowrap;
}
.ghost-btn:hover:not(:disabled) {
  border-color: color-mix(in srgb, var(--brand) 35%, var(--border));
  background: var(--surface-hover);
}
.ghost-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.params {
  padding: 2px 20px 6px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  flex: 1;
}
.field-label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-regular);
  margin-bottom: 7px;
}
.field-label .req {
  color: var(--danger);
  margin-left: 1px;
}
.control {
  display: flex;
  gap: 8px;
  align-items: center;
}
.control > :deep(.app-input),
.control > :not(.mini) {
  flex: 1;
  min-width: 0;
}
.control .mini {
  flex: 0 0 132px;
  max-width: 140px;
}
.control .test-btn {
  flex: 0 0 auto;
  min-width: 52px;
  padding-left: 12px;
  padding-right: 12px;
}
.path-maps {
  width: 100%;
  resize: vertical;
  min-height: 72px;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: 12.5px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  line-height: 1.5;
  box-sizing: border-box;
}
.path-maps:focus {
  outline: none;
  border-color: #93b4ff;
  box-shadow: 0 0 0 3px rgba(76, 116, 223, 0.15);
}
.path-maps::placeholder {
  color: var(--text-muted);
}
.card-error {
  margin: 0;
  font-size: 12px;
  color: var(--danger);
}

.card-foot {
  margin-top: auto;
  padding: 14px 20px 18px;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  border-top: 1px solid var(--border-soft);
  background: linear-gradient(
    180deg,
    transparent,
    color-mix(in srgb, var(--surface-muted) 90%, transparent)
  );
}

@media (max-width: 980px) {
  .proxy-grid {
    grid-template-columns: 1fr;
  }
  .page-head {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
