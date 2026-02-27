import { createContext, useContext, useMemo, useState, type PropsWithChildren } from "react";

export type Locale = "zh-CN" | "en";

const LOCALE_STORAGE_KEY = "aihub_locale_v1";

function normalizeLocale(input: string): Locale | "" {
  const v = String(input ?? "").trim().toLowerCase();
  if (!v) return "";
  if (v === "en" || v.startsWith("en-")) return "en";
  if (v === "zh" || v.startsWith("zh-")) return "zh-CN";
  return "";
}

export function isZhLocale(locale: string): boolean {
  return normalizeLocale(locale) === "zh-CN";
}

export function getStoredLocale(): Locale | "" {
  try {
    if (typeof localStorage === "undefined") return "";
    return normalizeLocale(localStorage.getItem(LOCALE_STORAGE_KEY) ?? "");
  } catch {
    return "";
  }
}

export function detectLocale(): Locale {
  try {
    const langs =
      typeof navigator !== "undefined" && Array.isArray((navigator as any).languages)
        ? ((navigator as any).languages as string[])
        : [];
    for (const l of langs) {
      const n = normalizeLocale(l);
      if (n) return n;
    }
    const n = normalizeLocale(typeof navigator !== "undefined" ? (navigator as any).language : "");
    if (n) return n;
  } catch {
    // ignore
  }
  return "zh-CN";
}

export function getPreferredLocale(): Locale {
  return getStoredLocale() || detectLocale();
}

export function setPreferredLocale(locale: Locale): void {
  try {
    if (typeof localStorage === "undefined") return;
    localStorage.setItem(LOCALE_STORAGE_KEY, locale);
  } catch {
    // ignore
  }
}

type I18nContextValue = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
};

const I18nContext = createContext<I18nContextValue | null>(null);

export function I18nProvider({ children }: PropsWithChildren) {
  const [locale, setLocaleState] = useState<Locale>(() => getPreferredLocale());

  const value = useMemo<I18nContextValue>(() => {
    return {
      locale,
      setLocale: (next) => {
        setPreferredLocale(next);
        setLocaleState(next);
      },
    };
  }, [locale]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): {
  locale: Locale;
  isZh: boolean;
  t: (m: { zh: string; en: string }) => string;
  setLocale: (locale: Locale) => void;
} {
  const ctx = useContext(I18nContext);
  const locale = ctx?.locale ?? getPreferredLocale();
  const isZh = isZhLocale(locale);
  return {
    locale,
    isZh,
    setLocale: ctx?.setLocale ?? (() => undefined),
    t: (m) => (isZh ? m.zh : m.en),
  };
}
