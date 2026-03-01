import { useState } from 'react'

export default function LandingHowItWorks() {
  const [tab, setTab] = useState<'claude' | 'codex'>('claude')

  return (
    <section id="how-it-works" aria-labelledby="hiw-heading" className="py-20 sm:py-24 bg-gray-950 text-white">
      <div className="mx-auto max-w-4xl px-4 sm:px-6">
        <div className="text-center mb-14">
          <p className="text-sm font-semibold text-blue-400 uppercase tracking-widest mb-3">Setup</p>
          <h2 id="hiw-heading" className="text-3xl sm:text-4xl font-bold">How It Works</h2>
          <p className="text-gray-400 mt-3">From zero to full visibility in under a minute. Works with Claude Code, OpenAI Codex, and more.</p>
        </div>

        <ol className="space-y-10">
          <li className="grid sm:grid-cols-[64px_1fr] gap-5 items-start">
            <div aria-hidden="true" className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl border border-white/10 bg-white/5 text-sm font-bold text-blue-400">
              01
            </div>
            <div>
              <h3 className="text-lg font-bold mb-1">Point your AI tools to TokenGate</h3>
              <p className="text-gray-400 mb-4">A couple of config changes. That&apos;s it. No code changes, no SDK.</p>
              <div
                className="rounded-xl border border-white/10 bg-black overflow-hidden"
                aria-label="Configuration to connect to TokenGate"
              >
                <div className="flex border-b border-white/10">
                  <button
                    className={`px-4 py-2 text-sm font-medium transition-colors ${tab === 'claude' ? 'text-blue-400 bg-white/5 border-b-2 border-blue-400' : 'text-gray-500 hover:text-gray-300'}`}
                    onClick={() => setTab('claude')}
                  >
                    Claude Code
                  </button>
                  <button
                    className={`px-4 py-2 text-sm font-medium transition-colors ${tab === 'codex' ? 'text-blue-400 bg-white/5 border-b-2 border-blue-400' : 'text-gray-500 hover:text-gray-300'}`}
                    onClick={() => setTab('codex')}
                  >
                    OpenAI Codex
                  </button>
                </div>
                <div className="p-5 font-mono text-sm overflow-x-auto">
                  <div aria-hidden="true" className="flex gap-1.5 mb-4">
                    <span className="h-3 w-3 rounded-full bg-red-500/60" />
                    <span className="h-3 w-3 rounded-full bg-yellow-500/60" />
                    <span className="h-3 w-3 rounded-full bg-green-500/60" />
                  </div>
                  {tab === 'claude' ? (
                    <code className="whitespace-pre">
                      <span className="text-purple-400">export </span>
                      <span className="text-blue-300">ANTHROPIC_BASE_URL</span>
                      <span className="text-gray-500">=</span>
                      <span className="text-green-400">https://gateway.tokengate.to</span>
                      {'\n'}
                      <span className="text-purple-400">export </span>
                      <span className="text-blue-300">ANTHROPIC_API_KEY</span>
                      <span className="text-gray-500">=</span>
                      <span className="text-green-400">tg_xxxxx</span>
                    </code>
                  ) : (
                    <code className="whitespace-pre">
                      <span className="text-gray-500"># ~/.codex/config.toml</span>
                      {'\n'}
                      <span className="text-blue-300">model_provider</span>
                      <span className="text-gray-500"> = </span>
                      <span className="text-green-400">"tokengate"</span>
                      {'\n\n'}
                      <span className="text-gray-500">[</span>
                      <span className="text-blue-300">model_providers.tokengate</span>
                      <span className="text-gray-500">]</span>
                      {'\n'}
                      <span className="text-blue-300">name</span>
                      <span className="text-gray-500"> = </span>
                      <span className="text-green-400">"TokenGate Proxy"</span>
                      {'\n'}
                      <span className="text-blue-300">base_url</span>
                      <span className="text-gray-500"> = </span>
                      <span className="text-green-400">"https://gateway.tokengate.to/v1"</span>
                      {'\n'}
                      <span className="text-blue-300">wire_api</span>
                      <span className="text-gray-500"> = </span>
                      <span className="text-green-400">"responses"</span>
                      {'\n'}
                      <span className="text-blue-300">http_headers</span>
                      <span className="text-gray-500"> = </span>
                      <span className="text-gray-500">{'{'}</span>
                      {'\n'}
                      <span className="text-blue-300">  "X-Tokengate-Key"</span>
                      <span className="text-gray-500"> = </span>
                      <span className="text-green-400">"tg_xxxxx"</span>
                      {'\n'}
                      <span className="text-gray-500">{'}'}</span>
                    </code>
                  )}
                </div>
              </div>
            </div>
          </li>

          <li className="grid sm:grid-cols-[64px_1fr] gap-5 items-start">
            <div aria-hidden="true" className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl border border-white/10 bg-white/5 text-sm font-bold text-blue-400">
              02
            </div>
            <div>
              <h3 className="text-lg font-bold mb-1">Use your AI tools as normal</h3>
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
