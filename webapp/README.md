# AIHub WebApp (`/app/`)

移动端优先 UI（React + Vite + Tailwind + shadcn/ui），由后端在 `/app/` 路径提供（PWA），并可用 Capacitor 打包成 Android APK。

## 开发

前置：后端已启动（见仓库根目录 `README.md`）。

在 `webapp/` 目录：

- 安装依赖：`npm install`
- 启动开发服务器：`npm run dev`

### API baseUrl（很重要）

Web/PWA（由后端提供 `/app/`）一般不需要配置；但以下情况需要配置：
- 你用 `npm run dev` 本地开发（前后端不同源）
- 你要打包 Android APK

使用环境变量 `VITE_API_BASE_URL` 指向 AIHub 服务端，例如（PowerShell）：

`$env:VITE_API_BASE_URL="http://localhost:8080"`

## 构建

- `npm run build`

## Android APK（Capacitor）

### 一键构建 Debug APK

1) 确保 `VITE_API_BASE_URL` 已设置为**手机可访问**的 AIHub 地址（并且系统浏览器也能访问，用于 GitHub OAuth）。
2) 在 `webapp/` 目录执行：`npm run android:build:debug`
3) APK 输出路径：`webapp/android/app/build/outputs/apk/debug/app-debug.apk`

说明：
- `npm run build` 用于后端 `/app/`（资源路径前缀为 `/app/`）
- `npm run build:android` 用于 APK（资源路径为根路径 `/`，避免本地 WebView 找不到 `/app/assets/...`）

可选：打开 Android Studio（用于签名/Release 包/真机调试）
- `npm run android:open`

### 安装 APK

- `adb install -r webapp/android/app/build/outputs/apk/debug/app-debug.apk`

## GitHub OAuth（APK 深链交接）

APK 登录流程：
- App 打开系统浏览器访问：`{API_BASE}/v1/auth/github/start?flow=app`
- GitHub 登录完成后，回调页会重定向到：`aihub://auth/github?exchange_token=...`
- App 捕获深链后调用：`POST /v1/auth/app/exchange` 换取 AIHub 用户 API key，并写入本地存储

注意事项：
- 后端必须已配置 GitHub OAuth（`AIHUB_GITHUB_CLIENT_ID` / `AIHUB_GITHUB_CLIENT_SECRET`）。
- 如果服务端在反向代理后（或存在多个域名），建议设置 `AIHUB_PUBLIC_BASE_URL`，避免 OAuth cookie 写在 A 域名而回调落到 B 域名导致“登录过期”。
