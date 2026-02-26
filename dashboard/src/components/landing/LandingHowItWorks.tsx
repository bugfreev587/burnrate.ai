export default function LandingHowItWorks() {
  return (
    <section id="how-it-works" aria-labelledby="hiw-heading" className="py-20 sm:py-24 bg-gray-950 text-white">
      <div className="mx-auto max-w-4xl px-4 sm:px-6">
        <div className="text-center mb-14">
          <p className="text-sm font-semibold text-blue-400 uppercase tracking-widest mb-3">Setup</p>
          <h2 id="hiw-heading" className="text-3xl sm:text-4xl font-bold">How It Works</h2>
          <p className="text-gray-400 mt-3">From zero to full visibility in under a minute. Example below uses Claude Code — works with any supported provider.</p>
        </div>

        <ol className="space-y-10">
          <li className="grid sm:grid-cols-[64px_1fr] gap-5 items-start">
            <div aria-hidden="true" className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl border border-white/10 bg-white/5 text-sm font-bold text-blue-400">
              01
            </div>
            <div>
              <h3 className="text-lg font-bold mb-1">Point your AI tools to TokenGate</h3>
              <p className="text-gray-400 mb-4">Set two environment variables. That&apos;s it. No code changes, no SDK, no new config files.</p>
              <div
                className="rounded-xl border border-white/10 bg-black p-5 font-mono text-sm overflow-x-auto"
                aria-label="Shell commands to configure TokenGate"
              >
                <div aria-hidden="true" className="flex gap-1.5 mb-4">
                  <span className="h-3 w-3 rounded-full bg-red-500/60" />
                  <span className="h-3 w-3 rounded-full bg-yellow-500/60" />
                  <span className="h-3 w-3 rounded-full bg-green-500/60" />
                </div>
                <code className="whitespace-pre">
                  <span className="text-purple-400">export </span>
                  <span className="text-blue-300">ANTHROPIC_BASE_URL</span>
                  <span className="text-gray-500">=</span>
                  <span className="text-green-400">https://gateway.tokengate.to/v1</span>
                  {'\n'}
                  <span className="text-purple-400">export </span>
                  <span className="text-blue-300">ANTHROPIC_API_KEY</span>
                  <span className="text-gray-500">=</span>
                  <span className="text-green-400">tg_xxxxx</span>
                </code>
              </div>
            </div>
          </li>

          <li className="grid sm:grid-cols-[64px_1fr] gap-5 items-start">
            <div aria-hidden="true" className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl border border-white/10 bg-white/5 text-sm font-bold text-blue-400">
              02
            </div>
            <div>
              <h3 className="text-lg font-bold mb-1">Use Claude Code as normal</h3>
              <p className="text-gray-400">Nothing changes in your workflow. Same commands, same editor, same speed.</p>
            </div>
          </li>

          <li className="grid sm:grid-cols-[64px_1fr] gap-5 items-start">
            <div aria-hidden="true" className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl border border-white/10 bg-white/5 text-sm font-bold text-blue-400">
              03
            </div>
            <div>
              <h3 className="text-lg font-bold mb-1">TokenGate tracks + enforces your policies</h3>
              <p className="text-gray-400">See usage in real time. Get alerts. Hit budget caps before your bill does.</p>
            </div>
          </li>
        </ol>
      </div>
    </section>
  )
}
