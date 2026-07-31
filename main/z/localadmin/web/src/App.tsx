import { Route, Routes } from 'react-router-dom'
import { AppShell } from '@/components/layout/app-shell'
import DiagnosePage from '@/features/diagnose/diagnose-page'
import DomainRoutePage from '@/features/domain-route/domain-route-page'
import LogPage from '@/features/log/log-page'
import MitmPage from '@/features/mitm/mitm-page'
import SettingsPage from '@/features/settings/settings-page'
import StatusPage from '@/features/status/status-page'
import TunPage from '@/features/tun/tun-page'

export default function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<StatusPage />} />
        <Route path="diagnose" element={<DiagnosePage />} />
        <Route path="domain-route" element={<DomainRoutePage />} />
        <Route path="tun" element={<TunPage />} />
        <Route path="mitm" element={<MitmPage />} />
        <Route path="log" element={<LogPage />} />
        <Route path="settings" element={<SettingsPage />} />
      </Route>
    </Routes>
  )
}
