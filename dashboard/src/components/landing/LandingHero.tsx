import { Link } from 'react-router-dom'

export default function LandingHero() {
  return (
    <section aria-label="Hero" className="relative bg-gray-950 text-white overflow-hidden pt-14">
      <div
        aria-hidden="true"
        className="absolute inset-0 bg-[linear-gradient(to_right,#ffffff07_1px,transparent_1px),linear-gradient(to_bottom,#ffffff07_1px,transparent_1px)] bg-[size:72px_72px]"
      />
      <div
        aria-hidden="true"
        className="absolute top-0 left-1/2 -translate-x-1/2 h-[500px] w-[700px] rounded-full bg-blue-600/10 blur-3xl pointer-events-none"
      />

      <div className="relative mx-auto max-w-4xl px-4 sm:px-6 pt-20 pb-28 text-center">
        <div className="inline-flex flex-wrap items-center justify-center gap-x-2 gap-y-1 rounded-full border border-white/10 bg-white/5 px-5 py-2 mb-10 text-xs sm:text-sm text-gray-400">
          <span aria-hidden="true" className="h-1.5 w-1.5 rounded-full bg-blue-500" />
          <span className="font-medium text-gray-300">Works in 30 seconds</span>
          <span aria-hidden="true" className="text-gray-600">·</span>
          <span>Set env vars</span>
          <span aria-hidden="true" className="text-gray-600">→</span>
          <span>Use Claude Code</span>
          <span aria-hidden="true" className="text-gray-600">→</span>
          <span>See usage + guardrails</span>
        </div>

        <h1 className="text-4xl sm:text-5xl md:text-6xl lg:text-7xl font-bold leading-[1.08] tracking-tight mb-5">
          Take Control of{' '}
          <span className="text-blue-400">Your AI</span>
        </h1>

        <p className="text-lg sm:text-xl text-gray-400 font-medium mb-6 tracking-wide">
          Spend · Limits · Usage · Efficiency
        </p>

        <p className="text-gray-400 text-base sm:text-lg max-w-2xl mx-auto mb-10 leading-relaxed">
          TokenGate is the control layer between your AI tools (Claude Code, Codex, etc) and LLM providers.
          Whether you pay per token or use a monthly subscription — get{' '}
          <span className="text-white font-medium">visibility + guardrails</span>.
        </p>

        <div className="flex flex-col sm:flex-row gap-3 justify-center">
          <Link
            to="/sign-up"
            className="inline-flex items-center justify-center rounded-lg bg-blue-600 px-7 py-3.5 text-base font-semibold text-white hover:bg-blue-700 transition-colors"
          >
            Start Free
          </Link>
          <a
            href="#how-it-works"
            className="inline-flex items-center justify-center rounded-lg border border-white/15 bg-white/5 px-7 py-3.5 text-base font-semibold text-white hover:bg-white/10 transition-colors"
          >
            Connect TokenGate in 30s →
          </a>
        </div>

        <div className="mt-16 flex flex-wrap items-center justify-center gap-x-6 gap-y-3 text-xs text-gray-600">
          <span className="uppercase tracking-widest font-medium">Works with</span>
          {['Claude Code', 'Anthropic API', 'Codex', 'OpenAI API', 'more coming soon'].map(
            (tool, i, arr) => (
              <span key={tool} className="flex items-center gap-6">
                <span className="text-gray-400">{tool}</span>
                {i < arr.length - 1 && <span aria-hidden="true" className="text-gray-700">·</span>}
              </span>
            ),
          )}
        </div>
      </div>
    </section>
  )
}
