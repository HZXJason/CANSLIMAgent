import { useMemo, useState } from 'react';
import { Activity, ArrowUpRight, BarChart3, BookOpen, Check, ChevronDown, CircleAlert, FileText, LayoutGrid, ListFilter, Menu, Plus, Search, Settings2, SlidersHorizontal, Sparkles, Target, X } from 'lucide-react';

type Stock = { symbol: string; name: string; price: string; change: string; score: string; tags: string[]; status: 'ready' | 'review' };

const stocks: Stock[] = [
  { symbol: 'DELL', name: 'Dell Technologies', price: '$141.22', change: '+2.8%', score: '92', tags: ['C', 'A', 'N', 'L'], status: 'ready' },
  { symbol: 'EME', name: 'EMCOR Group', price: '$612.40', change: '+1.4%', score: '88', tags: ['C', 'S', 'L'], status: 'review' },
  { symbol: 'FIX', name: 'Comfort Systems USA', price: '$1,126.18', change: '+0.9%', score: '86', tags: ['A', 'N', 'I'], status: 'ready' },
  { symbol: 'GEV', name: 'GE Vernova', price: '$518.71', change: '-0.6%', score: '79', tags: ['N', 'S'], status: 'review' }
];

const nav = [
  { label: '证据扫描', icon: LayoutGrid, active: true },
  { label: '个股分析', icon: BarChart3 },
  { label: '交易计划', icon: Target },
  { label: '交易日志', icon: BookOpen }
];

export default function App() {
  const [market, setMarket] = useState<'US' | 'CN'>('US');
  const [reviewOnly, setReviewOnly] = useState(false);
  const [query, setQuery] = useState('');
  const [notice, setNotice] = useState('');
  const filtered = useMemo(() => stocks.filter((stock) => (!reviewOnly || stock.status === 'review') && (`${stock.symbol} ${stock.name}`).toLowerCase().includes(query.toLowerCase())), [query, reviewOnly]);

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand"><span className="brand-mark">AL</span><div><strong>AlphaLens</strong><span>证券研究终端</span></div></div>
        <div className="market-switch" role="group" aria-label="市场切换">
          <button className={market === 'US' ? 'selected' : ''} onClick={() => setMarket('US')}>US</button>
          <button className={market === 'CN' ? 'selected' : ''} onClick={() => setMarket('CN')}>A股</button>
        </div>
        <nav aria-label="主导航">{nav.map(({ label, icon: Icon, active }) => <button key={label} className={`nav-item ${active ? 'active' : ''}`} onClick={() => setNotice(`${label}模块将在下一阶段接入真实数据`)}><Icon size={17} /><span>{label}</span>{active && <i />}</button>)}</nav>
        <div className="sidebar-foot"><div className="pulse"><span /> 数据源已连接</div><button className="nav-item" onClick={() => setNotice('设置模块将在下一阶段接入')}><Settings2 size={17} /><span>设置</span></button></div>
      </aside>

      <main className="main">
        <header className="topbar"><button className="mobile-menu" aria-label="打开菜单"><Menu size={20} /></button><div className="crumb"><span>工作台</span><ChevronDown size={14} /><b>证据扫描</b></div><div className="top-actions"><span className="asof">数据截至 2026-07-10</span><button className="icon-btn" aria-label="搜索"><Search size={18} /></button><button className="avatar" aria-label="账户">J</button></div></header>

        <div className="content">
          {notice && <div className="notice" role="status"><CircleAlert size={16} />{notice}<button onClick={() => setNotice('')} aria-label="关闭提示"><X size={15} /></button></div>}
          <section className="intro"><div><p className="section-kicker">TERMINAL / {market === 'US' ? 'US EQUITIES' : 'A-SHARES'}</p><h1>证据扫描</h1><p className="intro-copy">证券研究终端 · 来源 / 期间 / 状态</p></div><button className="primary" onClick={() => setNotice('扫描任务已加入队列')}><Activity size={16} />重新扫描 <span>⌘ ↵</span></button></section>

          <section className="stats-grid" aria-label="扫描摘要">
            <div className="stat stat-feature"><span>候选池</span><strong>503</strong><small>本次读取的股票</small><div className="spark-bars" aria-hidden="true">{[30,52,42,70,60,84,78,96,88].map((height, i) => <i key={i} style={{ height: `${height}%` }} />)}</div></div>
            <div className="stat"><span>达标</span><strong>160</strong><small><em>+12</em> 相比上次扫描</small></div>
            <div className="stat"><span>多视角分歧</span><strong className="warn">24</strong><small>存在来源或期间冲突</small></div>
            <div className="stat"><span>扫描状态</span><strong className="status-live"><i />已完成</strong><small>耗时 02:14 · 本地快照</small></div>
          </section>

          <section className="workspace"><div className="workspace-head"><div><h2>候选证据表</h2><p>按 CANSLIM 标签、来源和数据完整度排序</p></div><div className="view-toggle"><button className="active" aria-label="表格视图"><ListFilter size={16} /></button><button aria-label="卡片视图"><LayoutGrid size={16} /></button></div></div>
            <div className="toolbar"><div className="search"><Search size={16} /><input aria-label="搜索股票" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索代码或公司名称" /></div><button className={`filter ${reviewOnly ? 'filter-on' : ''}`} onClick={() => setReviewOnly((value) => !value)}><SlidersHorizontal size={15} />只看需要核查 {reviewOnly && <Check size={14} />}</button><button className="filter secondary"><FileText size={15} />筛选条件 <ChevronDown size={14} /></button></div>
            <div className="table-wrap"><table><thead><tr><th>标的</th><th>价格</th><th>CANSLIM</th><th>评分</th><th>证据状态</th><th aria-label="操作" /></tr></thead><tbody>{filtered.map((stock) => <tr key={stock.symbol}><td><div className="ticker"><span>{stock.symbol}</span><small>{stock.name}</small></div></td><td><strong className="price">{stock.price}</strong><small className={stock.change.startsWith('+') ? 'up' : 'down'}>{stock.change}</small></td><td><div className="tags">{stock.tags.map((tag) => <span key={tag}>{tag}</span>)}</div></td><td><span className="score">{stock.score}</span><small>/ 100</small></td><td><span className={`evidence ${stock.status}`}><i />{stock.status === 'ready' ? '证据齐全' : '需要核查'}</span></td><td><button className="row-action" onClick={() => setNotice(`${stock.symbol} 已加入分析队列`)}><ArrowUpRight size={16} /></button></td></tr>)}</tbody></table>{filtered.length === 0 && <div className="empty">没有符合当前条件的标的。</div>}</div>
          </section>

          <section className="lower-grid"><div className="brief"><div className="brief-head"><div><p className="section-kicker">RESEARCH QUEUE</p><h2>下一步核查</h2></div><Sparkles size={18} /></div><div className="brief-row"><span className="index">01</span><div><strong>确认 EME 的盈利期间</strong><p>财报日期与行情日期存在 3 天偏差</p></div><button onClick={() => setNotice('已打开 EME 核查记录')}>打开 <ArrowUpRight size={14} /></button></div><div className="brief-row"><span className="index">02</span><div><strong>补充 GEV 的行业对比</strong><p>缺少同业相对强度来源</p></div><button onClick={() => setNotice('已打开 GEV 核查记录')}>打开 <ArrowUpRight size={14} /></button></div></div><div className="principle"><div className="principle-line" /><p>RESEARCH, THEN DECIDE</p><strong>字段可以被核查，<br />判断仍然留在人手里。</strong><span>AlphaLens / terminal mode</span></div></section>
        </div>
      </main>
    </div>
  );
}
