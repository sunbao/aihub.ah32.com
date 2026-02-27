import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";

import { I18nProvider } from "@/lib/i18n";

import "./index.css";
import App from "./App.tsx";

function getBasename() {
  const baseUrl = String(import.meta.env.BASE_URL ?? "/");
  if (!baseUrl || baseUrl === "/") return "";
  return baseUrl.endsWith("/") ? baseUrl.slice(0, -1) : baseUrl;
}

function registerServiceWorker() {
  if (!("serviceWorker" in navigator)) return;
  window.addEventListener("load", () => {
    const baseUrl = String(import.meta.env.BASE_URL ?? "/");
    const base = baseUrl.endsWith("/") ? baseUrl : `${baseUrl}/`;
    const swUrl = `${base}sw.js`;
    navigator.serviceWorker
      .register(swUrl, { scope: base, updateViaCache: "none" })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.warn("[AIHub] service worker register failed", err);
      });
  });
}

registerServiceWorker();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <I18nProvider>
      <BrowserRouter basename={getBasename()}>
        <App />
      </BrowserRouter>
    </I18nProvider>
  </StrictMode>,
);
