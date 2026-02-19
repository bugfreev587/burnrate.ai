const subscriptionPains = [
  "You don't know your real usage",
  'You hit caps unexpectedly',
  "You can't tell which repo or workflow is consuming your quota",
]

const apiPains = [
  'You do know usage… after the bill arrives',
  'One bug can burn $200 overnight',
  "Teams can't enforce budgets consistently",
]

export default function LandingProblem() {
  return (
    <section id="problem" aria-labelledby="problem-heading" className="py-20 sm:py-24 bg-white">
      <div className="mx-auto max-w-6xl px-4 sm:px-6">
        <div className="text-center mb-14">
          <p className="text-sm font-semibold text-blue-600 uppercase tracking-widest mb-3">The Problem</p>
          <h2 id="problem-heading" className="text-3xl sm:text-4xl font-bold text-gray-900">
            AI feels unlimited…
            <br className="hidden sm:block" /> until it isn&apos;t.
          </h2>
        </div>

        <div className="grid md:grid-cols-2 gap-6">
          <div className="rounded-2xl border border-gray-200 p-8">
            <span className="inline-block rounded-lg bg-purple-50 px-3 py-1 text-sm font-semibold text-purple-700 mb-4">
              Subscription Plans (Claude Pro, ChatGPT Plus)
            </span>
            <h3 className="text-xl font-bold text-gray-900 mb-5">You don&apos;t see the problem coming</h3>
            <ul className="space-y-3" aria-label="Subscription plan problems">
              {subscriptionPains.map((item) => (
                <li key={item} className="flex items-start gap-3 text-gray-600">
                  <span aria-hidden="true" className="mt-0.5 shrink-0 text-red-400 font-bold">✕</span>
                  <span>{item}</span>
                </li>
              ))}
            </ul>
          </div>

          <div className="rounded-2xl border border-gray-200 p-8">
            <span className="inline-block rounded-lg bg-orange-50 px-3 py-1 text-sm font-semibold text-orange-700 mb-4">
              API Keys (OpenAI / Anthropic / Gemini)
            </span>
            <h3 className="text-xl font-bold text-gray-900 mb-5">You find out after the damage</h3>
            <ul className="space-y-3" aria-label="API key problems">
              {apiPains.map((item) => (
                <li key={item} className="flex items-start gap-3 text-gray-600">
                  <span aria-hidden="true" className="mt-0.5 shrink-0 text-red-400 font-bold">✕</span>
                  <span>{item}</span>
                </li>
              ))}
            </ul>
          </div>
        </div>

        <p className="text-center mt-10 text-xl font-semibold text-gray-900">
          Either way: you don&apos;t have control.
        </p>
      </div>
    </section>
  )
}
