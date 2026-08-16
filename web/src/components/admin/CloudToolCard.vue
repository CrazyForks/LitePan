<script setup lang="ts">
import { computed } from "vue";

type ToolCardTag = {
  label: string;
  variant?: "default" | "warn";
};

const props = withDefaults(defineProps<{
  enabled: boolean;
  name: string;
  driver: string;
  logoSrc?: string;
  logoAlt?: string;
  logoText?: string;
  tags?: ToolCardTag[];
  statValue?: string | number;
  statLabel?: string;
  compactStat?: boolean;
}>(), {
  logoSrc: "",
  logoAlt: "",
  logoText: "",
  tags: () => [],
  statValue: "",
  statLabel: "",
  compactStat: false,
});

const stateClass = computed(() => (props.enabled ? "is-enabled" : "is-disabled"));
</script>

<template>
  <article class="tool-card" :class="stateClass">
    <span class="tool-card__bar" :class="stateClass" />

    <div class="tool-card__head">
      <slot name="logo">
        <img v-if="logoSrc" class="tool-card__logo" :src="logoSrc" :alt="logoAlt || name" />
        <div v-else-if="logoText" class="tool-card__logo tool-card__logo--text" aria-hidden="true">{{ logoText }}</div>
      </slot>

      <div class="tool-card__meta">
        <h3 class="tool-card__name">
          {{ name }}
          <span
            v-for="tag in tags"
            :key="`${name}-${tag.label}`"
            class="tool-card__tag"
            :class="{ 'tool-card__tag--warn': tag.variant === 'warn' }"
          >
            {{ tag.label }}
          </span>
        </h3>
        <p class="tool-card__driver">{{ driver }}</p>
      </div>

      <slot name="toggle" />
    </div>

    <div class="tool-card__desc">
      <slot />
    </div>

    <div class="tool-card__row">
      <div class="tool-card__stat" :class="{ 'tool-card__stat--compact': compactStat }">
        <span class="tool-card__num">{{ statValue }}</span>
        <span v-if="statLabel" class="tool-card__label">{{ statLabel }}</span>
      </div>
      <div class="tool-card__actions">
        <slot name="actions" />
      </div>
    </div>
  </article>
</template>

<style scoped>
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

.tool-card__logo--text {
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

.tool-card__desc :deep(code) {
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

.tool-card__stat--compact {
  min-width: 0;
}

.tool-card__stat--compact .tool-card__num {
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
</style>
