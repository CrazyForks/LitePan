let lockCount = 0;
let savedBodyOverflow = "";
let savedBodyPaddingRight = "";
let savedHtmlOverflow = "";

function scrollbarWidth(): number {
  return Math.max(0, window.innerWidth - document.documentElement.clientWidth);
}

export function lockPageScroll(): void {
  lockCount += 1;
  if (lockCount > 1) return;

  const body = document.body;
  const html = document.documentElement;
  savedBodyOverflow = body.style.overflow;
  savedBodyPaddingRight = body.style.paddingRight;
  savedHtmlOverflow = html.style.overflow;

  const gutter = scrollbarWidth();
  html.style.overflow = "hidden";
  body.style.overflow = "hidden";
  if (gutter > 0) {
    body.style.paddingRight = `${gutter}px`;
  }
}

export function unlockPageScroll(): void {
  if (lockCount <= 0) return;
  lockCount -= 1;
  if (lockCount > 0) return;

  const body = document.body;
  const html = document.documentElement;
  html.style.overflow = savedHtmlOverflow;
  body.style.overflow = savedBodyOverflow;
  body.style.paddingRight = savedBodyPaddingRight;
}
