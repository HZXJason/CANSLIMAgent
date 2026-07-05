import { Navigate, Route, Routes } from 'react-router-dom';
import AppShell from './components/AppShell';
import ScreenerPage from './pages/Screener';
import AnalysisPage from './pages/Analysis';
import JournalEntry from './pages/Journal/JournalEntry';
import JournalList from './pages/Journal/JournalList';
import SettingsPage from './pages/Settings';
import TradePlanEntry from './pages/TradePlans/TradePlanEntry';
import TradePlanList from './pages/TradePlans';

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
