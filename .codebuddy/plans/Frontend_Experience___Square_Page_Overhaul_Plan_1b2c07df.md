---
name: Frontend Experience & Square Page Overhaul Plan
overview: "Comprehensively optimize frontend experience: implement Infinite Scroll for Square Page, enhance AppShell visuals, add global Page Transitions, and improve performance with Lazy Loading and Caching."
design:
  architecture:
    framework: react
    component: shadcn
  styleKeywords:
    - Glassmorphism
    - Fluid
    - Modern
  fontSystem:
    fontFamily: Inter, system-ui, sans-serif
    heading:
      size: 24px
      weight: 600
    subheading:
      size: 16px
      weight: 500
    body:
      size: 14px
      weight: 400
  colorSystem:
    primary:
      - "#0f172a"
    background:
      - "#ffffff"
      - "#020817"
    text:
      - "#0f172a"
      - "#f8fafc"
    functional:
      - "#3b82f6"
      - "#ef4444"
todos:
  - id: setup-components
    content: Create Loading (Skeleton) and ErrorBoundary components
    status: completed
  - id: polish-app-shell
    content: Refine AppShell with Glassmorphism and Page Transitions
    status: completed
    dependencies:
      - setup-components
  - id: analyze-square-page
    content: Use [subagent:code-explorer] to analyze SquarePage data fetching logic
    status: completed
  - id: optimize-square-page
    content: Refactor SquarePage to Infinite Scroll and Clickable Cards
    status: completed
    dependencies:
      - analyze-square-page
      - setup-components
  - id: impl-lazy-loading
    content: Refactor App.tsx to use React.lazy and Suspense
    status: completed
    dependencies:
      - setup-components
  - id: optimize-build
    content: Configure Vite manualChunks for cache optimization
    status: completed
---

## User Requirements

Optimize the frontend application (`webapp`) to enhance **User Experience (UX)** and **Visual Style (UI)**.

## Core Features

- **Visual Polish**: Adopt a modern "Glassmorphism" style for navigation and headers. Improve typography and spacing.
- **Fluid Experience**: Implement **Infinite Scroll** for the Square (listing) page to replace manual "Load More" buttons. Make content cards fully clickable.
- **Performance Perception**: Add **Skeleton Screens** (loading placeholders) instead of generic spinners. Implement **Page Transition** animations.
- **Technical Performance**: Enable **Route Lazy Loading** and **Vite Chunk Split** to reduce initial load time.
- **Stability**: Add a Global **Error Boundary** to prevent white-screen crashes.

## Tech Stack

- **Framework**: React 19 + Vite
- **Styling**: Tailwind CSS + tailwindcss-animate
- **Icons**: Lucide React
- **State/Logic**: React Hooks (IntersectionObserver for infinite scroll)

## Implementation Approach

1.  **Style System**: Refine `AppShell.tsx` and `index.css` to use backdrop-blur and semi-transparent backgrounds (Glassmorphism).
2.  **UX Patterns**:

    - Refactor `SquarePage` to use a `IntersectionObserver` hook for detecting scroll bottom and triggering data fetch.
    - Replace `Loader2` spinners with `Skeleton` components (shadcn/ui) for content loading.

3.  **Architecture**:

    - Wrap routes in `App.tsx` with `Suspense` and `React.lazy`.
    - Create a `GlobalErrorBoundary` component.

## Design Style

Adopt a **Modern Glassmorphism** aesthetic.

- **Translucency**: Use `bg-background/80` + `backdrop-blur-md` for sticky headers and bottom navigation.
- **Depth**: Subtle shadows and borders (`border-white/10` in dark mode) to define layers.
- **Feedback**: Interactive elements should have immediate visual feedback (scale/opacity) on touch/hover.

# Agent Extensions

- **code-explorer**
- Purpose: Analyze the current data fetching implementation in `SquarePage` to safely refactor it to infinite scroll.
- Expected outcome: Identify the `useQuery` or `fetch` logic and the "Load More" button structure.