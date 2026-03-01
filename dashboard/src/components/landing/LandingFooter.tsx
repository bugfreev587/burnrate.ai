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
    { label: 'Support', href: 'mailto:hello@tokengate.to', internal: false },
  ],
  Legal: [
    { label: 'Privacy', href: '/privacy', internal: true },
    { label: 'Terms', href: '/terms', internal: true },
  ],
}

export default function LandingFooter() {
  return (
    <footer className="border-t border-gray-100 bg-white">
      <div className="mx-auto max-w-6xl px-4 sm:px-6 py-12">
        <div className="grid sm:grid-cols-4 gap-8 mb-10">
          <div>
            <Link to="/" className="font-bold text-gray-900 text-lg mb-2 block">
              TokenGate
            </Link>
            <p className="text-sm text-gray-500 leading-relaxed">
              The control layer for AI usage. Visibility, guardrails, and cost control for every team.
            </p>
          </div>

          {Object.entries(links).map(([group, items]) => (
            <div key={group}>
              <p className="text-xs font-semibold text-gray-400 uppercase tracking-widest mb-3">{group}</p>
              <ul className="space-y-2">
                {items.map((item) => (
                  <li key={item.label}>
                    {item.internal ? (
                      <Link to={item.href} className="text-sm text-gray-600 hover:text-gray-900 transition-colors">
                        {item.label}
                      </Link>
                    ) : (
                      <a href={item.href} className="text-sm text-gray-600 hover:text-gray-900 transition-colors">
                        {item.label}
                      </a>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        <div className="flex flex-col sm:flex-row items-center justify-between gap-4 pt-8 border-t border-gray-100">
          <p className="text-sm text-gray-400">
            &copy; {new Date().getFullYear()} TokenGate. All rights reserved.
          </p>
          <a
            href="mailto:hello@tokengate.to"
            className="text-sm text-gray-400 hover:text-gray-600 transition-colors"
          >
            hello@tokengate.to
          </a>
        </div>
      </div>
    </footer>
  )
}
