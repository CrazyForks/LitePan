<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { getApiErrorMessage } from "@/api/client";
import {
  classificationApi,
  type ClassificationConfig,
  type ClassificationRule,
  type ClassificationTemplate,
  type ClassificationTemplateKind,
  type ClassificationTMDBDetail,
} from "@/api/cloudTools";
import { toast } from "@/composables/useToast";
import AppButton from "@/components/base/AppButton.vue";
import AppModal from "@/components/base/AppModal.vue";
import CloudToolCard from "@/components/admin/CloudToolCard.vue";

const props = withDefaults(defineProps<{ searchQuery?: string }>(), { searchQuery: "" });

const templateMeta: Array<{ kind: ClassificationTemplateKind; name: string; desc: string }> = [
  { kind: "media", name: "内置模板一", desc: "仅按 电影 / 剧集分类" },
  { kind: "region", name: "内置模板二", desc: "按原产国家二级分类" },
  { kind: "genre", name: "内置模板三", desc: "按影片类型二级分类" },
  { kind: "custom", name: "自定义模板", desc: "自由组合目录层级与匹配条件" },
];

const emptyConfig = (): ClassificationConfig => ({
  version: 1,
  enabled: false,
  selected_template: "media",
  templates: [],
});

const config = ref<ClassificationConfig>(emptyConfig());
const draft = ref<ClassificationConfig>(emptyConfig());
const open = ref(false);
const helpOpen = ref(false);
const saving = ref(false);
const detailTMDBID = ref("");
const detailMediaType = ref<"movie" | "tv">("movie");
const detailLoading = ref(false);
const detailResult = ref<ClassificationTMDBDetail | null>(null);

function cloneConfig(value: ClassificationConfig): ClassificationConfig {
  return JSON.parse(JSON.stringify(value)) as ClassificationConfig;
}

function matches(title: string) {
  const query = props.searchQuery.trim().toLowerCase();
  return !query || title.toLowerCase().includes(query);
}

function templateLabel(kind: ClassificationTemplateKind) {
  return templateMeta.find((item) => item.kind === kind)?.name ?? kind;
}

const selectedTemplate = computed(() =>
  draft.value.templates.find((item) => item.kind === draft.value.selected_template),
);

async function load() {
  try {
    config.value = await classificationApi.getConfig();
  } catch (error) {
    toast.error(getApiErrorMessage(error, "加载分类整理配置失败"));
  }
}

onMounted(load);

function openSettings() {
  draft.value = cloneConfig(config.value);
  helpOpen.value = false;
  detailResult.value = null;
  open.value = true;
}

function selectTemplate(kind: ClassificationTemplateKind) {
  draft.value.selected_template = kind;
}

function addCustomRootRule(template: ClassificationTemplate) {
  template.rules.push({ name: "新分类", condition: "type=tv", fallback_to_self: false, children: [] });
}

function addChildRule(template: ClassificationTemplate, parent: ClassificationRule) {
  const condition = template.kind === "region"
    ? "origin_country=CN"
    : template.kind === "genre"
      ? "genres=剧情"
      : template.kind === "custom"
        ? "genres=剧情"
        : "type=movie";
  (parent.children ??= []).push({ name: "新分类", condition, fallback_to_self: false, children: [] });
}

function removeRule(rules: ClassificationRule[], index: number) {
  rules.splice(index, 1);
}

function objectStringValues(value: unknown, key: string) {
  if (!Array.isArray(value)) return [];
  return value
    .map((item) => {
      if (typeof item === "string") return item;
      if (item && typeof item === "object") {
        const raw = (item as Record<string, unknown>)[key];
        return typeof raw === "string" ? raw : "";
      }
      return "";
    })
    .filter(Boolean);
}

const detailTitle = computed(() => {
  const detail = detailResult.value;
  if (!detail) return "";
  return String(detail.title ?? detail.name ?? `TMDB ${detail.id ?? detailTMDBID.value}`);
});

const detailConditions = computed(() => {
  const detail = detailResult.value;
  if (!detail) return [];
  const conditions: Array<{ label: string; value: string }> = [
    { label: "媒体类型", value: `type=${detail.media_type ?? detailMediaType.value}` },
  ];
  const originCountries = objectStringValues(detail.origin_country, "iso_3166_1");
  const genres = objectStringValues(detail.genres, "name");
  if (originCountries.length) conditions.push({ label: "原产地区", value: `origin_country=${originCountries.join(";")}` });
  if (genres.length) conditions.push({ label: "影片类型", value: `genres=${genres.join(";")}` });
  return conditions;
});

const detailJSON = computed(() => JSON.stringify(detailResult.value, null, 2));

async function lookupTMDBDetail() {
  const tmdbID = detailTMDBID.value.trim();
  if (!/^\d{1,10}$/.test(tmdbID) || Number(tmdbID) <= 0) {
    toast.error("请输入 1～10 位有效 TMDB ID");
    return;
  }
  detailLoading.value = true;
  detailResult.value = null;
  try {
    detailResult.value = await classificationApi.lookupTMDBDetail({
      tmdb_id: tmdbID,
      media_type: detailMediaType.value,
    });
  } catch (error) {
    toast.error(getApiErrorMessage(error, "查询 TMDB 详情失败"));
  } finally {
    detailLoading.value = false;
  }
}

async function copyCondition(value: string) {
  try {
    await navigator.clipboard.writeText(value);
    toast.success("匹配条件已复制");
  } catch {
    toast.error("复制失败，请手动选择文本");
  }
}

async function persist(next: ClassificationConfig, successMessage: string) {
  saving.value = true;
  try {
    config.value = await classificationApi.saveConfig(next);
    toast.success(successMessage);
    return true;
  } catch (error) {
    toast.error(getApiErrorMessage(error, "保存分类整理配置失败"));
    return false;
  } finally {
    saving.value = false;
  }
}

async function toggleEnabled() {
  await persist(
    { ...cloneConfig(config.value), enabled: !config.value.enabled },
    config.value.enabled ? "分类整理已停用" : "分类整理已启用",
  );
}

async function saveSettings() {
  if (await persist(cloneConfig(draft.value), "分类模板已保存")) open.value = false;
}
</script>

<template>
  <div v-show="matches('目录整理分类')">
    <CloudToolCard
      :enabled="config.enabled"
      name="目录整理分类"
      driver="移动整理 · 按模板生成分类目录"
      logo-src="/logos/classification.png"
      logo-alt="目录整理分类"
      :stat-value="templateLabel(config.selected_template)"
      :compact-stat="true"
    >
      <template #toggle>
        <button
          class="check-toggle"
          type="button"
          :class="{ on: config.enabled }"
          :aria-label="config.enabled ? '停用分类整理' : '启用分类整理'"
          :disabled="saving"
          title="启用 / 停用"
          @click="toggleEnabled"
        >
          <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M3.5 8.5 6.5 11.5 12.5 4.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" /></svg>
        </button>
      </template>
      移动整理时按所选模板放入分类目录；无法归类时放入目标根目录，本地重命名模式不受影响。
      <template #actions>
        <AppButton size="sm" variant="secondary" :disabled="saving" @click="openSettings">分类设置</AppButton>
      </template>
    </CloudToolCard>

    <AppModal :open="open" title="请选择分类模板" size="lg" @close="open = false">
      <div class="classification-template-tabs">
        <button
          v-for="item in templateMeta"
          :key="item.kind"
          type="button"
          class="classification-template-tab"
          :class="{ active: draft.selected_template === item.kind }"
          @click="selectTemplate(item.kind)"
        >
          <strong>{{ item.name }}</strong><span>{{ item.desc }}</span>
        </button>
      </div>

      <div v-if="selectedTemplate" class="classification-editor">
        <div class="classification-table-head" aria-hidden="true">
          <span>分类目录</span>
          <span>匹配条件</span>
          <span></span>
        </div>

        <section
          v-for="(rule, index) in selectedTemplate.rules"
          :key="`root-${index}`"
          class="classification-group"
        >
          <div class="classification-row classification-row--root">
            <input v-model.trim="rule.name" maxlength="120" aria-label="一级分类目录" placeholder="目录名" />
            <input
              v-model.trim="rule.condition"
              maxlength="500"
              aria-label="一级匹配条件"
              :readonly="selectedTemplate.kind !== 'custom'"
              :title="selectedTemplate.kind === 'custom' ? '' : '内置模板的一级匹配条件固定，仅可修改目录名称'"
              :placeholder="selectedTemplate.kind === 'custom' ? 'type=tv，genres=真人秀' : 'type=movie'"
            />
            <div class="classification-actions">
              <button v-if="selectedTemplate.kind === 'custom'" type="button" class="danger" :disabled="selectedTemplate.rules.length === 1" title="删除" @click="removeRule(selectedTemplate.rules, index)">×</button>
            </div>
          </div>

          <div
            v-for="(child, childIndex) in (rule.children ?? [])"
            :key="`child-${index}-${childIndex}`"
            class="classification-row classification-row--child"
          >
            <label>
              <span aria-hidden="true">└</span>
              <input v-model.trim="child.name" maxlength="120" aria-label="二级分类目录" placeholder="二级目录名" />
            </label>
            <input
              v-model.trim="child.condition"
              maxlength="500"
              aria-label="二级匹配条件"
              :placeholder="selectedTemplate.kind === 'region' ? 'origin_country=CN;HK' : selectedTemplate.kind === 'custom' ? 'origin_country=JP，genres=动画' : 'genres=犯罪;悬疑'"
            />
            <div class="classification-actions">
              <button type="button" class="danger" title="删除" @click="removeRule(rule.children ?? [], childIndex)">×</button>
            </div>
          </div>

          <label v-if="selectedTemplate.kind === 'custom' && (rule.children?.length ?? 0) > 0" class="classification-fallback">
            <input v-model="rule.fallback_to_self" type="checkbox" />
            <span>子分类均未命中时，放入「{{ rule.name || "当前" }}」目录</span>
          </label>
          <button v-if="selectedTemplate.kind !== 'media'" type="button" class="classification-add classification-add--child" @click="addChildRule(selectedTemplate, rule)">+ 二级分类</button>
        </section>

        <button v-if="selectedTemplate.kind === 'custom'" type="button" class="classification-add" @click="addCustomRootRule(selectedTemplate)">+ 一级分类</button>
      </div>
      <template #footer>
        <AppButton class="classification-help-button" variant="secondary" @click="helpOpen = true">查看帮助</AppButton>
        <AppButton variant="secondary" :disabled="saving" @click="open = false">取消</AppButton>
        <AppButton variant="primary" :disabled="saving" @click="saveSettings">{{ saving ? "保存中…" : "保存" }}</AppButton>
      </template>
    </AppModal>

    <AppModal :open="helpOpen" title="分类帮助" size="lg" nested @close="helpOpen = false">
      <div class="classification-help">
        <p>移动整理时，影片会按模板放进分类目录；没匹配上就放在任务目标根目录。本地重命名不受影响。</p>
      </div>

      <section class="classification-lookup">
        <div class="classification-lookup__intro">
          <strong>TMDB 字段查询</strong>
          <span>输入 ID 查看真实字段，并复制为匹配条件</span>
        </div>
        <form class="classification-lookup__form" @submit.prevent="lookupTMDBDetail">
          <select v-model="detailMediaType" aria-label="TMDB 媒体类型">
            <option value="movie">电影</option>
            <option value="tv">电视剧</option>
          </select>
          <input v-model.trim="detailTMDBID" inputmode="numeric" maxlength="10" aria-label="TMDB ID" placeholder="TMDB ID，如 281495" />
          <AppButton type="submit" variant="secondary" :disabled="detailLoading">
            {{ detailLoading ? "查询中…" : "查询" }}
          </AppButton>
        </form>

        <div v-if="detailResult" class="classification-detail">
          <div class="classification-detail__title">
            <strong>{{ detailTitle }}</strong>
            <span>TMDB {{ detailResult.id }} · {{ detailResult.media_type === "tv" ? "电视剧" : "电影" }}</span>
          </div>
          <div class="classification-detail__conditions">
            <button
              v-for="item in detailConditions"
              :key="item.value"
              type="button"
              :title="`复制 ${item.value}`"
              @click="copyCondition(item.value)"
            >
              <span>{{ item.label }}</span><code>{{ item.value }}</code><b>复制</b>
            </button>
          </div>
          <details class="classification-detail__raw">
            <summary>查看全部 TMDB 详情字段</summary>
            <pre>{{ detailJSON }}</pre>
          </details>
        </div>
      </section>

      <div class="classification-help classification-help--after-query">
        <p><strong>模板怎么选：</strong>模板一分电影和剧集，模板二按国家分，模板三按影片类型分；想自己搭配就用自定义模板。</p>
        <p><strong>条件怎么填：</strong>同一字段有多个可选值时，用分号隔开，例如 <code>origin_country=CN;US</code>，表示原产国家是 CN 或 US 都能命中。不同字段之间用中文或英文逗号隔开，例如 <code>type=tv，genres=动画</code>，表示必须同时是剧集且类型包含动画；这种多字段组合仅用于自定义模板。常用字段有 <code>type</code>、<code>origin_country</code>、<code>genres</code>，不确定实际返回值时可以使用上面的 TMDB 查询。</p>
      </div>

      <template #footer>
        <AppButton variant="primary" @click="helpOpen = false">关闭</AppButton>
      </template>
    </AppModal>
  </div>
</template>

<style scoped>
.check-toggle { width: 28px; height: 28px; border-radius: 50%; border: 0; padding: 0; flex-shrink: 0; display: inline-flex; align-items: center; justify-content: center; cursor: pointer; background: var(--border); color: var(--text-muted); transition: background .18s ease, color .18s ease, box-shadow .18s ease; }
.check-toggle svg { width: 14px; height: 14px; }
.check-toggle:hover { background: var(--surface-hover); }
.check-toggle.on { background: var(--success); color: #fff; box-shadow: 0 0 0 4px rgba(16, 185, 129, .16); }
.check-toggle:disabled { opacity: .5; cursor: not-allowed; }
.classification-template-tabs { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; }
.classification-template-tab { min-width: 0; display: grid; gap: 4px; text-align: left; border: 1px solid var(--border); border-radius: var(--radius-md); padding: 10px; background: var(--surface); color: var(--text); cursor: pointer; }
.classification-template-tab span { color: var(--text-muted); font-size: 12px; }
.classification-template-tab.active { border-color: var(--brand); background: color-mix(in srgb, var(--brand) 7%, var(--surface)); box-shadow: 0 0 0 2px color-mix(in srgb, var(--brand) 12%, transparent); }
.classification-lookup { margin-top: 14px; padding: 12px; border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--surface-sunken); }
.classification-lookup__intro { display: flex; align-items: baseline; gap: 8px; }
.classification-lookup__intro span { color: var(--text-muted); font-size: 12px; }
.classification-lookup__form { display: grid; grid-template-columns: 110px minmax(180px, 1fr) auto; gap: 8px; margin-top: 9px; }
.classification-lookup__form select, .classification-lookup__form input { box-sizing: border-box; min-width: 0; width: 100%; border: 1px solid var(--border); border-radius: var(--radius-sm); padding: 8px 9px; background: var(--surface); color: var(--text); }
.classification-detail { margin-top: 11px; padding-top: 11px; border-top: 1px solid var(--border); }
.classification-detail__title { display: flex; align-items: baseline; gap: 8px; }
.classification-detail__title span { color: var(--text-muted); font-size: 12px; }
.classification-detail__conditions { display: grid; gap: 6px; margin-top: 9px; }
.classification-detail__conditions button { display: grid; grid-template-columns: 72px minmax(0, 1fr) auto; gap: 8px; align-items: center; width: 100%; padding: 7px 9px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); color: var(--text); text-align: left; cursor: pointer; }
.classification-detail__conditions button:hover { border-color: var(--brand); }
.classification-detail__conditions span { color: var(--text-muted); font-size: 12px; }
.classification-detail__conditions code { overflow-wrap: anywhere; white-space: normal; }
.classification-detail__conditions b { color: var(--brand); font-size: 12px; }
.classification-detail__raw { margin-top: 9px; color: var(--text-regular); font-size: 12px; }
.classification-detail__raw summary { cursor: pointer; color: var(--brand); }
.classification-detail__raw pre { box-sizing: border-box; max-height: 280px; overflow: auto; margin: 8px 0 0; padding: 10px; border-radius: var(--radius-sm); background: var(--surface); color: var(--text); font-size: 11px; line-height: 1.55; white-space: pre-wrap; overflow-wrap: anywhere; }
.classification-editor { margin-top: 16px; overflow: hidden; border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--surface-sunken); }
.classification-table-head, .classification-row { display: grid; grid-template-columns: minmax(130px, .8fr) minmax(250px, 1.5fr) 32px; gap: 10px; align-items: center; }
.classification-table-head { padding: 9px 12px; border-bottom: 1px solid var(--border); color: var(--text-muted); font-size: 12px; font-weight: 600; }
.classification-group { padding: 10px 12px; border-bottom: 1px solid var(--border); }
.classification-row + .classification-row { margin-top: 7px; }
.classification-row input { box-sizing: border-box; width: 100%; min-width: 0; border: 1px solid var(--border); border-radius: var(--radius-sm); padding: 8px 9px; background: var(--surface); color: var(--text); }
.classification-row input:read-only { background: var(--surface-sunken); color: var(--text-muted); cursor: default; }
.classification-row--root > input:first-child { font-weight: 600; }
.classification-row--child label { min-width: 0; display: grid; grid-template-columns: 18px minmax(0, 1fr); align-items: center; color: var(--text-muted); }
.classification-actions { display: flex; justify-content: flex-end; gap: 3px; }
.classification-actions button, .classification-add { border: 1px solid var(--border); border-radius: 7px; background: var(--surface); color: var(--text-regular); cursor: pointer; }
.classification-actions button { width: 27px; height: 32px; padding: 0; }
.classification-actions button:disabled { opacity: .35; cursor: default; }
.classification-actions button.danger { color: var(--danger); }
.classification-fallback { display: flex; align-items: center; gap: 7px; margin: 8px 0 0 28px; color: var(--text-muted); font-size: 12px; cursor: pointer; }
.classification-fallback input { margin: 0; accent-color: var(--brand); }
.classification-add { margin: 10px 12px; padding: 7px 10px; font-size: 12px; }
.classification-add--child { margin: 8px 0 0 28px; padding-block: 5px; }
.classification-help-button { margin-right: auto; }
.classification-help p { margin: 0; color: var(--text-regular); font-size: 12px; line-height: 1.65; }
.classification-help p + p { margin-top: 5px; }
.classification-help code { padding: 1px 4px; border-radius: 4px; background: var(--surface-sunken); color: var(--text); }
.classification-help--after-query { margin-top: 12px; }
@media (max-width: 760px) {
  .classification-template-tabs { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .classification-lookup__intro, .classification-detail__title { align-items: flex-start; flex-direction: column; gap: 2px; }
  .classification-lookup__form { grid-template-columns: 1fr; }
  .classification-detail__conditions button { grid-template-columns: 1fr auto; }
  .classification-detail__conditions span { grid-column: 1 / -1; }
  .classification-table-head { display: none; }
  .classification-row { grid-template-columns: 1fr; gap: 7px; }
  .classification-actions { justify-content: flex-end; }
}
@media (max-width: 480px) {
  .classification-template-tabs { grid-template-columns: 1fr; }
}
</style>
