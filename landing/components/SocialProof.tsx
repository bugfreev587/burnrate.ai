const pains = [
  'Runaway sessions that drain your quota overnight',
  'Silent rate limits with no warning',
  '"Why am I suddenly capped?" moments',
  'Unexpected API bills from a single bug',
]

export default function SocialProof() {
  return (
    <section aria-label="Pain points" className="border-b border-gray-100 bg-gray-50 py-12">
      <div className="mx-auto max-w-6xl px-4 sm:px-6">
        <p className="text-center text-sm font-medium text-gray-500 mb-8">
          Used by developers who rely on Claude Code every day — and don&apos;t want surprises
        </p>
        <ul className="flex flex-wrap justify-center gap-3" aria-label="Common problems">
          {pains.map((pain) => (
            <li
              key={pain}
              className="inline-flex items-center gap-2 rounded-full border border-gray-200 bg-white px-4 py-2 text-sm text-gray-600"
            >
              <span aria-hidden="true" className="text-red-400 font-bold">✕</span>
              {pain}
            </li>
          ))}
        </ul>
      </div>
    </section>
  )
}
