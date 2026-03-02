const points = [
  'Tokens in / out per session',
  'Usage by repo / project',
  '"API cost equivalent" — what this would cost on API pricing',
  'Peak usage patterns that trigger caps',
]

const mockRows = [
  { label: 'Input tokens', sub: 'this session', value: '42,310' },
  { label: 'Output tokens', sub: 'this session', value: '8,920' },
  { label: 'API cost equivalent', sub: 'vs $0/subscription', value: '$0.83' },
  { label: 'Repository', sub: 'tracked', value: 'my-app / main' },
]

export default function LandingForSubscription() {
  return (
    <section aria-labelledby="sub-heading" className="py-20 sm:py-24 bg-[#070d18]">
      <div className="mx-auto max-w-6xl px-4 sm:px-6">
        <div className="grid md:grid-cols-2 gap-12 items-center">
          <div>
            <span className="inline-block rounded-lg bg-violet-500/15 px-3 py-1 text-sm font-semibold text-violet-200 mb-5">
              For Subscription Users
            </span>
            <h2 id="sub-heading" className="text-3xl sm:text-4xl font-bold text-slate-100 mb-4">
              Know what &ldquo;unlimited&rdquo; actually costs.
            </h2>
            <p className="text-slate-300 text-lg mb-8 leading-relaxed">
              Even if you pay $20/month, your usage isn&apos;t free — it&apos;s just hidden. TokenGate shows
              what&apos;s really happening inside every session.
            </p>
            <ul className="space-y-3 mb-8" aria-label="Subscription visibility features">
              {points.map((p) => (
                <li key={p} className="flex items-start gap-3 text-slate-200">
                  <span aria-hidden="true" className="mt-1 shrink-0 text-blue-400">✓</span>
                  <span>{p}</span>
                </li>
              ))}
            </ul>
            <div className="flex flex-wrap gap-6 text-sm font-semibold text-slate-300">
              <span>✓ Predictability</span>
              <span>✓ Fewer limit hits</span>
              <span>✓ Less wasted context</span>
            </div>
          </div>

          <div aria-label="Example session overview" className="rounded-2xl border border-white/15 bg-slate-950/70 p-6">
            <p className="text-xs font-semibold text-slate-400 uppercase tracking-widest mb-4">
              Session Overview
            </p>
            <dl className="divide-y divide-white/10">
              {mockRows.map((row) => (
                <div key={row.label} className="flex items-center justify-between py-3">
                  <div>
                    <dt className="text-sm font-medium text-slate-200">{row.label}</dt>
                    <dd className="text-xs text-slate-400">{row.sub}</dd>
                  </div>
                  <span className="font-mono text-sm font-bold text-slate-100">{row.value}</span>
                </div>
              ))}
            </dl>
          </div>
        </div>
      </div>
    </section>
  )
}
