import Link from 'next/link';
import { SiteHeader } from '@/components/site-header';
import { RefyneText } from '@/components/refyne-logo';

export default function PrivacyPage() {
  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950">
      <SiteHeader />

      <main className="container mx-auto px-4 py-16 max-w-3xl">
        <h1 className="text-3xl font-bold mb-8">Privacy Policy</h1>

        <div className="prose prose-zinc dark:prose-invert max-w-none space-y-6 text-sm">
          <p className="text-zinc-500">Last updated: January 2025</p>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">1. Introduction</h2>
            <p>
              <RefyneText /> (&quot;we&quot;, &quot;us&quot;, &quot;our&quot;) is committed to protecting your privacy.
              This Privacy Policy explains how we collect, use, and safeguard your information when you use our
              web extraction API service.
            </p>
            <p>
              We are based in the United Kingdom and comply with the UK General Data Protection Regulation
              (UK GDPR) and the Data Protection Act 2018.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">2. Information We Collect</h2>

            <h3 className="text-lg font-medium mt-6 mb-2">Account Information</h3>
            <p>When you create an account, we collect:</p>
            <ul className="list-disc pl-6 space-y-1">
              <li>Email address</li>
              <li>Name (if provided)</li>
              <li>Authentication data (managed by our auth provider, Clerk)</li>
            </ul>

            <h3 className="text-lg font-medium mt-6 mb-2">Usage Data</h3>
            <p>When you use our service, we collect:</p>
            <ul className="list-disc pl-6 space-y-1">
              <li>URLs you submit for extraction</li>
              <li>Schemas you create or use</li>
              <li>API call logs and timestamps</li>
              <li>Token usage and billing information</li>
            </ul>

            <h3 className="text-lg font-medium mt-6 mb-2">Technical Data</h3>
            <ul className="list-disc pl-6 space-y-1">
              <li>IP address</li>
              <li>Browser type and version</li>
              <li>Device information</li>
            </ul>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">3. How We Use Your Information</h2>
            <p>We use your information to:</p>
            <ul className="list-disc pl-6 space-y-1">
              <li>Provide and maintain our service</li>
              <li>Process your API requests</li>
              <li>Calculate billing and usage</li>
              <li>Prevent abuse and enforce rate limits</li>
              <li>Improve our service</li>
              <li>Communicate with you about your account</li>
            </ul>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">4. Data Sharing</h2>
            <p>We share data with the following third parties to provide our service:</p>
            <ul className="list-disc pl-6 space-y-1">
              <li><strong>LLM Providers</strong> (e.g., OpenRouter, Anthropic, OpenAI) - We send webpage content to these providers for data extraction. If you use your own API keys (BYOK), requests go directly to your chosen provider.</li>
              <li><strong>Clerk</strong> - Authentication and user management</li>
              <li><strong>Stripe</strong> - Payment processing (we do not store card details)</li>
            </ul>
            <p className="mt-4">
              We do not sell your personal data to third parties.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">5. Data Retention</h2>
            <p>
              We retain your account data for as long as your account is active. Usage logs and extraction
              results are retained for up to 90 days for debugging and billing purposes. You may request
              deletion of your data at any time.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">6. Your Rights</h2>
            <p>Under UK GDPR, you have the right to:</p>
            <ul className="list-disc pl-6 space-y-1">
              <li>Access your personal data</li>
              <li>Rectify inaccurate data</li>
              <li>Request deletion of your data</li>
              <li>Object to processing</li>
              <li>Data portability</li>
              <li>Withdraw consent</li>
            </ul>
            <p className="mt-4">
              To exercise these rights, please contact us at the address below.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">7. Security</h2>
            <p>
              We implement appropriate technical and organisational measures to protect your data, including:
            </p>
            <ul className="list-disc pl-6 space-y-1">
              <li>Encryption of API keys at rest</li>
              <li>HTTPS for all data transmission</li>
              <li>Access controls and authentication</li>
            </ul>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">8. Cookies</h2>
            <p>
              We use essential cookies for authentication and session management. We do not use
              advertising or tracking cookies.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">9. Changes to This Policy</h2>
            <p>
              We may update this Privacy Policy from time to time. We will notify you of any significant
              changes by posting the new policy on this page and updating the &quot;Last updated&quot; date.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">10. Contact Us</h2>
            <p>
              If you have any questions about this Privacy Policy or wish to exercise your rights,
              please contact us at:
            </p>
            <p className="mt-2">
              Email: privacy@refyne.uk
            </p>
          </section>
        </div>

        <div className="mt-12 pt-8 border-t border-zinc-200 dark:border-zinc-800">
          <Link href="/" className="text-sm text-zinc-500 hover:text-zinc-900 dark:hover:text-white transition-colors">
            &larr; Back to home
          </Link>
        </div>
      </main>
    </div>
  );
}
