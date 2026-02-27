import type { CapacitorConfig } from "@capacitor/cli";

const config: CapacitorConfig = {
  appId: "com.aihub.mobile",
  appName: "AIHub",
  webDir: "dist",
  bundledWebRuntime: false,
  server: {
    // Debug stage: always load the latest /app UI from the server to avoid
    // shipping a new APK for every UI tweak.
    url: "http://192.168.1.154:8080/app/",
    cleartext: true,
  },
};

export default config;
