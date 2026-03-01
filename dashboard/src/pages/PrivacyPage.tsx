import { Link } from 'react-router-dom'
import LandingFooter from '../components/landing/LandingFooter'

export default function PrivacyPage() {
  return (
    <div className="min-h-screen flex flex-col bg-white">
      {/* Nav */}
      <nav className="border-b border-gray-100 bg-white">
        <div className="mx-auto max-w-3xl px-4 sm:px-6 py-4 flex items-center justify-between">
          <Link to="/" className="font-bold text-gray-900 text-lg">TokenGate</Link>
          <Link to="/" className="text-sm text-gray-500 hover:text-gray-900 transition-colors">
            &larr; Back to home
          </Link>
        </div>
      </nav>

      {/* Content */}
      <main className="flex-1 mx-auto max-w-3xl px-4 sm:px-6 py-12">
        <h1 className="text-3xl font-bold text-gray-900 mb-2">Privacy Policy</h1>
        <p className="text-sm text-gray-400 mb-10">Last updated: March 1, 2026</p>

        <div className="prose prose-gray max-w-none space-y-8 text-gray-700 leading-relaxed text-[15px]">

          {/* 1 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">1. Introduction</h2>
            <p>
              TokenGate ("we", "us", or "our") operates the TokenGate platform (the "Service").
              This Privacy Policy explains how we collect, use, disclose, and safeguard your
              information when you use our Service. Please read this policy carefully. By using the
              Service, you consent to the practices described herein.
            </p>
          </section>

          {/* 2 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">2. Information We Collect</h2>

            <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">2.1 Account Information</h3>
            <p>
              When you create an account via our authentication provider (Clerk), we receive and
              store your email address, first and last name, and a unique user identifier. We also
              store your organizational role (owner, admin, member, or viewer) and account status.
            </p>

            <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">2.2 Usage Data</h3>
            <p>
              When you use the Service to proxy or report AI API requests, we collect usage
              metadata including: token counts (prompt, completion, cache, and reasoning tokens),
              AI provider and model identifiers, computed costs, request latency, API key
              identifiers, and timestamps. This data powers your dashboards, budget enforcement,
              and cost analytics.
            </p>

            <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">2.3 Provider Keys</h3>
            <p>
              If you store third-party AI provider API keys in TokenGate, those keys are encrypted
              at rest using AES-256-GCM envelope encryption. Keys are only decrypted transiently
              during request processing and are never stored in plaintext.
            </p>

            <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">2.4 Billing Information</h3>
            <p>
              Payment information (credit card numbers, billing addresses) is collected and
              processed directly by Stripe. We store only your Stripe customer ID and subscription
              status — we never have access to your full payment card details.
            </p>

            <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">2.5 Audit Data</h3>
            <p>
              We log administrative actions (API key creation, member invitations, provider key
              changes, etc.) including the acting user, IP address, timestamp, and before/after
              state for compliance and security purposes.
            </p>

            <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">2.6 AI Prompt and Response Content</h3>
            <p>
              We do not store the content of your AI prompts or responses. TokenGate acts as a
              pass-through proxy — request and response payloads are forwarded to the upstream AI
              provider and are not persisted by us.
            </p>
          </section>

          {/* 3 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">3. How We Use Your Information</h2>
            <p>We use the information we collect to:</p>
            <ul className="list-disc pl-6 mt-2 space-y-1">
              <li>Provide, operate, and maintain the Service.</li>
              <li>Track and display your AI usage, costs, and budget status.</li>
              <li>Enforce budget limits and rate limits you have configured.</li>
              <li>Process payments and manage your subscription.</li>
              <li>Provide audit trails and compliance reporting.</li>
              <li>Send service-related notifications (budget alerts, system updates).</li>
              <li>Improve and develop new features for the Service.</li>
              <li>Detect and prevent fraud, abuse, or security incidents.</li>
            </ul>
          </section>

          {/* 4 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">4. Data Sharing and Third Parties</h2>
            <p>We share your information only in the following circumstances:</p>
            <ul className="list-disc pl-6 mt-2 space-y-1">
              <li>
                <strong>Clerk</strong> — Provides authentication services. Receives your email and
                name during sign-up and sign-in.
              </li>
              <li>
                <strong>Stripe</strong> — Processes subscription payments. Receives billing
                information you provide at checkout.
              </li>
              <li>
                <strong>AI Providers</strong> — Your proxied API requests (including provider keys
                and request payloads) are forwarded to the AI provider you have configured
                (Anthropic, OpenAI, Google Gemini, AWS Bedrock, or Vertex AI).
              </li>
              <li>
                <strong>Legal Compliance</strong> — We may disclose information if required by law,
                legal process, or governmental request.
              </li>
            </ul>
            <p className="mt-2">
              We do not sell your personal information to third parties. We do not share usage data
              with advertisers.
            </p>
          </section>

          {/* 5 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">5. Data Security</h2>
            <p>
              We implement industry-standard security measures to protect your data:
            </p>
            <ul className="list-disc pl-6 mt-2 space-y-1">
              <li>Provider keys encrypted with AES-256-GCM envelope encryption at rest.</li>
              <li>TokenGate API keys hashed and salted before storage.</li>
              <li>Role-based access control (RBAC) with organization and project-level permissions.</li>
              <li>Multi-tenant data isolation — each workspace's data is strictly separated.</li>
              <li>Audit logging of all administrative actions.</li>
              <li>Encrypted data transmission via HTTPS.</li>
            </ul>
            <p className="mt-2">
              While we use commercially reasonable measures to protect your data, no method of
              electronic storage or transmission is 100% secure, and we cannot guarantee absolute
              security.
            </p>
          </section>

          {/* 6 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">6. Data Retention</h2>
            <p>
              We retain your data for as long as your account is active or as needed to provide the
              Service. Usage logs and cost ledger records are retained according to your plan's data
              retention period (7 days for Free, 90 days for Pro, 180 days for Team, 1+ year for
              Business). Audit logs and immutable cost records may be retained longer for compliance
              and financial auditing purposes.
            </p>
            <p className="mt-2">
              Upon account deletion, we will remove your personal information within 30 days,
              except where retention is required by law or for legitimate business purposes such as
              fraud prevention.
            </p>
          </section>

          {/* 7 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">7. Your Rights</h2>
            <p>Depending on your jurisdiction, you may have the right to:</p>
            <ul className="list-disc pl-6 mt-2 space-y-1">
              <li>Access the personal data we hold about you.</li>
              <li>Request correction of inaccurate data.</li>
              <li>Request deletion of your personal data.</li>
              <li>Object to or restrict certain processing of your data.</li>
              <li>Export your data in a portable format (available via audit reports and CSV export).</li>
            </ul>
            <p className="mt-2">
              To exercise any of these rights, please contact us at{' '}
              <a href="mailto:tokengate.to@gmail.com" className="text-blue-600 hover:text-blue-800 underline">
                tokengate.to@gmail.com
              </a>.
            </p>
          </section>

          {/* 8 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">8. Cookies and Tracking</h2>
            <p>
              We use essential cookies required for authentication and session management (provided
              by Clerk). We do not use advertising or third-party tracking cookies. We do not use
              analytics trackers that share data with third parties.
            </p>
          </section>

          {/* 9 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">9. Children's Privacy</h2>
            <p>
              The Service is not directed to individuals under the age of 16. We do not knowingly
              collect personal information from children. If we learn that we have collected
              information from a child under 16, we will delete that information promptly.
            </p>
          </section>

          {/* 10 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">10. Changes to This Policy</h2>
            <p>
              We may update this Privacy Policy from time to time. We will notify you of material
              changes by posting the updated policy on this page with a revised "Last updated" date.
              Your continued use of the Service after changes constitutes acceptance of the updated
              policy.
            </p>
          </section>

          {/* 11 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">11. Contact</h2>
            <p>
              If you have questions or concerns about this Privacy Policy, please contact us at{' '}
              <a href="mailto:tokengate.to@gmail.com" className="text-blue-600 hover:text-blue-800 underline">
                tokengate.to@gmail.com
              </a>.
            </p>
          </section>

        </div>
      </main>

      <LandingFooter />
    </div>
  )
}
