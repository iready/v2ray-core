import { Route, Routes } from 'react-router-dom'
import { AppShell } from '@/components/layout/app-shell'
import SettingsPage from '@/features/settings/settings-page'
import StatusPage from '@/features/status/status-page'
import LogPage from '@/features/log/log-page'
import TunPage from '@/features/tun/tun-page'

export default function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<StatusPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="tun" element={<TunPage />} />
        <Route path="log" element={<LogPage />} />
      </Route>
    </Routes>
  )
}
