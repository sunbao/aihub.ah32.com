import { Navigate, Route, Routes } from "react-router-dom";

import { AppShell } from "@/app/AppShell";
import { AdminAssignPage } from "@/app/pages/AdminAssignPage";
import { AdminModerationPage } from "@/app/pages/AdminModerationPage";
import { AgentDetailPage } from "@/app/pages/AgentDetailPage";
import { CurationPage } from "@/app/pages/CurationPage";
import { MePage } from "@/app/pages/MePage";
import { RunDetailPage } from "@/app/pages/RunDetailPage";
import { RunListPage } from "@/app/pages/RunListPage";
import { SquarePage } from "@/app/pages/SquarePage";
import { TimelinePage } from "@/app/pages/TimelinePage";
import { UniquenessTestPage } from "@/app/pages/UniquenessTestPage";
import { WeeklyReportPage } from "@/app/pages/WeeklyReportPage";

export default function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route path="/" element={<SquarePage />} />
        <Route path="/curations" element={<CurationPage />} />
        <Route path="/runs" element={<RunListPage />} />
        <Route path="/runs/:runId" element={<RunDetailPage />} />
        <Route path="/agents/:agentId" element={<AgentDetailPage />} />
        <Route path="/agents/:agentId/uniqueness" element={<UniquenessTestPage />} />
        <Route path="/agents/:agentId/weekly-report" element={<WeeklyReportPage />} />
        <Route path="/me" element={<MePage />} />
        <Route path="/me/timeline" element={<TimelinePage />} />
        <Route path="/admin/moderation" element={<AdminModerationPage />} />
        <Route path="/admin/assign" element={<AdminAssignPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
