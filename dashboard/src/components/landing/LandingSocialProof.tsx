const pains = [
  'Runaway sessions that drain your quota overnight',
  'Silent rate limits with no warning',
  '"Why am I suddenly capped?" moments',
  'Unexpected API bills from a single bug',
]

export default function LandingSocialProof() {
  return (
    <section aria-label="Pain points" className="border-b border-white/10 bg-[#0b1220] py-12">
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <p className="text-center text-sm font-medium text-slate-400 mb-8">
          Used by developers who rely on AI Code Assistant every day — and don&apos;t want surprises
        </p>
        <ul className="flex flex-wrap justify-center gap-3" aria-label="Common problems">
          {pains.map((pain) => (
            <li
              key={pain}
              className="inline-flex items-center gap-2 rounded-full border border-white/15 bg-white/5 px-4 py-2 text-sm text-slate-300"
            >
              <span aria-hidden="true" className="text-rose-400 font-bold">✕</span>
              {pain}
            </li>
          ))}
        </ul>
      </div>
    </section>
  )
}
