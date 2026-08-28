import { FormEvent, useEffect, useRef, useState } from 'react';
import { Bot, Loader2, Send, X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { askResearchAssistant } from '../../lib/todayApi';
import { FALLBACK_AI_ANALYSIS_OPTIONS, normalizeAIAnalysisConfig, readStoredAIAnalysisConfig } from '../../lib/aiAnalysisConfig';

type Message = { role: 'user' | 'assistant'; content: string };
const suggestions = [
  '你可以问问我 CPI 指数对股市有哪些影响？',
  '想知道 FOMC 为什么会影响成长股吗？',
  '可以问我非农数据和利率预期的关系。'
];

export function ResearchAssistant({ market }: { market: 'US' | 'CN' }) {
  const [open, setOpen] = useState(false);
  const [bubbleVisible, setBubbleVisible] = useState(true);
  const [suggestionIndex, setSuggestionIndex] = useState(0);
  const [question, setQuestion] = useState('');
  const [messages, setMessages] = useState<Message[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  useEffect(() => {
    if (open) return;
    const interval = window.setInterval(() => {
      setSuggestionIndex((current) => (current + 1) % suggestions.length);
      setBubbleVisible(true);
      window.setTimeout(() => setBubbleVisible(false), 8_000);
    }, 28_000);
    return () => window.clearInterval(interval);
  }, [open]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const text = question.trim();
    if (!text || loading) return;
    const config = normalizeAIAnalysisConfig(readStoredAIAnalysisConfig(), FALLBACK_AI_ANALYSIS_OPTIONS);
    setMessages((current) => [...current, { role: 'user', content: text }]);
    setQuestion('');
    setLoading(true);
    setError(null);
    try {
      const response = await askResearchAssistant({ question: text, market, llm_provider: config.llm_provider, model: config.assistant_model, ...(config.backend_url.trim() ? { backend_url: config.backend_url.trim() } : {}) });
      setMessages((current) => [...current, { role: 'assistant', content: response.answer }]);
    } catch {
      setError('暂时无法回答，请检查设置中的 API 配置。');
    } finally {
      setLoading(false);
    }
  };

  return <div className="fixed bottom-[calc(5rem+env(safe-area-inset-bottom))] right-4 z-40 sm:bottom-6 sm:right-6">
    {!open && bubbleVisible ? <button type="button" onClick={() => { setOpen(true); setBubbleVisible(false); }} className="absolute bottom-[4.4rem] right-0 w-[min(18rem,calc(100vw-2rem))] rounded-sm border border-slate-700 bg-slate-900 px-4 py-3 text-left text-sm leading-5 text-slate-200 shadow-[0_12px_32px_rgba(0,0,0,0.4)] transition-colors hover:border-brand-300/45 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-300/60">
      {suggestions[suggestionIndex]}
    </button> : null}

    {open ? <section aria-label="AI 助手" className="mb-3 flex h-[min(30rem,calc(100vh-7rem))] w-[min(23rem,calc(100vw-2rem))] flex-col overflow-hidden rounded-sm border border-slate-700 bg-slate-950 shadow-[0_20px_56px_rgba(0,0,0,0.55)]">
      <header className="flex items-center justify-between border-b border-rule px-4 py-3">
        <div className="flex items-center gap-2.5"><span className="flex h-8 w-8 items-center justify-center rounded-full border border-brand-300/35 bg-brand-300/10 text-brand-200"><Bot className="h-4 w-4" /></span><h2 className="font-bold text-white">AI 助手</h2></div>
        <button type="button" aria-label="关闭 AI 助手" onClick={() => setOpen(false)} className="rounded-sm p-2 text-slate-400 hover:bg-slate-800 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-300/60"><X className="h-4 w-4" /></button>
      </header>
      <div className="flex-1 space-y-3 overflow-y-auto px-4 py-4" aria-live="polite">
        {messages.length === 0 ? <div className="space-y-2">{suggestions.map((suggestion) => <button key={suggestion} type="button" onClick={() => setQuestion(suggestion.replace(/^你可以问问我 |^想知道 |^可以问我 /, '').replace(/[。？]$/, '？'))} className="block w-full rounded-sm border border-slate-800 px-3 py-2.5 text-left text-sm text-slate-300 transition-colors hover:border-slate-600 hover:bg-slate-900">{suggestion}</button>)}</div> : null}
        {messages.map((message, index) => <div key={`${message.role}-${index}`} className={message.role === 'user' ? 'ml-8 rounded-sm bg-brand-300/12 px-3 py-2.5 text-sm text-brand-50' : 'mr-4 border-l border-slate-700 pl-3 text-sm leading-6 text-slate-200'}>{message.content}</div>)}
        {loading ? <div className="flex items-center gap-2 text-sm text-slate-400"><Loader2 className="h-4 w-4 animate-spin" />正在回答</div> : null}
        {error ? <p className="text-sm text-rose-300">{error}</p> : null}
      </div>
      <form onSubmit={(event) => void submit(event)} className="flex gap-2 border-t border-rule p-3">
        <input ref={inputRef} value={question} onChange={(event) => setQuestion(event.target.value)} maxLength={500} placeholder="输入你的问题" className="min-h-11 min-w-0 flex-1 rounded-sm border border-slate-700 bg-slate-900 px-3 text-sm text-white outline-none placeholder:text-slate-500 focus:border-brand-300/60 focus:ring-2 focus:ring-brand-300/15" />
        <Button type="submit" aria-label="发送" className="min-h-11 w-11 shrink-0 px-0" disabled={!question.trim() || loading}><Send className="h-4 w-4" /></Button>
      </form>
    </section> : null}

    <button type="button" aria-label={open ? '收起 AI 助手' : '打开 AI 助手'} onClick={() => { setOpen((current) => !current); setBubbleVisible(false); }} className="ml-auto flex h-14 w-14 items-center justify-center rounded-full border border-brand-300/45 bg-slate-950 text-brand-200 shadow-[0_10px_30px_rgba(0,0,0,0.45)] transition-[transform,background-color,border-color] duration-200 hover:-translate-y-0.5 hover:border-brand-200 hover:bg-slate-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-300/70 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas motion-reduce:transform-none">
      {open ? <X className="h-5 w-5" /> : <Bot className="h-6 w-6" />}
    </button>
  </div>;
}
