export default function FinalCTA() {
  return (
    <section aria-labelledby="cta-heading" className="py-24 sm:py-32 bg-gray-950 text-white">
      <div className="mx-auto max-w-3xl px-4 sm:px-6 text-center">
        <h2 id="cta-heading" className="text-3xl sm:text-4xl md:text-5xl font-bold mb-4 leading-tight">
          Stop guessing.
          <br />
          Start controlling.
        </h2>
        <p className="text-gray-400 text-lg max-w-xl mx-auto mb-10 leading-relaxed">
          Whether your plan is &ldquo;monthly&rdquo; or &ldquo;per token,&rdquo; TokenGate gives you
          visibility, guardrails, and confidence.
        </p>
        <div className="flex flex-col sm:flex-row gap-3 justify-center">
          <a
            href="https://app.tokengate.to/sign-up"
            className="inline-flex items-center justify-center rounded-lg bg-blue-600 px-8 py-4 text-base font-semibold text-white hover:bg-blue-700 transition-colors"
          >
            Start Free
          </a>
          <a
            href="https://app.tokengate.to"
            className="inline-flex items-center justify-center rounded-lg border border-white/15 bg-white/5 px-8 py-4 text-base font-semibold text-white hover:bg-white/10 transition-colors"
          >
            See a Demo Dashboard →
          </a>
        </div>
      </div>
    </section>
  )
}
