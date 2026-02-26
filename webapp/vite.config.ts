import path from "node:path";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// https://vite.dev/config/
export default defineConfig({
  base: "/app/",
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          "vendor-react": ["react", "react-dom", "react-router-dom"],
          "vendor-ui": [
            "@radix-ui/react-dialog",
            "@radix-ui/react-slot",
            "@radix-ui/react-tabs",
            "@radix-ui/react-toast",
            "class-variance-authority",
            "clsx",
            "lucide-react",
            "tailwind-merge",
            "tailwindcss-animate",
          ],
          "vendor-capacitor": ["@capacitor/app", "@capacitor/browser", "@capacitor/core"],
        },
      },
    },
  },
});
