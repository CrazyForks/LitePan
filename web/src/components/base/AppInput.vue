<script setup lang="ts">
withDefaults(
  defineProps<{
    modelValue: string | number | null;
    type?: "text" | "password" | "number";
    placeholder?: string;
    disabled?: boolean;
    autocomplete?: string;
    ignoreAutofill?: boolean;
  }>(),
  { type: "text", placeholder: "", disabled: false, autocomplete: undefined, ignoreAutofill: false },
);
const emit = defineEmits<{ "update:modelValue": [string] }>();

function clearReadonly(event: FocusEvent) {
  (event.target as HTMLInputElement).removeAttribute("readonly");
}
</script>

<template>
  <input
    class="app-input"
    :type="type"
    :value="modelValue ?? ''"
    :placeholder="placeholder"
    :disabled="disabled"
    :autocomplete="autocomplete"
    :readonly="ignoreAutofill || undefined"
    @focus="ignoreAutofill ? clearReadonly($event) : undefined"
    @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
  />
</template>

<style scoped>
.app-input {
  width: 100%;
  padding: 9px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text);
  transition: var(--transition);
}
.app-input:focus {
  outline: none;
  border-color: var(--brand);
}
.app-input:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
</style>
