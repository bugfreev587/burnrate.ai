import { Link } from 'react-router-dom'

const links = {
  Product: [
    { label: 'Features', href: '#features', internal: false },
    { label: 'Pricing', href: '#pricing', internal: false },
    { label: 'Dashboard', href: '/dashboard', internal: true },
  ],
  Docs: [
    { label: 'Quick Start', href: '#how-it-works', internal: false },
    { label: 'FAQ', href: '#faq', internal: false },
    { label: 'GitHub', href: 'https://github.com/bugfreev587/TokenGate.to', internal: false },
    { label: 'Support', href: 'mailto:hello@tokengate.to', internal: false },
  ],
  Legal: [
    { label: 'Privacy Policy', href: '/privacy', internal: true },
    { label: 'Terms of Service', href: '/terms', internal: true },
  ],
}

export default function LandingFooter() {
  return (
    <footer className="border-t border-white/10 bg-[#04070d]">
      <div className="mx-auto max-w-7xl px-4 sm:px-6 py-12">
        <div className="grid sm:grid-cols-4 gap-8 mb-10">
          <div>
            <Link to="/" className="font-bold text-slate-100 text-lg mb-2 block">
              TokenGate
            </Link>
            <p className="text-sm text-slate-400 leading-relaxed">
              The control layer for AI usage. Visibility, guardrails, and cost control for every team.
            </p>
          </div>

          {Object.entries(links).map(([group, items]) => (
            <div key={group}>
              <p className="text-xs font-semibold text-slate-500 uppercase tracking-widest mb-3">{group}</p>
              <ul className="space-y-2">
                {items.map((item) => (
                  <li key={item.label}>
                    {item.internal ? (
                      <Link to={item.href} className="text-sm text-slate-400 hover:text-slate-100 transition-colors">
                        {item.label}
                      </Link>
                    ) : (
                      <a href={item.href} className="text-sm text-slate-400 hover:text-slate-100 transition-colors">
                        {item.label}
                      </a>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        <div className="flex flex-col sm:flex-row items-center justify-between gap-4 pt-8 border-t border-white/10">
          <p className="text-sm text-slate-500">
            &copy; {new Date().getFullYear()} TokenGate. All rights reserved.
          </p>
          <div className="flex items-center gap-4">
            <a
              href="https://github.com/bugfreev587/TokenGate.to"
              target="_blank"
              rel="noopener noreferrer"
              className="text-slate-500 hover:text-slate-300 transition-colors"
              aria-label="GitHub"
            >
              <svg className="h-5 w-5" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                <path fillRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0 1 12 6.844a9.59 9.59 0 0 1 2.504.337c1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.02 10.02 0 0 0 22 12.017C22 6.484 17.522 2 12 2Z" clipRule="evenodd" />
              </svg>
            </a>
            <a
              href="mailto:hello@tokengate.to"
              className="text-sm text-slate-500 hover:text-slate-300 transition-colors"
            >
              hello@tokengate.to
            </a>
          </div>
        </div>
      </div>
    </footer>
  )
}
