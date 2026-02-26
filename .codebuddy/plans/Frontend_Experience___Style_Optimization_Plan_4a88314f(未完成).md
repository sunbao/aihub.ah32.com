---
name: Frontend Experience & Style Optimization Plan
overview: Enhance frontend with modern UI polishing, smooth page transitions, and performance optimizations (lazy loading, caching).
design:
  architecture:
    framework: react
    component: shadcn
  styleKeywords:
    - Glassmorphism
    - Clean
    - Modern
  fontSystem:
    fontFamily: Inter, system-ui, sans-serif
    heading:
      size: 1.25rem
      weight: 600
    subheading:
      size: 1rem
      weight: 500
    body:
      size: 0.875rem
      weight: 400
  colorSystem:
    primary:
      - "#0f172a"
    background:
      - "#ffffff"
      - rgba(255, 255, 255, 0.8)
    text:
      - "#020817"
      - "#64748b"
    functional:
      - "#ef4444"
      - "#22c55e"
todos:
  - id: create-components
    content: Create Loading and ErrorBoundary components
    status: pending
  - id: optimize-build
    content: Configure manualChunks in vite.config.ts
    status: pending
  - id: implement-lazy-routes
    content: Refactor App.tsx to use React.lazy and Suspense
    status: pending
    dependencies:
      - create-components
  - id: enhance-ui
    content: Polish AppShell UI with better glassmorphism effects
    status: pending
    dependencies:
      - create-components
---

## User Requirements

Optimize the `webapp` frontend focusing on performance, stability, and visual experience.

## Core Features

- **Performance**: Implement Route Lazy Loading to reduce initial bundle size and improve load speed.
- **Build Optimization**: Configure Vite manual chunks for better caching of vendor libraries.
- **Stability**: Add a Global Error Boundary to catch crashes and prevent white screens.
- **UX Enhancement**: Add a polished Loading state and improve the visual design of the AppShell (navigation/header) with glassmorphism effects.

## Tech Stack

- **Framework**: React + TypeScript
- **Build Tool**: Vite
- **Styling**: Tailwind CSS + tailwindcss-animate
- **Routing**: React Router DOM

## Implementation Approach

1.  **Code Splitting**: Use `React.lazy()` and `<Suspense>` in `App.tsx` to load page components on demand.
2.  **Error Handling**: Implement a class-based `ErrorBoundary` component to catch render errors.
3.  **Build Config**: Update `vite.config.ts` `rollupOptions` to separate `react`, `react-dom`, and UI libraries into distinct chunks.
4.  **UI/UX**:

    -   Create a centered `Loading` spinner component.
    -   Update `AppShell` to use enhanced `backdrop-blur` and border styles for a more modern feel.

## Directory Structure

```
webapp/src/
├── components/
│   ├── ErrorBoundary.tsx   # [NEW] Global error catcher with "Try Again" UI
│   └── Loading.tsx         # [NEW] Centered loading spinner for Suspense fallback
├── App.tsx                 # [MODIFY] Implement React.lazy and Suspense wrapper
├── app/
│   └── AppShell.tsx        # [MODIFY] Enhance visual style (glassmorphism, spacing)
└── vite.config.ts          # [MODIFY] Add manualChunks configuration
```

## Design Style

Refine the existing "Modern Clean" aesthetic.

- **Glassmorphism**: Enhance the header and bottom navigation with stronger blur effects (`backdrop-blur-md`) and subtle borders to create depth.
- **Transitions**: Add smooth fade-in animations for page content.
- **Feedback**: Clear, centered loading indicators and friendly error states.