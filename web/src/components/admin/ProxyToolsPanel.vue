<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { getApiErrorMessage } from "@/api/client";
import {
  fetchEmbyConfigs,
  refreshEmbyLibrary,
  saveEmbyConfigs,
  testEmbyConfig,
  type EmbyConfig,
  type EmbyConfigUpdate,
} from "@/api/emby";
import { fetchFnosConfig, saveFnosConfig, testFnosConfig, type FnosConfig } from "@/api/fnos";
import { confirm } from "@/composables/useConfirm";
import { copyTextToClipboard, toast } from "@/composables/useToast";
import AppButton from "@/components/base/AppButton.vue";
import AppModal from "@/components/base/AppModal.vue";
import SettingsHelpTooltip from "@/components/admin/SettingsHelpTooltip.vue";
import embyLogo from "@/assets/proxy/embylogo.png";
import fnosLogo from "@/assets/proxy/fnmovielogo.png";

const props = withDefaults(defineProps<{ searchQuery?: string }>(), { searchQuery: "" });

function matches(title: string) {
  const q = props.searchQuery.trim().toLowerCase();
  return !q || title.toLowerCase().includes(q);
}

const embyConfigs = ref<EmbyConfig[]>([]);
const embyEnabled = ref(false);
const embyOpen = ref(false);
const embyEditorOpen = ref(false);
const embySaving = ref(false);
const embyTesting = ref(false);
const embyRefreshingID = ref("");
const editingID = ref("");
const embyDraft = reactive<EmbyConfigUpdate>({
  name: "",
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
const fnosOpen = ref(false);
const fnosSaving = ref(false);
const fnosTesting = ref(false);

const embyRunning = computed(() => embyConfigs.value.filter((item) => item.running).length);

onMounted(async () => {
  try {
    const [emby, fnos] = await Promise.all([fetchEmbyConfigs(), fetchFnosConfig()]);
    embyEnabled.value = Boolean(emby.enabled);
    embyConfigs.value = emby.items || [];
    applyFnos(fnos);
  } catch (error) {
    toast.error(getApiErrorMessage(error, "加载反代配置失败"));
  }
});

function applyFnos(config: FnosConfig) {
  fnosForm.enabled = Boolean(config.enabled);
  fnosForm.fnos_url = config.fnos_url || "";
  fnosForm.proxy_port = config.proxy_port || "";
  fnosForm.strm_path_maps = config.strm_path_maps || "";
  fnosForm.strm_dir = config.strm_dir || "/app/strm";
  fnosForm.proxy_url = config.proxy_url || "";
  fnosForm.running = Boolean(config.running);
  fnosForm.last_error = config.last_error || "";
}

function resolveProxyURL(proxyURL: string, port: string) {
  const value = port.trim();
  if (!value) return proxyURL;
  try {
    const url = new URL(proxyURL || `http://127.0.0.1:${value}`);
    if (["127.0.0.1", "localhost"].includes(url.hostname) && !["127.0.0.1", "localhost"].includes(window.location.hostname)) {
      return `${window.location.protocol}//${window.location.hostname}:${value}`;
    }
  } catch {}
  return proxyURL;
}

async function copyEndpoint(config: { proxy_url: string; proxy_port: string; running: boolean }) {
  const endpoint = resolveProxyURL(config.proxy_url, config.proxy_port);
  if (!config.running || !endpoint) {
    toast.error("反代尚未运行");
    return;
  }
  await copyTextToClipboard(endpoint, { successMessage: "已复制反代地址", errorMessage: "复制失败" });
}

function openNewEmby() {
  editingID.value = "";
  Object.assign(embyDraft, {
    id: "",
    name: embyConfigs.value.length ? `Emby ${embyConfigs.value.length + 1}` : "Emby",
    emby_url: "",
    api_key: "",
    proxy_port: "",
  });
  embyEditorOpen.value = true;
}

function editEmby(config: EmbyConfig) {
  editingID.value = config.id;
  Object.assign(embyDraft, {
    id: config.id,
    name: config.name,
    emby_url: config.emby_url,
    api_key: config.api_key,
    proxy_port: config.proxy_port,
  });
  embyEditorOpen.value = true;
}

function updatesFromConfigs(configs: EmbyConfig[]): EmbyConfigUpdate[] {
  return configs.map((item) => ({
    id: item.id,
    name: item.name,
    emby_url: item.emby_url,
    api_key: item.api_key,
    proxy_port: String(item.proxy_port || ""),
  }));
}

async function persistEmby(items: EmbyConfigUpdate[], message: string, enabled = embyEnabled.value) {
  embySaving.value = true;
  try {
    const state = await saveEmbyConfigs(enabled, items);
    embyEnabled.value = state.enabled;
    embyConfigs.value = state.items || [];
    toast.success(message);
    return true;
  } catch (error) {
    toast.error(getApiErrorMessage(error, "保存 Emby 配置失败"));
    return false;
  } finally {
    embySaving.value = false;
  }
}

async function saveEmbyEditor() {
  if (!embyDraft.name.trim() || !embyDraft.emby_url.trim() || !embyDraft.api_key.trim() || !String(embyDraft.proxy_port || "").trim()) {
    toast.error("请填写配置名称、Emby 地址、API Key 和反代端口");
    return;
  }
  const items = updatesFromConfigs(embyConfigs.value);
  const next = { ...embyDraft, id: editingID.value || undefined };
  const index = items.findIndex((item) => item.id === editingID.value);
  if (index >= 0) items[index] = next;
  else items.push(next);
  if (await persistEmby(items, editingID.value ? "Emby 配置已保存" : "Emby 配置已添加")) embyEditorOpen.value = false;
}

async function testEmby() {
  embyTesting.value = true;
  try {
    await testEmbyConfig({ ...embyDraft, id: editingID.value || undefined });
    toast.success("Emby 连接成功");
  } catch (error) {
    toast.error(getApiErrorMessage(error, "Emby 连接失败"));
  } finally {
    embyTesting.value = false;
  }
}

async function setEmbyEnabled(enabled: boolean) {
  if (enabled && embyConfigs.value.length === 0) {
    toast.error("请先添加 Emby 配置");
    embyOpen.value = true;
    return;
  }
  await persistEmby(
    updatesFromConfigs(embyConfigs.value),
    enabled ? "Emby 反代已启用" : "Emby 反代已停用",
    enabled,
  );
}

async function deleteEmby(config: EmbyConfig) {
  const ok = await confirm({
    title: "删除 Emby 配置？",
    message: `将删除「${config.name}」。引用它的自动联动需要重新选择 Emby。`,
    confirmText: "确认删除",
    cancelText: "取消",
    danger: true,
  }).catch(() => false);
  if (!ok) return;
  await persistEmby(updatesFromConfigs(embyConfigs.value.filter((item) => item.id !== config.id)), "Emby 配置已删除");
}

async function refreshEmby(config: EmbyConfig) {
  embyRefreshingID.value = config.id;
  try {
    await refreshEmbyLibrary({ config_id: config.id, mode: "global" });
    toast.success(`已通知「${config.name}」刷库`);
  } catch (error) {
    toast.error(getApiErrorMessage(error, "刷库失败"));
  } finally {
    embyRefreshingID.value = "";
  }
}

async function saveFnos(enabled = fnosForm.enabled) {
  fnosSaving.value = true;
  try {
    applyFnos(await saveFnosConfig({ enabled, fnos_url: fnosForm.fnos_url, proxy_port: String(fnosForm.proxy_port || ""), strm_path_maps: fnosForm.strm_path_maps }));
    toast.success("飞牛影视反代配置已保存");
    return true;
  } catch (error) {
    toast.error(getApiErrorMessage(error, "保存飞牛影视配置失败"));
    return false;
  } finally {
    fnosSaving.value = false;
  }
}

async function testFnos() {
  fnosTesting.value = true;
  try {
    await testFnosConfig({ enabled: fnosForm.enabled, fnos_url: fnosForm.fnos_url, proxy_port: String(fnosForm.proxy_port || ""), strm_path_maps: fnosForm.strm_path_maps });
    toast.success("飞牛影视连接成功");
  } catch (error) {
    toast.error(getApiErrorMessage(error, "飞牛影视连接失败"));
  } finally {
    fnosTesting.value = false;
  }
}
</script>

<template>
  <div class="proxy-enhancement-cards">
    <article v-show="matches('Emby 反代')" class="tool-card" :class="embyEnabled ? 'is-enabled' : 'is-disabled'">
      <span class="tool-card__bar" :class="embyEnabled ? 'is-enabled' : 'is-disabled'" />
      <div class="tool-card__head">
        <img class="tool-card__logo" :src="embyLogo" alt="Emby" />
        <div class="tool-card__meta"><h3 class="tool-card__name">Emby 反代 <span class="tool-card__tag">Emby专用</span><SettingsHelpTooltip title="Emby 反代说明"><p>在播放器和 Emby 之间加一层：播放 STRM 时，把真实的网盘播放地址直接交给播放器。</p></SettingsHelpTooltip></h3><p class="tool-card__driver">STRM 播放直连 · 支持多个 Emby 服务</p></div>
        <button class="check-toggle" type="button" :class="{ on: embyEnabled }" :disabled="embySaving" title="启用 / 停用" @click="setEmbyEnabled(!embyEnabled)"><svg viewBox="0 0 16 16"><path d="M3.5 8.5 6.5 11.5 12.5 4.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" /></svg></button>
      </div>
      <p class="tool-card__desc">解决 Emby 网页端或客户端播放 STRM 时，只能由 Emby 服务端代为拉流、走不了 302 直连的问题：反代访问后，LitePan 把 302 的 CDN 地址直接交给播放器。</p>
      <div class="tool-card__row"><div class="tool-card__stat"><span class="tool-card__num">{{ embyConfigs.length }}</span><span class="tool-card__label">个配置 · {{ embyRunning }} 个运行</span></div><AppButton variant="secondary" @click="embyOpen = true">配置反代参数</AppButton></div>
    </article>

    <article v-show="matches('飞牛影视反代')" class="tool-card" :class="fnosForm.enabled ? 'is-enabled' : 'is-disabled'">
      <span class="tool-card__bar" :class="fnosForm.enabled ? 'is-enabled' : 'is-disabled'" />
      <div class="tool-card__head">
        <img class="tool-card__logo" :src="fnosLogo" alt="飞牛影视" />
        <div class="tool-card__meta"><h3 class="tool-card__name">飞牛影视反代 <span class="tool-card__tag">第三方播放器</span><SettingsHelpTooltip title="飞牛影视反代说明"><p>在播放器和飞牛影视之间加一层：播放 STRM 时，把真实的网盘播放地址直接交给播放器。</p><p>爆米花 / VidHub / SenPlayer 等连接配置里的「反代入口」，就能正常播放 STRM 影片。</p></SettingsHelpTooltip></h3><p class="tool-card__driver">STRM 播放直连 · 飞牛路径转换</p></div>
        <button class="check-toggle" type="button" :class="{ on: fnosForm.enabled }" :disabled="fnosSaving" title="启用 / 停用" @click="saveFnos(!fnosForm.enabled)"><svg viewBox="0 0 16 16"><path d="M3.5 8.5 6.5 11.5 12.5 4.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" /></svg></button>
      </div>
      <p class="tool-card__desc">解决第三方播放器（如 VidHub、SenPlayer、爆米花）把影视来源填成飞牛影视后，无法播放 STRM 的问题：反代访问后，将飞牛保存的 STRM 路径还原为 LitePan 地址。</p>
      <div class="tool-card__row"><div class="tool-card__stat"><span class="tool-card__num">{{ fnosForm.running ? '运行中' : fnosForm.enabled ? '待监听' : '未启用' }}</span></div><AppButton variant="secondary" @click="fnosOpen = true">配置反代参数</AppButton></div>
    </article>

    <AppModal :open="embyOpen" title="Emby 反代配置" size="lg" @close="embyOpen = false">
      <div v-if="embyConfigs.length" class="config-list">
        <div v-for="config in embyConfigs" :key="config.id" class="config-item">
          <div class="config-item__main"><strong>{{ config.name }}</strong><span :class="config.running ? 'status-on' : 'status-off'">{{ config.running ? `运行中 · :${config.proxy_port}` : embyEnabled ? '未监听' : '未启用' }}</span><small v-if="config.last_error" class="field-error">{{ config.last_error }}</small></div>
          <div class="config-item__actions">
            <AppButton variant="secondary" :disabled="embyRefreshingID === config.id" @click="refreshEmby(config)">{{ embyRefreshingID === config.id ? '刷库中…' : '手动刷库' }}</AppButton>
            <AppButton variant="secondary" @click="editEmby(config)">编辑</AppButton><AppButton variant="danger" @click="deleteEmby(config)">删除</AppButton>
          </div>
        </div>
      </div>
      <div v-else class="config-empty">还没有 Emby 配置</div>
      <template #footer><div class="modal-footer-center"><AppButton variant="primary" @click="openNewEmby">添加配置</AppButton></div></template>
    </AppModal>

    <AppModal :open="embyEditorOpen" :title="editingID ? '编辑 Emby 配置' : '添加 Emby 配置'" size="md" nested @close="embyEditorOpen = false">
      <div class="form-grid">
        <label><span>配置名称</span><input v-model.trim="embyDraft.name" placeholder="例如：家庭 Emby" /></label>
        <label><span>Emby 地址<SettingsHelpTooltip title="Emby 地址说明"><p>你的 Emby 服务器地址，例如 http://192.168.1.10:8096。</p><p>给 LitePan 连 Emby 用的，播放器里不要填这个。</p></SettingsHelpTooltip></span><input v-model.trim="embyDraft.emby_url" placeholder="http://192.168.1.10:8096" /></label>
        <label><span>API Key<SettingsHelpTooltip title="API Key 说明"><p>在 Emby 后台「API 密钥」里生成一个，粘贴到这里，用来连接 Emby 和刷库。</p></SettingsHelpTooltip></span><input v-model.trim="embyDraft.api_key" type="password" autocomplete="new-password" placeholder="Emby API Key" /></label>
        <label><span>反代端口<SettingsHelpTooltip title="反代端口说明"><p>反代用的端口，随便选一个没被占用的数字就行。</p><p>留空则不启动反代。</p></SettingsHelpTooltip></span><input v-model.trim="embyDraft.proxy_port" inputmode="numeric" placeholder="例如 18097" /></label>
      </div>
      <template #footer><AppButton class="footer-left" variant="secondary" :disabled="embyTesting" @click="testEmby">{{ embyTesting ? '测试中…' : '测试连接' }}</AppButton><AppButton variant="secondary" @click="embyEditorOpen = false">取消</AppButton><AppButton variant="primary" :disabled="embySaving" @click="saveEmbyEditor">{{ embySaving ? '保存中…' : '保存' }}</AppButton></template>
    </AppModal>

    <AppModal :open="fnosOpen" title="飞牛影视反代配置" size="md" @close="fnosOpen = false">
      <div class="form-grid"><label><span>飞牛影视地址<SettingsHelpTooltip title="飞牛影视地址说明"><p>你的飞牛影视地址，端口一般是 8005（不是 NAS 管理页的 5666）。</p><p>给 LitePan 连飞牛用的，播放器里不要填这个。</p></SettingsHelpTooltip></span><input v-model.trim="fnosForm.fnos_url" placeholder="http://192.168.1.50:8005" /></label><label><span>飞牛 STRM 目录<SettingsHelpTooltip title="飞牛 STRM 目录说明"><p>把 Docker 里映射到 <code>/app/strm</code> 的左边路径填到这里。</p><p>例：<code>/vol1/1000/Strm/LitePanGO:/app/strm</code> → 填 <code>/vol1/1000/Strm/LitePanGO</code>。</p><p>两边路径相同则可留空。</p></SettingsHelpTooltip></span><input v-model.trim="fnosForm.strm_path_maps" placeholder="/vol1/1000/Strm/LitePanGO" /></label><label><span>反代端口<SettingsHelpTooltip title="反代端口说明"><p>反代用的端口，随便选一个没被占用的数字就行，别和 Emby 反代用同一个。</p><p>留空则不启动反代。</p></SettingsHelpTooltip></span><input v-model.trim="fnosForm.proxy_port" inputmode="numeric" placeholder="例如 18997" /></label><small v-if="fnosForm.last_error" class="field-error">{{ fnosForm.last_error }}</small></div>
      <div class="endpoint"><div class="endpoint-label">反代入口<SettingsHelpTooltip title="反代入口说明"><p>在播放器里添加飞牛服务器时，填这个地址。</p><p>注意不是上面的「飞牛影视地址」，别填混了。</p></SettingsHelpTooltip></div><div class="endpoint-row"><span class="endpoint-url" :class="{ muted: !fnosForm.running }">{{ fnosForm.running ? resolveProxyURL(fnosForm.proxy_url, fnosForm.proxy_port) : '启动后生成入口' }}</span><button class="ghost-btn" type="button" :disabled="!fnosForm.running" @click="copyEndpoint(fnosForm)">复制</button></div></div>
      <template #footer><AppButton class="footer-left" variant="secondary" :disabled="fnosTesting" @click="testFnos">{{ fnosTesting ? '测试中…' : '测试连接' }}</AppButton><AppButton variant="secondary" @click="fnosOpen = false">取消</AppButton><AppButton variant="primary" :disabled="fnosSaving" @click="saveFnos().then(ok => { if (ok) fnosOpen = false; })">{{ fnosSaving ? '保存中…' : '保存' }}</AppButton></template>
    </AppModal>
  </div>
</template>

<style scoped>
.proxy-enhancement-cards {
  display:contents
}

.tool-card {
  position:relative;
  background:var(--surface);
  border:1px solid var(--border);
  border-radius:var(--radius-xl);
  padding:20px;
  overflow:hidden;
  transition:var(--transition)
}

.tool-card:hover {
  box-shadow:var(--shadow-card)
}

.tool-card.is-enabled {
  border-color:color-mix(in srgb,var(--success) 40%,var(--border))
}

.tool-card__bar {
  position:absolute;
  inset:0 0 0 auto;
  width:4px
}

.tool-card__bar.is-enabled {
  background:linear-gradient(180deg,var(--success),#059669)
}

.tool-card__bar.is-disabled {
  background:linear-gradient(180deg,#9ca3af,#6b7280)
}

.tool-card__head {
  display:flex;
  align-items:center;
  gap:14px
}

.tool-card__logo {
  width:48px;
  height:48px;
  border-radius:var(--radius-md);
  flex-shrink:0;
  object-fit:contain
}

.tool-card__meta {
  flex:1;
  min-width:0
}

.tool-card__name {
  margin:0;
  font-size:15px;
  font-weight:600;
  display:flex;
  align-items:center;
  gap:8px;
  flex-wrap:wrap
}

.tool-card__tag {
  font-size:11px;
  font-weight:500;
  padding:1px 8px;
  border-radius:var(--radius-pill);
  background:var(--info-soft);
  color:var(--info)
}

.tool-card__driver {
  margin:2px 0 0;
  font-size:12px;
  color:var(--text-muted)
}

.tool-card__desc {
  margin:14px 0 0;
  font-size:13px;
  color:var(--text-regular)
}

.tool-card__row {
  display:flex;
  align-items:center;
  justify-content:space-between;
  gap:12px;
  margin-top:16px;
  padding-top:14px;
  border-top:1px dashed var(--border)
}

.tool-card__stat {
  display:flex;
  align-items:baseline;
  gap:8px
}

.tool-card__num {
  font-size:16px;
  font-weight:700;
  color:var(--text)
}

.tool-card__label {
  font-size:13px;
  color:var(--text-muted)
}

.check-toggle {
  width:28px;
  height:28px;
  border-radius:50%;
  border:0;
  padding:0;
  flex-shrink:0;
  display:inline-flex;
  align-items:center;
  justify-content:center;
  cursor:pointer;
  background:var(--border);
  color:var(--text-muted)
}

.check-toggle svg {
  width:14px;
  height:14px
}

.check-toggle.on {
  background:var(--success);
  color:#fff;
  box-shadow:0 0 0 4px rgba(16,185,129,.16)
}

.check-toggle:disabled {
  opacity:.5;
  cursor:not-allowed
}

.config-list {
  display:grid;
  gap:10px
}

.config-item {
  display:flex;
  flex-wrap:wrap;
  align-items:center;
  gap:10px 16px;
  padding:13px 14px;
  border:1px solid var(--border-soft);
  border-radius:var(--radius-md);
  background:var(--surface-sunken)
}

.config-item__main {
  min-width:190px;
  flex:1;
  display:grid;
  grid-template-columns:auto 1fr;
  align-items:center;
  gap:4px 8px
}

.config-item__main small {
  grid-column:1/-1;
  color:var(--text-muted);
  overflow:hidden;
  text-overflow:ellipsis;
  white-space:nowrap
}

.config-item__actions {
  display:flex;
  align-items:center;
  justify-content:flex-end;
  gap:6px;
  flex-wrap:wrap
}

.status-on {
  color:var(--success);
  font-size:12px
}

.status-off {
  color:var(--text-muted);
  font-size:12px
}

.config-empty {
  padding:38px 0;
  text-align:center;
  color:var(--text-muted)
}

.form-grid {
  display:grid;
  gap:14px
}

.form-grid label:not(.check-line) {
  display:grid;
  gap:6px;
  font-size:13px;
  font-weight:600;
  color:var(--text-regular)
}

.form-grid label > span:first-child {
  display:inline-flex;
  align-items:center;
  gap:6px
}

.form-grid input:not([type=checkbox]) {
  width:100%;
  box-sizing:border-box;
  border:1px solid var(--border);
  border-radius:var(--radius-sm);
  padding:9px 11px;
  font-size:13px;
  background:var(--surface);
  color:var(--text)
}

.check-line {
  display:flex;
  align-items:center;
  gap:8px;
  color:var(--text-regular);
  font-size:13px
}

.field-error {
  color:var(--danger)!important
}

.footer-left {
  margin-right:auto
}

.modal-footer-center {
  display:flex;
  align-items:center;
  justify-content:center;
  width:100%
}

.endpoint {
  padding:11px 12px;
  border-radius:12px;
  background:radial-gradient(120% 80% at 0% 0%, rgba(76,116,223,.08), transparent 55%), var(--surface-sunken);
  border:1px solid var(--border-soft)
}

.form-grid + .endpoint {
  margin-top:14px
}

.endpoint-label {
  display:flex;
  align-items:center;
  gap:4px;
  font-size:11px;
  font-weight:600;
  letter-spacing:.06em;
  text-transform:uppercase;
  color:var(--text-muted);
  margin-bottom:6px
}

.endpoint-row {
  display:flex;
  align-items:center;
  gap:8px
}

.endpoint-url {
  flex:1;
  min-width:0;
  font-family:ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size:12.5px;
  color:var(--text-regular);
  white-space:nowrap;
  overflow:hidden;
  text-overflow:ellipsis
}

.endpoint-url.muted {
  color:var(--text-muted)
}

.ghost-btn {
  appearance:none;
  border:1px solid var(--border);
  background:var(--surface);
  color:var(--text-regular);
  font:inherit;
  font-size:12px;
  padding:5px 10px;
  border-radius:8px;
  cursor:pointer;
  white-space:nowrap
}

.ghost-btn:hover:not(:disabled) {
  border-color:color-mix(in srgb, var(--brand) 35%, var(--border));
  background:var(--surface-hover)
}

.ghost-btn:disabled {
  opacity:.45;
  cursor:not-allowed
}

@media(max-width:760px) {
  .config-item {
    align-items:stretch;
    flex-direction:column
  }

  .config-item__actions {
    justify-content:flex-start
  }

}
</style>
