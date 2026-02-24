export async function copyText(text: string): Promise<boolean> {
  const v = String(text ?? "");

  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(v);
      return true;
    }
  } catch (error) {
    console.warn("navigator.clipboard.writeText failed", error);
  }

  try {
    const ta = document.createElement("textarea");
    ta.value = v;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.top = "-1000px";
    ta.style.left = "-1000px";
    ta.style.opacity = "0";
    document.body.appendChild(ta);

    ta.focus({ preventScroll: true });
    ta.select();
    ta.setSelectionRange(0, ta.value.length);

    const ok = document.execCommand && document.execCommand("copy");
    document.body.removeChild(ta);
    if (ok) return true;
    console.warn('document.execCommand("copy") returned false');
  } catch (error) {
    console.warn('document.execCommand("copy") failed', error);
  }

  try {
    window.prompt("复制失败，请手动复制：", v);
  } catch (error) {
    console.warn("window.prompt copy fallback failed", error);
  }
  return false;
}

