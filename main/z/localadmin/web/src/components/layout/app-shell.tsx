import { NavLink, Outlet } from 'react-router-dom'
import { Activity, FileText, Network, Settings2 } from 'lucide-react'
import { cn } from '@/lib/utils'

const navItems = [
  { to: '/', label: '状态', icon: Activity, end: true },
  { to: '/tun', label: 'TUN', icon: Network, end: false },
  { to: '/log', label: '日志', icon: FileText, end: false },
  { to: '/settings', label: '连接配置', icon: Settings2, end: false },
]

export function AppShell() {
  return (
    <div className="bg-background min-h-svh">
      <header className="border-border bg-card/80 sticky top-0 z-10 border-b backdrop-blur">
        <div className="mx-auto flex h-12 max-w-5xl items-center justify-between gap-4 px-4">
          <div className="flex items-center gap-1">
            {navItems.map(({ to, label, icon: Icon, end }) => (
              <NavLink
                key={to}
                to={to}
                end={end}
                className={({ isActive }) =>
                  cn(
                    'inline-flex h-8 items-center gap-1.5 rounded-lg px-3 text-sm font-medium transition-colors',
                    isActive
                      ? 'bg-accent text-accent-foreground'
                      : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                  )
                }
              >
                <Icon className="size-4" />
                {label}
              </NavLink>
            ))}
          </div>
          <span className="text-muted-foreground hidden text-xs sm:inline">Rocket Agent 本地后台</span>
        </div>
      </header>
      <main className="mx-auto max-w-5xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  )
}
