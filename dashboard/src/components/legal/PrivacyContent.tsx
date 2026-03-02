export default function PrivacyContent() {
  return (
    <div className="prose prose-gray max-w-none space-y-8 text-gray-700 leading-relaxed text-[15px]">
      <p>
        This Privacy Policy explains how TokenGate ("TokenGate," "we," "us," or "our") collects,
        uses, discloses, and safeguards information when you access or use the TokenGate platform,
        website, and related services (collectively, the "Service").
      </p>
      <p>
        If you do not agree with the practices described in this Privacy Policy, please do not use
        the Service.
      </p>

      {/* Summary */}
      <section className="bg-gray-50 border border-gray-200 rounded-lg p-5">
        <h2 className="text-xl font-semibold text-gray-900 mb-3">Summary of Key Points</h2>
        <ul className="list-disc pl-6 space-y-1">
          <li>We collect only the data necessary to operate an AI usage governance platform.</li>
          <li>We do not store AI prompt or response content.</li>
          <li>Provider API keys are encrypted at rest and never stored in plaintext.</li>
          <li>Payment information is handled entirely by Stripe.</li>
          <li>We do not sell personal data and do not use advertising trackers.</li>
          <li>The Service is intended for U.S.-based users only at this time.</li>
        </ul>
      </section>

      {/* 1 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">1. Information We Collect</h2>

        <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">
          1.1 Information You Provide to Us
        </h3>
        <p>When you register for or use the Service, we may collect:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Name</li>
          <li>Email address</li>
          <li>Workspace / organization identifiers</li>
          <li>Organizational role (e.g., owner, admin, editor, viewer)</li>
          <li>Account status and preferences</li>
          <li>Communications you send to us (e.g., support emails)</li>
        </ul>
        <p className="mt-2">
          Authentication is provided via Clerk, which acts as our identity provider.
        </p>

        <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">
          1.2 Usage and Technical Data
        </h3>
        <p>
          When you use TokenGate to proxy or report AI API requests, we collect usage metadata only,
          including:
        </p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Token counts (prompt, completion, cache creation, cache read, reasoning)</li>
          <li>AI provider and model identifiers</li>
          <li>Computed cost data</li>
          <li>Request latency and timestamps</li>
          <li>API key identifiers</li>
          <li>Project and workspace identifiers</li>
          <li>IP address and user agent (for security and auditing)</li>
        </ul>
        <p className="mt-2">
          This data is required to provide dashboards, budget enforcement, billing, and audit trails.
        </p>

        <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">
          1.3 AI Prompt and Response Content
        </h3>
        <p className="font-medium">We do not store the content of AI prompts or AI responses.</p>
        <p className="mt-2">
          TokenGate operates as a pass-through control plane. Request and response payloads are
          forwarded to the configured AI provider and are not persisted by TokenGate, except
          transiently in memory for request handling.
        </p>

        <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">1.4 Provider API Keys</h3>
        <p>
          If you store third-party AI provider API keys (e.g., Anthropic, OpenAI):
        </p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Keys are encrypted at rest using AES-256-GCM envelope encryption</li>
          <li>Keys are decrypted only transiently during request execution</li>
          <li>Keys are never logged or stored in plaintext</li>
        </ul>
        <p className="mt-2">
          You remain solely responsible for the usage and cost incurred with third-party providers.
        </p>

        <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">1.5 Billing Information</h3>
        <p>All payment processing is handled by Stripe.</p>
        <p className="mt-2">We store only:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Stripe customer ID</li>
          <li>Subscription plan</li>
          <li>Billing status</li>
        </ul>
        <p className="mt-2">We do not store credit card numbers or payment instrument details.</p>
        <p className="mt-2">
          Stripe's Privacy Policy:{' '}
          <a
            href="https://stripe.com/privacy"
            target="_blank"
            rel="noopener noreferrer"
            className="text-blue-600 hover:text-blue-800 underline"
          >
            https://stripe.com/privacy
          </a>
        </p>

        <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">
          1.6 Audit and Security Logs
        </h3>
        <p>We record administrative and security events, including:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>API key creation or revocation</li>
          <li>Provider key changes</li>
          <li>Membership and role changes</li>
          <li>Budget and limit modifications</li>
          <li>Login activity</li>
        </ul>
        <p className="mt-2">
          Audit logs include timestamps, acting user IDs, IP addresses, and before/after state where
          applicable.
        </p>
      </section>

      {/* 2 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">2. How We Use Your Information</h2>
        <p>We use collected information to:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Provide, operate, and maintain the Service</li>
          <li>Authenticate users and manage accounts</li>
          <li>Track AI usage, costs, and budgets</li>
          <li>Enforce rate limits and spend limits</li>
          <li>Process subscriptions and billing</li>
          <li>Provide audit trails and compliance reporting</li>
          <li>Send service-related notifications</li>
          <li>Improve Service reliability and features</li>
          <li>Detect and prevent fraud, abuse, or security incidents</li>
          <li>Comply with legal obligations</li>
        </ul>
      </section>

      {/* 3 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">
          3. Legal Basis for Processing (U.S. Users)
        </h2>
        <p>We process personal information based on:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Performance of a contract (providing the Service)</li>
          <li>Legitimate business interests (security, billing, analytics)</li>
          <li>Legal compliance obligations</li>
        </ul>
        <p className="mt-2">
          TokenGate does not currently market or target users in the European Union.
        </p>
      </section>

      {/* 4 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">4. Sharing of Information</h2>
        <p>We share information only in the following limited circumstances:</p>

        <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">Service Providers</h3>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li><strong>Clerk</strong> — authentication and user identity</li>
          <li><strong>Stripe</strong> — subscription billing and payments</li>
          <li><strong>Cloud infrastructure providers</strong> — hosting and secure storage</li>
        </ul>

        <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">AI Providers</h3>
        <p>
          Your API requests are forwarded to the AI provider you configure (e.g., Anthropic, OpenAI).
          Their use of data is governed by their respective privacy policies.
        </p>

        <h3 className="text-lg font-medium text-gray-800 mt-4 mb-2">Legal Requirements</h3>
        <p>
          We may disclose information if required by law, subpoena, court order, or governmental
          request.
        </p>

        <p className="mt-4 font-medium">
          We do not sell personal information and do not share data with advertisers.
        </p>
      </section>

      {/* 5 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">
          5. Cookies and Tracking Technologies
        </h2>
        <p>
          We use essential cookies only, primarily for authentication and session management via
          Clerk.
        </p>
        <p className="mt-2">We do not use:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Advertising cookies</li>
          <li>Cross-site tracking</li>
          <li>Behavioral profiling</li>
        </ul>
        <p className="mt-2">
          We do not respond to "Do Not Track" signals, as there is no consistent industry standard.
        </p>
      </section>

      {/* 6 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">6. Data Retention</h2>
        <p>We retain information only as long as necessary to provide the Service:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Usage data: retained based on your subscription plan</li>
          <li>Audit logs and cost ledgers: retained for compliance and financial integrity</li>
          <li>Account data: retained while your account is active</li>
        </ul>
        <p className="mt-2">
          Upon account deletion, personal data is removed within 30 days, unless retention is
          required by law or legitimate business purposes.
        </p>
      </section>

      {/* 7 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">7. Your Rights</h2>
        <p>Depending on your jurisdiction, you may have the right to:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Access your personal information</li>
          <li>Correct inaccurate information</li>
          <li>Request deletion of your personal data</li>
          <li>Export usage and audit data</li>
          <li>Restrict certain processing activities</li>
        </ul>
        <p className="mt-2">
          Requests may be submitted to{' '}
          <a
            href="mailto:tokengate.to@gmail.com"
            className="text-blue-600 hover:text-blue-800 underline"
          >
            tokengate.to@gmail.com
          </a>.
          We may require identity verification before fulfilling requests.
        </p>
      </section>

      {/* 8 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">8. Children's Privacy</h2>
        <p>The Service is not intended for individuals under 16 years of age.</p>
        <p className="mt-2">
          We do not knowingly collect personal information from children. If such data is discovered,
          it will be deleted promptly.
        </p>
      </section>

      {/* 9 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">9. International Data Transfers</h2>
        <p>TokenGate is hosted in the United States.</p>
        <p className="mt-2">
          By using the Service, you acknowledge that your data will be processed and stored in the
          United States.
        </p>
      </section>

      {/* 10 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">10. Security Measures</h2>
        <p>We implement industry-standard safeguards, including:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Encrypted storage of provider credentials</li>
          <li>Hashed and salted API keys</li>
          <li>Role-based access control (RBAC)</li>
          <li>Multi-tenant data isolation</li>
          <li>Encrypted data transmission (HTTPS)</li>
          <li>Comprehensive audit logging</li>
        </ul>
        <p className="mt-2">
          No system is 100% secure, but we continuously improve our security posture.
        </p>
      </section>

      {/* 11 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">
          11. Changes to This Privacy Policy
        </h2>
        <p>We may update this Privacy Policy periodically.</p>
        <p className="mt-2">
          Material changes will be posted on this page with an updated "Last updated" date.
        </p>
        <p className="mt-2">
          Continued use of the Service constitutes acceptance of the updated policy.
        </p>
      </section>

      {/* 12 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">12. Contact Information</h2>
        <p>
          If you have questions or concerns regarding this Privacy Policy, please contact:
        </p>
        <p className="mt-2">
          Email:{' '}
          <a
            href="mailto:tokengate.to@gmail.com"
            className="text-blue-600 hover:text-blue-800 underline"
          >
            tokengate.to@gmail.com
          </a>
        </p>
        <p>
          Website:{' '}
          <a
            href="https://tokengate.to"
            target="_blank"
            rel="noopener noreferrer"
            className="text-blue-600 hover:text-blue-800 underline"
          >
            https://tokengate.to
          </a>
        </p>
      </section>
    </div>
  )
}
