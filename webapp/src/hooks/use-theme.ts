import { useEffect, useState } from "react";

type Theme = "light" | "dark" | "system";

const STORAGE_KEY = "aihub-theme";

function getSystemTheme(): "light" | "dark" {
  if (typeof window === "undefined") return "light";
  if (typeof window.matchMedia !== "function") return "light";
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function applyTheme(theme: Theme) {
  const root = document.documentElement;
  const resolved = theme === "system" ? getSystemTheme() : theme;
  root.classList.remove("light", "dark");
  root.classList.add(resolved);
  // 同步更新 meta theme-color
  const metaThemeColor = document.querySelector('meta[name="theme-color"]');
  if (metaThemeColor) {
    metaThemeColor.setAttribute("content", resolved === "dark" ? "#0d0f1a" : "#f5f5fa");
  }
}

function getSavedTheme(): Theme {
  try {
    const stored = localStorage.getItem(STORAGE_KEY) as Theme | null;
    if (stored === "light" || stored === "dark" || stored === "system") return stored;
  } catch (error) {
    console.warn("[AIHub] Failed to read theme from localStorage", error);
  }
  return "system";
}

// 立即执行一次（在模块加载时），确保不闪屏
if (typeof document !== "undefined") {
  applyTheme(getSavedTheme());
}

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(getSavedTheme);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  // 跟随系统主题变化
  useEffect(() => {
    if (theme !== "system") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const handler = () => applyTheme("system");
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, [theme]);

  function setTheme(t: Theme) {
    try {
      localStorage.setItem(STORAGE_KEY, t);
    } catch (error) {
      console.warn("[AIHub] Failed to persist theme to localStorage", error);
    }
    setThemeState(t);
    applyTheme(t); // 立即应用，不等 useEffect
  }

  const resolved: "light" | "dark" = theme === "system" ? getSystemTheme() : theme;

  return { theme, setTheme, resolved };
}
