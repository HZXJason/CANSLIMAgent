import { lazy } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import AppShell from './components/AppShell';

const ScreenerPage = lazy(() => import('./pages/Screener'));
const AnalysisPage = lazy(() => import('./pages/Analysis'));
const JournalEntry = lazy(() => import('./pages/Journal/JournalEntry'));
const JournalList = lazy(() => import('./pages/Journal/JournalList'));
const SettingsPage = lazy(() => import('./pages/Settings'));
const TradePlanEntry = lazy(() => import('./pages/TradePlans/TradePlanEntry'));
const TradePlanList = lazy(() => import('./pages/TradePlans'));

export default function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route path="/" element={<Navigate to="/screener" replace />} />
        <Route path="/screener" element={<ScreenerPage />} />
        <Route path="/analysis" element={<AnalysisPage />} />
        <Route path="/analysis/:symbol" element={<AnalysisPage />} />
        <Route path="/trade-plans" element={<TradePlanList />} />
        <Route path="/trade-plans/new" element={<TradePlanEntry />} />
        <Route path="/trade-plans/:id" element={<TradePlanEntry />} />
        <Route path="/journal" element={<JournalList />} />
        <Route path="/journal/new" element={<JournalEntry />} />
        <Route path="/journal/:id" element={<JournalEntry />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/screener" replace />} />
      </Route>
    </Routes>
  );
}
