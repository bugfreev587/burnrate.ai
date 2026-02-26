const capabilities: { text: string; comingSoon?: boolean }[] = [
  { text: 'Real-time usage visibility across all providers' },
  { text: 'Budget caps with hard stops — block before overspend' },
  { text: 'Rate limiting per key, model & provider' },
  { text: 'Projected monthly spend & trend analysis' },
  { text: 'Efficiency insights to cut waste', comingSoon: true },
]

export default function LandingSolution() {
  return (
    <section aria-labelledby="solution-heading" className="py-20 sm:py-24 bg-gray-950 text-white">
      <div className="mx-auto max-w-6xl px-4 sm:px-6">
        <div className="grid md:grid-cols-2 gap-12 items-center">
          <div>
            <p className="text-sm font-semibold text-blue-400 uppercase tracking-widest mb-3">The Solution</p>
            <h2 id="solution-heading" className="text-3xl sm:text-4xl font-bold mb-5">
              TokenGate is the Control Plane for AI Usage.
            </h2>
            <p className="text-gray-400 text-lg leading-relaxed">
              It sits between your tools and models to give you full visibility and enforcement — in real time,
              for every request.
            </p>
          </div>

          <ul className="space-y-3" aria-label="Core capabilities">
            {capabilities.map((item) => (
              <li
                key={item.text}
                className="flex items-center gap-4 rounded-xl border border-white/10 bg-white/5 px-5 py-4"
              >
                <span
                  aria-hidden="true"
                  className="shrink-0 flex h-7 w-7 items-center justify-center rounded-full bg-blue-600 text-sm font-bold text-white"
                >
                  ✓
                </span>
                <span className="text-gray-200">
                  {item.text}
                  {item.comingSoon && (
                    <span className="ml-1.5 inline-flex items-center rounded-full bg-amber-50 px-2 py-0.5 text-[10px] font-semibold text-amber-600 border border-amber-200">
                      Coming Soon
                    </span>
                  )}
                </span>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </section>
  )
}
