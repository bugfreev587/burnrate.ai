import { Link } from 'react-router-dom'
import LandingFooter from '../components/landing/LandingFooter'

export default function TermsPage() {
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
        <h1 className="text-3xl font-bold text-gray-900 mb-2">Terms of Service</h1>
        <p className="text-sm text-gray-400 mb-10">Last updated: March 1, 2026</p>

        <div className="prose prose-gray max-w-none space-y-8 text-gray-700 leading-relaxed text-[15px]">

          {/* 1 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">1. Acceptance of Terms</h2>
            <p>
              By accessing or using TokenGate ("Service"), you agree to be bound by these Terms of
              Service ("Terms"). If you do not agree, do not use the Service. We may update these
              Terms from time to time; continued use after changes constitutes acceptance.
            </p>
          </section>

          {/* 2 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">2. Description of Service</h2>
            <p>
              TokenGate is an AI usage control layer that provides visibility, budget enforcement,
              and cost tracking for teams using large-language-model APIs. The Service acts as a
              proxy and reporting layer between your applications and third-party AI providers
              (Anthropic, OpenAI, Google Gemini, AWS Bedrock, Vertex AI, and others).
            </p>
            <p className="mt-2">
              Core capabilities include per-request token and cost tracking, budget limits with
              automatic enforcement, rate limiting, role-based access control, audit logging,
              provider key management with AES-256-GCM envelope encryption, and multi-tenant
              workspace isolation.
            </p>
          </section>

          {/* 3 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">3. Account Registration</h2>
            <p>
              You must create an account via our authentication provider (Clerk) to use the
              Service. You are responsible for maintaining the confidentiality of your account
              credentials and for all activity that occurs under your account. You agree to provide
              accurate, current, and complete information during registration.
            </p>
          </section>

          {/* 4 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">4. Subscriptions and Billing</h2>
            <p>
              TokenGate offers Free, Pro, Team, and Business subscription plans. Paid subscriptions
              are billed through Stripe on a monthly or annual basis. By subscribing to a paid plan,
              you authorize us to charge your payment method on a recurring basis until you cancel.
            </p>
            <p className="mt-2">
              Plan downgrades take effect at the end of your current billing period. You may cancel
              your subscription at any time; access continues until the end of the paid period.
              Refunds are not provided for partial billing periods unless required by applicable law.
            </p>
          </section>

          {/* 5 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">5. Acceptable Use</h2>
            <p>You agree not to:</p>
            <ul className="list-disc pl-6 mt-2 space-y-1">
              <li>Use the Service for any unlawful purpose or in violation of any applicable law.</li>
              <li>Attempt to gain unauthorized access to the Service, other accounts, or related systems.</li>
              <li>Interfere with or disrupt the integrity or performance of the Service.</li>
              <li>Reverse-engineer, decompile, or disassemble any part of the Service.</li>
              <li>Use the Service to transmit malware, spam, or other harmful content through proxied AI requests.</li>
              <li>Exceed rate limits or circumvent budget enforcement mechanisms.</li>
              <li>Share your API keys or provider keys with unauthorized third parties.</li>
            </ul>
          </section>

          {/* 6 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">6. API Keys and Provider Keys</h2>
            <p>
              You may store third-party AI provider API keys within TokenGate. Provider keys are
              encrypted at rest using AES-256-GCM envelope encryption and are only decrypted
              transiently during request processing. You are solely responsible for the security
              and proper use of your provider keys and for any costs incurred with third-party
              providers through those keys.
            </p>
            <p className="mt-2">
              TokenGate API keys are hashed and salted before storage. You are responsible for
              safeguarding your API keys. We are not liable for unauthorized use resulting from
              your failure to protect your credentials.
            </p>
          </section>

          {/* 7 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">7. Data and Content</h2>
            <p>
              TokenGate processes AI API requests on your behalf. We log usage metadata (token
              counts, model identifiers, costs, latency, and timestamps) for tracking and billing
              purposes. We do not store the content of your AI prompts or responses unless
              explicitly required for a feature you have enabled.
            </p>
            <p className="mt-2">
              You retain all rights to your data. We claim no ownership over your content or usage
              data. For details on data collection and processing, please see our{' '}
              <Link to="/privacy" className="text-blue-600 hover:text-blue-800 underline">
                Privacy Policy
              </Link>.
            </p>
          </section>

          {/* 8 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">8. Service Availability</h2>
            <p>
              We strive to maintain high availability but do not guarantee uninterrupted service.
              The Service may be temporarily unavailable due to maintenance, updates, or
              circumstances beyond our control. We are not liable for any loss or damage arising
              from service interruptions.
            </p>
          </section>

          {/* 9 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">9. Third-Party Services</h2>
            <p>
              The Service integrates with third-party services including Clerk (authentication),
              Stripe (payment processing), and various AI providers. Your use of these third-party
              services is subject to their respective terms and privacy policies. We are not
              responsible for the availability, accuracy, or practices of any third-party service.
            </p>
          </section>

          {/* 10 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">10. Limitation of Liability</h2>
            <p>
              TO THE MAXIMUM EXTENT PERMITTED BY LAW, TOKENGATE AND ITS OFFICERS, DIRECTORS,
              EMPLOYEES, AND AGENTS SHALL NOT BE LIABLE FOR ANY INDIRECT, INCIDENTAL, SPECIAL,
              CONSEQUENTIAL, OR PUNITIVE DAMAGES, INCLUDING BUT NOT LIMITED TO LOSS OF PROFITS,
              DATA, OR GOODWILL, ARISING OUT OF OR RELATED TO YOUR USE OF THE SERVICE.
            </p>
            <p className="mt-2">
              OUR TOTAL AGGREGATE LIABILITY FOR ALL CLAIMS ARISING OUT OF OR RELATED TO THESE
              TERMS OR THE SERVICE SHALL NOT EXCEED THE AMOUNT YOU PAID US IN THE TWELVE (12)
              MONTHS PRECEDING THE CLAIM.
            </p>
          </section>

          {/* 11 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">11. Disclaimer of Warranties</h2>
            <p>
              THE SERVICE IS PROVIDED "AS IS" AND "AS AVAILABLE" WITHOUT WARRANTIES OF ANY KIND,
              WHETHER EXPRESS, IMPLIED, OR STATUTORY, INCLUDING BUT NOT LIMITED TO IMPLIED
              WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, AND
              NON-INFRINGEMENT. TOKENGATE DOES NOT WARRANT THAT THE SERVICE WILL BE ERROR-FREE,
              SECURE, OR UNINTERRUPTED.
            </p>
          </section>

          {/* 12 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">12. Indemnification</h2>
            <p>
              You agree to indemnify, defend, and hold harmless TokenGate and its affiliates from
              any claims, damages, losses, liabilities, and expenses (including reasonable
              attorneys' fees) arising out of or related to your use of the Service, your violation
              of these Terms, or your infringement of any rights of a third party.
            </p>
          </section>

          {/* 13 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">13. Termination</h2>
            <p>
              We may suspend or terminate your access to the Service at any time, with or without
              cause, and with or without notice. Upon termination, your right to use the Service
              ceases immediately. Provisions that by their nature should survive termination
              (including limitations of liability, indemnification, and dispute resolution) shall
              survive.
            </p>
          </section>

          {/* 14 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">14. Governing Law</h2>
            <p>
              These Terms shall be governed by and construed in accordance with the laws of the
              State of California, United States, without regard to its conflict-of-law principles.
              Any disputes arising under these Terms shall be resolved in the state or federal
              courts located in San Francisco County, California.
            </p>
          </section>

          {/* 15 */}
          <section>
            <h2 className="text-xl font-semibold text-gray-900 mb-3">15. Contact</h2>
            <p>
              If you have questions about these Terms, please contact us at{' '}
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
