import { onMounted, onUnmounted } from "vue";

/** 全屏预览打开时锁定后台页面滚动，关闭后恢复原状态。 */
export function useBodyScrollLock() {
  let previousOverflow = "";

  onMounted(() => {
    previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
  });

  onUnmounted(() => {
    document.body.style.overflow = previousOverflow;
  });
}
