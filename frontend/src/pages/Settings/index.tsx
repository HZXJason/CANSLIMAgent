import { Settings } from 'lucide-react';
import { Card } from '../../components/ui/card';
import { API_BASE_URL } from '../../lib/api';

export default function SettingsPage() {
  return (
    <section className="mx-auto w-full max-w-[1180px] px-4 py-6 sm:px-6 lg:px-8">
      <header className="rounded-lg border border-slate-800/90 bg-slate-900/70 p-5 shadow-2xl shadow-slate-950/25 backdrop-blur">
        <p className="text-sm font-semibold text-cyan-300/80">AlphaLens</p>
        <h1 className="mt-2 text-3xl font-black tracking-tight text-white">设置</h1>
      </header>

      <Card className="mt-6 rounded-lg">
        <div className="flex items-start gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-slate-950/70 text-cyan-300">
            <Settings className="h-5 w-5" />
          </div>
          <div>
            <h2 className="text-lg font-bold text-white">本地运行配置</h2>
            <p className="mt-3 text-sm leading-6 text-slate-400">当前 API 地址：{API_BASE_URL}</p>
            <p className="mt-2 text-sm leading-6 text-slate-500">后续可以把数据源 Key、缓存策略和界面偏好集中放在这里。</p>
          </div>
        </div>
      </Card>
    </section>
  );
}
