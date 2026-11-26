import { lazy } from 'react';
import { Navigate, Route, Routes, useParams } from 'react-router-dom';
import AppShell from './components/AppShell';

const DeskPage = lazy(() => import('./pages/Desk'));
const ResearchPage = lazy(() => import('./pages/Research'));
const PositionsPage = lazy(() => import('./pages/Positions'));
const SystemPage = lazy(() => import('./pages/System'));
const JournalEntry = lazy(() => import('./pages/Journal/JournalEntry'));
const TradePlanEntry = lazy(() => import('./pages/TradePlans/TradePlanEntry'));

function LegacyAnalysisRedirect() {
  const { symbol } = useParams();
  return <Navigate to={symbol ? '/research/' + encodeURIComponent(symbol) : '/research'} replace />;
}

export default function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route path="/" element={<Navigate to="/desk" replace />} />
        <Route path="/desk" element={<DeskPage />} />
        <Route path="/research" element={<ResearchPage />} />
        <Route path="/research/:symbol" element={<ResearchPage />} />
        <Route path="/positions" element={<PositionsPage />} />
        <Route path="/system" element={<SystemPage />} />

        {/* Detail forms & backward-compatible redirects */}
        <Route path="/trade-plans/new" element={<TradePlanEntry />} />
        <Route path="/trade-plans/:id" element={<TradePlanEntry />} />
        <Route path="/journal/new" element={<JournalEntry />} />
        <Route path="/journal/:id" element={<JournalEntry />} />

        <Route path="/today" element={<Navigate to="/desk" replace />} />
        <Route path="/screener" element={<Navigate to="/research" replace />} />
        <Route path="/analysis" element={<Navigate to="/research" replace />} />
        <Route path="/analysis/:symbol" element={<LegacyAnalysisRedirect />} />
        <Route path="/trade-plans" element={<Navigate to="/positions?tab=plans" replace />} />
        <Route path="/journal" element={<Navigate to="/positions?tab=fills" replace />} />
        <Route path="/me" element={<Navigate to="/system" replace />} />
        <Route path="/settings" element={<Navigate to="/system?tab=connections" replace />} />
        <Route path="*" element={<Navigate to="/desk" replace />} />
      </Route>
    </Routes>
  );
}
