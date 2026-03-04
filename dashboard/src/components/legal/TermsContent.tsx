import { Link } from 'react-router-dom'

export default function TermsContent() {
  return (
    <div className="prose prose-gray max-w-none space-y-8 text-gray-700 leading-relaxed text-[15px]">
      <p>
        These Terms of Service ("Terms") are a legally binding agreement between you ("you",
        "Customer") and TokenGate ("TokenGate", "we", "us", "our") governing your access to and use
        of the TokenGate website, dashboard, APIs, and related services (collectively, the
        "Service").
      </p>
      <p>If you do not agree to these Terms, you may not use the Service.</p>

      {/* 1 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">1. Agreement to Terms</h2>
        <p>
          By accessing or using the Service, you acknowledge that you have read, understood, and
          agree to be bound by these Terms and any policies referenced herein, including our{' '}
          <Link to="/privacy" className="text-blue-600 hover:text-blue-800 underline">
            Privacy Policy
          </Link>.
        </p>
        <p className="mt-2">
          We may update these Terms from time to time. We will update the "Last updated" date. Your
          continued use of the Service after changes become effective constitutes acceptance of the
          updated Terms.
        </p>
      </section>

      {/* 2 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">2. Eligibility</h2>
        <p>
          You must be at least 18 years old and have the legal capacity to enter into these Terms.
          If you use the Service on behalf of an entity, you represent that you have authority to
          bind that entity to these Terms.
        </p>
      </section>

      {/* 3 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">3. The Service (What TokenGate Does)</h2>
        <p>
          TokenGate is an infrastructure gateway and AI governance layer that provides visibility,
          budget enforcement, rate limiting, audit logging, and cost tracking for teams using
          third-party AI providers. The Service acts as a proxy between your clients/apps and
          third-party AI provider APIs (including but not limited to Anthropic and OpenAI).
        </p>
        <p className="mt-2">Core capabilities may include:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Request routing and enforcement (budgets, rate limits, allowlists)</li>
          <li>Per-request token and cost tracking</li>
          <li>Role-based access control and multi-tenant workspace isolation</li>
          <li>Audit logs and reporting</li>
          <li>Provider key management (including encryption at rest)</li>
        </ul>
        <p className="mt-2">
          <strong>Important:</strong> TokenGate does not generate AI responses. All model outputs
          (including text, code, images, and other content) are produced entirely by third-party AI
          providers. TokenGate does not operate or control third-party AI providers, their models,
          their pricing, or their service availability.
        </p>
      </section>

      {/* 4 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">4. Account Registration and Security</h2>
        <p>
          You may be required to create an account (e.g., via our authentication provider such as
          Clerk). You agree to provide accurate, current, and complete information and to keep it
          updated.
        </p>
        <p className="mt-2">You are responsible for:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Maintaining the confidentiality of your credentials</li>
          <li>All activity occurring under your account</li>
          <li>Any actions taken using TokenGate API keys issued to you or your workspace</li>
        </ul>
        <p className="mt-2">
          You must promptly notify us of any suspected unauthorized access or security breach.
        </p>
      </section>

      {/* 5 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">5. Subscription Plans, Fees, and Payment</h2>
        <p>
          TokenGate may offer Free, Pro, Team, and Business plans. Plan features, limits, and
          pricing are described on our website or within the dashboard and may change over time.
        </p>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">5.1 Payment Processor</h3>
        <p>
          Paid subscriptions are billed through Stripe. By subscribing, you authorize TokenGate
          (through Stripe) to charge your selected payment method on a recurring basis until you
          cancel or your subscription ends.
        </p>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">5.2 Taxes</h3>
        <p>
          Prices are listed in U.S. dollars unless otherwise stated. We may collect applicable taxes
          (e.g., sales tax/VAT) when required.
        </p>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">5.3 Upgrades, Downgrades, and Proration</h3>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>
            Upgrades may take effect immediately and may be prorated depending on plan configuration
            and Stripe settings.
          </li>
          <li>
            Downgrades generally take effect at the end of the current billing period unless we
            explicitly state otherwise in the dashboard.
          </li>
          <li>
            If a downgrade reduces your allowed quotas (e.g., number of API keys, limits, projects),
            you may be required to remove or disable resources above the new plan limits before the
            downgrade can complete.
          </li>
        </ul>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">5.4 Cancellation</h3>
        <p>
          You may cancel a paid subscription at any time through your account settings (or by
          contacting us). Unless required by law, we do not provide refunds for partial billing
          periods.
        </p>
      </section>

      {/* 6 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">6. Acceptable Use</h2>
        <p>You agree not to use the Service to:</p>
        <ol className="list-decimal pl-6 mt-2 space-y-1">
          <li>Violate any law, regulation, or third-party rights</li>
          <li>Circumvent, disable, or interfere with security or access controls</li>
          <li>Probe, scan, or test the vulnerability of the Service without authorization</li>
          <li>
            Reverse engineer, decompile, or attempt to extract source code except where prohibited
            by law
          </li>
          <li>
            Introduce malware, abuse traffic, spam, or automated requests that degrade service
            integrity
          </li>
          <li>
            Evade rate limits, budget enforcement, model/project allowlists, or billing controls
          </li>
          <li>
            Share or sell access credentials, TokenGate API keys, or provider keys to unauthorized
            parties
          </li>
          <li>
            Use the Service to compete with TokenGate (including building a substantially similar
            product) using non-public information
          </li>
          <li>
            Harvest or scrape the dashboard or APIs in a manner that imposes unreasonable load
          </li>
        </ol>
        <p className="mt-2">
          We may suspend or terminate access if we believe you are violating these Terms.
        </p>
      </section>

      {/* 7 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">7. API Keys and Provider Keys (BYOK)</h2>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">7.1 TokenGate API Keys</h3>
        <p>
          TokenGate API keys may be hashed and/or stored using secure mechanisms. You are responsible
          for safeguarding any API keys issued to you or your workspace.
        </p>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">7.2 Provider Keys (Bring Your Own Key)</h3>
        <p>
          You may store third-party provider API keys in TokenGate. Provider keys may be encrypted at
          rest (e.g., AES-256-GCM envelope encryption) and decrypted transiently during request
          processing.
        </p>
        <p className="mt-2">You are solely responsible for:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>The security and permitted use of your provider keys</li>
          <li>Any costs incurred with third-party providers through those keys</li>
          <li>Complying with provider terms, rate limits, and usage policies</li>
        </ul>
        <p className="mt-2">
          TokenGate is not responsible for third-party provider charges, pricing changes, quota caps,
          or provider-side suspensions.
        </p>
      </section>

      {/* 8 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">8. Data, Logging, and Privacy</h2>
        <p>
          TokenGate may process and route your requests to third-party AI providers and may log usage
          metadata such as:
        </p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Token counts, model identifiers, estimated and/or calculated costs</li>
          <li>Latency, timestamps, request identifiers</li>
          <li>Workspace/project/api-key attribution and enforcement outcomes</li>
        </ul>
        <p className="mt-2">
          <strong>Cost estimates:</strong> Cost calculations displayed in the Service are estimates
          based on publicly available provider pricing at the time of the request. Actual charges
          from AI providers may differ due to pricing changes, rounding, promotional rates, or
          provider-specific billing practices. TokenGate does not guarantee the accuracy of cost
          estimates and is not responsible for discrepancies between estimated and actual provider
          charges.
        </p>
        <p className="mt-2">
          <strong>Content storage:</strong> By default, TokenGate does not store the full content of
          prompts or responses unless you explicitly enable a feature that requires it (if offered).
          Any content-handling behavior is described in our{' '}
          <Link to="/privacy" className="text-blue-600 hover:text-blue-800 underline">
            Privacy Policy
          </Link>{' '}
          and product settings.
        </p>
        <p className="mt-2">
          You retain ownership of your data. We do not claim ownership over your prompts, responses,
          or business data.
        </p>
        <p className="mt-2">
          For details on how we collect and process data, see our{' '}
          <Link to="/privacy" className="text-blue-600 hover:text-blue-800 underline">
            Privacy Policy
          </Link>.
        </p>
      </section>

      {/* 9 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">9. Third-Party Services</h2>
        <p>
          The Service may integrate with third-party services (e.g., Clerk, Stripe, Anthropic,
          OpenAI). Your use of third-party services is subject to their respective terms and privacy
          policies. TokenGate does not control and is not responsible for third-party services'
          availability, security, or practices.
        </p>
      </section>

      {/* 10 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">10. Intellectual Property</h2>
        <p>
          The Service (including software, design, text, and logos) is owned by TokenGate or its
          licensors and is protected by intellectual property laws. Subject to these Terms, TokenGate
          grants you a limited, non-exclusive, non-transferable license to access and use the Service
          for your internal business purposes.
        </p>
        <p className="mt-2">
          You may not copy, modify, distribute, sell, or lease any part of the Service except as
          expressly permitted by these Terms.
        </p>
      </section>

      {/* 11 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">11. Feedback</h2>
        <p>
          If you provide feedback, ideas, or suggestions, you grant TokenGate a perpetual, worldwide,
          royalty-free right to use them without restriction or compensation.
        </p>
      </section>

      {/* 12 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">12. Service Availability; Changes</h2>
        <p>
          We strive to maintain high availability, but the Service is provided on an "as available"
          basis. We may modify, suspend, or discontinue parts of the Service at any time. We are not
          liable for interruptions or downtime.
        </p>
      </section>

      {/* 13 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">13. Termination</h2>
        <p>
          We may suspend or terminate your access to the Service at any time with or without notice
          if we believe you violated these Terms or pose a risk to the Service.
        </p>
        <p className="mt-2">Upon termination:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Your right to use the Service stops immediately</li>
          <li>
            Some provisions (e.g., fees owed, disclaimers, limitation of liability, indemnity,
            dispute resolution) survive termination
          </li>
        </ul>
      </section>

      {/* 14 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">14. Disclaimer of Warranties</h2>
        <p>
          TO THE MAXIMUM EXTENT PERMITTED BY LAW, THE SERVICE IS PROVIDED "AS IS" AND "AS
          AVAILABLE," WITHOUT WARRANTIES OF ANY KIND, WHETHER EXPRESS, IMPLIED, OR STATUTORY,
          INCLUDING IMPLIED WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, AND
          NON-INFRINGEMENT.
        </p>
        <p className="mt-2">
          We do not warrant that the Service will be uninterrupted, secure, error-free, or that it
          will prevent all budget overruns or policy violations in all cases.
        </p>
      </section>

      {/* 15 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">15. Limitation of Liability</h2>
        <p>
          TO THE MAXIMUM EXTENT PERMITTED BY LAW, TOKENGATE AND ITS OFFICERS, DIRECTORS, EMPLOYEES,
          AND AGENTS WILL NOT BE LIABLE FOR ANY INDIRECT, INCIDENTAL, SPECIAL, CONSEQUENTIAL,
          EXEMPLARY, OR PUNITIVE DAMAGES, INCLUDING LOST PROFITS, LOST REVENUE, LOSS OF DATA, OR
          GOODWILL, ARISING OUT OF OR RELATED TO YOUR USE OF THE SERVICE.
        </p>
        <p className="mt-2">
          OUR TOTAL LIABILITY FOR ALL CLAIMS ARISING OUT OF OR RELATED TO THE SERVICE OR THESE TERMS
          WILL NOT EXCEED THE AMOUNT YOU PAID TO TOKENGATE IN THE TWELVE (12) MONTHS BEFORE THE
          EVENT GIVING RISE TO THE CLAIM.
        </p>
        <p className="mt-2">
          Some jurisdictions do not allow certain limitations; in those cases, these limitations
          apply to the fullest extent permitted by law.
        </p>
      </section>

      {/* 16 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">16. Indemnification</h2>
        <p>
          You agree to defend, indemnify, and hold harmless TokenGate and its affiliates, officers,
          directors, employees, and agents from and against any claims, damages, liabilities, losses,
          and expenses (including reasonable attorneys' fees) arising out of or related to:
        </p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>Your use of the Service</li>
          <li>Your violation of these Terms</li>
          <li>Your violation of third-party rights or laws</li>
          <li>
            Your provider key usage and third-party provider charges incurred through your keys
          </li>
        </ul>
      </section>

      {/* 17 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">17. DMCA / Copyright Complaints</h2>
        <p>
          If you believe content available through the Service infringes your copyright, please
          contact us with sufficient detail so we can investigate and respond appropriately.
        </p>
      </section>

      {/* 18 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">18. Electronic Communications</h2>
        <p>
          By using the Service, you consent to receive communications electronically (e.g., email,
          dashboard notices). You agree that electronic communications satisfy any legal requirement
          for written notice.
        </p>
      </section>

      {/* 19 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">19. Governing Law; Venue</h2>
        <p>
          These Terms are governed by the laws of the State of California, without regard to conflict
          of laws principles.
        </p>
        <p className="mt-2">
          Any dispute arising out of or relating to these Terms or the Service will be brought in the
          state or federal courts located in San Francisco County, California, and you consent to
          personal jurisdiction and venue in those courts.
        </p>
      </section>

      {/* 20 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">20. Dispute Resolution; Arbitration; Class Action Waiver</h2>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">
          20.1 Informal Resolution (30-Day Good-Faith Negotiation)
        </h3>
        <p>
          To expedite resolution and reduce the cost of any dispute, controversy, or claim arising
          out of or relating to these Terms or the Service (each, a "Dispute"), you and TokenGate
          agree to first attempt to resolve the Dispute informally and in good faith for at least
          thirty (30) days before initiating arbitration or litigation.
        </p>
        <p className="mt-2">
          Informal negotiations begin upon written notice from one party to the other describing the
          nature of the Dispute and the relief sought. You agree to send such notice to{' '}
          <a href="mailto:tokengate.to@gmail.com" className="text-blue-600 hover:text-blue-800 underline">
            tokengate.to@gmail.com
          </a>
          , and we will respond using the contact information associated with your account.
        </p>
        <p className="mt-2">
          If the Dispute is not resolved within thirty (30) days after notice is sent, either party
          may proceed to arbitration as set forth below.
        </p>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">20.2 Binding Arbitration</h3>
        <p>
          Except for the excluded disputes described in Section 20.5, all Disputes shall be finally
          and exclusively resolved by binding arbitration, rather than in court.
        </p>
        <p className="mt-2">
          You understand and agree that by accepting these Terms, you and TokenGate are each waiving
          the right to a trial by jury or to participate in a class action.
        </p>
        <p className="mt-2">
          The arbitration shall be administered by the American Arbitration Association ("AAA") and
          conducted under the AAA Commercial Arbitration Rules, or, if applicable, the AAA Consumer
          Arbitration Rules, as in effect at the time arbitration is initiated. The AAA rules are
          available at{' '}
          <a
            href="https://www.adr.org"
            target="_blank"
            rel="noopener noreferrer"
            className="text-blue-600 hover:text-blue-800 underline"
          >
            https://www.adr.org
          </a>.
        </p>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">
          20.3 Arbitration Procedures and Location
        </h3>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>
            Arbitration may be conducted in person, by submission of documents, by telephone, or by
            video conference, at the arbitrator's discretion.
          </li>
          <li>
            Unless otherwise required by law or AAA rules, the arbitration shall take place in San
            Francisco, California, or another mutually agreed location.
          </li>
          <li>
            The arbitrator shall issue a written decision stating the essential findings and
            conclusions on which the award is based.
          </li>
          <li>
            The arbitrator must follow applicable law, and the award may be challenged if the
            arbitrator fails to do so.
          </li>
        </ul>
        <p className="mt-2">
          Judgment on the arbitration award may be entered in any court having jurisdiction.
        </p>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">20.4 Fees and Costs</h3>
        <p>
          Each party shall bear its own attorneys' fees and costs, unless the arbitrator determines
          otherwise in accordance with applicable law or AAA rules.
        </p>
        <p className="mt-2">
          AAA filing, administrative, and arbitrator fees shall be allocated according to AAA rules,
          subject to any limits imposed by applicable law.
        </p>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">
          20.5 Exceptions to Arbitration
        </h3>
        <p>The following Disputes are not subject to the above arbitration requirements:</p>
        <ol className="list-decimal pl-6 mt-2 space-y-1">
          <li>
            Claims seeking to enforce or protect a party's intellectual property rights (including
            trademarks, copyrights, trade secrets)
          </li>
          <li>
            Claims related to unauthorized access, misuse, or abuse of the Service, including
            security breaches or credential misuse
          </li>
          <li>Claims seeking injunctive or equitable relief to prevent imminent harm</li>
          <li>Claims that cannot be arbitrated under applicable law</li>
        </ol>
        <p className="mt-2">
          For these excluded matters, either party may bring claims in the courts specified in
          Section 19 (Governing Law; Venue).
        </p>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">20.6 Class Action Waiver</h3>
        <p>To the fullest extent permitted by law:</p>
        <ul className="list-disc pl-6 mt-2 space-y-1">
          <li>
            All Disputes must be brought on an individual basis only, and not as a plaintiff or class
            member in any purported class, collective, consolidated, or representative action.
          </li>
          <li>
            The arbitrator may not consolidate claims of more than one person or preside over any
            form of representative or class proceeding.
          </li>
        </ul>
        <p className="mt-2">
          If this class action waiver is found to be unenforceable, then the entirety of this
          arbitration provision shall be null and void, and the Dispute shall be resolved in court.
        </p>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">
          20.7 Time Limitation on Claims
        </h3>
        <p>
          Any Dispute must be initiated within one (1) year after the cause of action arises. Claims
          not brought within this period are permanently barred, to the fullest extent permitted by
          law.
        </p>

        <h3 className="text-lg font-medium text-gray-900 mt-4 mb-2">20.8 Survival</h3>
        <p>
          This Dispute Resolution section shall survive termination of your account or these Terms.
        </p>
      </section>

      {/* 21 */}
      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-3">21. Contact</h2>
        <p>
          Questions about these Terms:{' '}
          <a href="mailto:tokengate.to@gmail.com" className="text-blue-600 hover:text-blue-800 underline">
            tokengate.to@gmail.com
          </a>
        </p>
      </section>
    </div>
  )
}
