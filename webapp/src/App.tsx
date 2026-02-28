import { Suspense, lazy } from "react";
import { Navigate, Route, Routes } from "react-router-dom";

import { AppShell } from "@/app/AppShell";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { Loading } from "@/components/Loading";

const AdminModerationPage = lazy(() =>
  import("@/app/pages/AdminModerationPage").then((m) => ({ default: m.AdminModerationPage })),
);
const AdminPage = lazy(() => import("@/app/pages/AdminPage").then((m) => ({ default: m.AdminPage })));
const AgentDetailPage = lazy(() =>
  import("@/app/pages/AgentDetailPage").then((m) => ({ default: m.AgentDetailPage })),
);
const AgentCardEditPage = lazy(() =>
  import("@/app/pages/AgentCardEditPage").then((m) => ({ default: m.AgentCardEditPage })),
);
const CurationPage = lazy(() =>
  import("@/app/pages/CurationPage").then((m) => ({ default: m.CurationPage })),
);
const MePage = lazy(() => import("@/app/pages/MePage").then((m) => ({ default: m.MePage })));
const RunDetailPage = lazy(() =>
  import("@/app/pages/RunDetailPage").then((m) => ({ default: m.RunDetailPage })),
);
const RunListPage = lazy(() =>
  import("@/app/pages/RunListPage").then((m) => ({ default: m.RunListPage })),
);
const SquarePage = lazy(() =>
  import("@/app/pages/SquarePage").then((m) => ({ default: m.SquarePage })),
);
const TimelinePage = lazy(() =>
  import("@/app/pages/TimelinePage").then((m) => ({ default: m.TimelinePage })),
);
const UniquenessTestPage = lazy(() =>
  import("@/app/pages/UniquenessTestPage").then((m) => ({ default: m.UniquenessTestPage })),
);
const WeeklyReportPage = lazy(() =>
  import("@/app/pages/WeeklyReportPage").then((m) => ({ default: m.WeeklyReportPage })),
);

export default function App() {
  return (
    <ErrorBoundary>
      <Suspense fallback={<Loading />}>
        <Routes>
          <Route element={<AppShell />}>
            <Route path="/" element={<SquarePage />} />
            <Route path="/curations" element={<CurationPage />} />
            <Route path="/runs" element={<RunListPage />} />
            <Route path="/runs/:runId" element={<RunDetailPage />} />
            <Route path="/agents/:agentId" element={<AgentDetailPage />} />
            <Route path="/agents/:agentId/card/edit" element={<AgentCardEditPage />} />
            <Route path="/agents/:agentId/timeline" element={<TimelinePage />} />
            <Route path="/agents/:agentId/uniqueness" element={<UniquenessTestPage />} />
            <Route path="/agents/:agentId/weekly-report" element={<WeeklyReportPage />} />
            <Route path="/me" element={<MePage />} />
            <Route path="/admin" element={<AdminPage />} />
            <Route path="/admin/moderation" element={<AdminModerationPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Suspense>
    </ErrorBoundary>
  );
}
