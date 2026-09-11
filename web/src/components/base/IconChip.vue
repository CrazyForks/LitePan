<script setup lang="ts">
import IconOrText from "@/components/icons/IconOrText.vue";

// 统计卡和概况卡共用的图标块。
withDefaults(defineProps<{ icon: string; tone?: "blue" | "green" | "amber" | "purple" | "red" }>(), {
  tone: "blue",
});
</script>

<template>
  <span class="icon-chip" :class="`icon-chip--${tone}`">
    <IconOrText :value="icon" :size="20" />
  </span>
</template>

<style scoped>
.icon-chip {
  width: 44px;
  height: 44px;
  display: grid;
  place-items: center;
  border-radius: var(--radius-card);
  overflow: hidden;
  flex-shrink: 0;
  color: #fff;
  font-size: 18px;
  background: var(--brand-gradient);
}
.icon-chip--blue {
  background: var(--brand-gradient);
}
.icon-chip--green {
  background: linear-gradient(135deg, #34d399, #059669);
}
.icon-chip--amber {
  background: linear-gradient(135deg, #f59e0b, #f97316);
}
.icon-chip--purple {
  background: linear-gradient(135deg, #7c3aed, #4f46e5);
}
.icon-chip--red {
  background: linear-gradient(135deg, #ef4444, #f97316);
}
</style>
