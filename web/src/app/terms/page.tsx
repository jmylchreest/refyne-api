import Link from 'next/link';
import { SiteHeader } from '@/components/site-header';
import { RefyneText } from '@/components/refyne-logo';

export default function TermsPage() {
  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950">
      <SiteHeader />

      <main className="container mx-auto px-4 py-16 max-w-3xl">
        <h1 className="text-3xl font-bold mb-8">Terms of Service</h1>

        <div className="prose prose-zinc dark:prose-invert max-w-none space-y-6 text-sm">
          <p className="text-zinc-500">Last updated: January 2025</p>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">1. Introduction</h2>
            <p>
              These Terms of Service (&quot;Terms&quot;) govern your use of the <RefyneText /> web extraction
              API service (&quot;Service&quot;) operated by <RefyneText /> (&quot;we&quot;, &quot;us&quot;, &quot;our&quot;).
            </p>
            <p>
              By accessing or using our Service, you agree to be bound by these Terms. If you do not agree
              to these Terms, you may not use the Service.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">2. Description of Service</h2>
            <p>
              <RefyneText /> provides an API service that extracts structured data from web pages using
              artificial intelligence. The Service allows you to define schemas and extract data matching
              those schemas from publicly accessible URLs.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">3. Account Registration</h2>
            <p>
              To use the Service, you must create an account. You agree to:
            </p>
            <ul className="list-disc pl-6 space-y-1">
              <li>Provide accurate and complete information</li>
              <li>Maintain the security of your account credentials</li>
              <li>Accept responsibility for all activity under your account</li>
              <li>Notify us immediately of any unauthorised access</li>
            </ul>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">4. Acceptable Use</h2>
            <p>You agree NOT to use the Service to:</p>
            <ul className="list-disc pl-6 space-y-1">
              <li>Extract data from websites that prohibit scraping in their terms of service</li>
              <li>Violate any applicable laws or regulations</li>
              <li>Infringe upon intellectual property rights</li>
              <li>Extract personal data without a lawful basis</li>
              <li>Engage in any activity that disrupts or interferes with the Service</li>
              <li>Attempt to circumvent rate limits or usage restrictions</li>
              <li>Access content behind authentication without authorisation</li>
              <li>Extract content that is illegal, harmful, or offensive</li>
            </ul>
            <p className="mt-4">
              You are solely responsible for ensuring your use of the Service complies with the terms of
              service of any websites you extract data from.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">5. API Keys and BYOK</h2>
            <p>
              The Service allows you to use your own API keys (&quot;Bring Your Own Key&quot; or BYOK) for
              LLM providers. When using BYOK:
            </p>
            <ul className="list-disc pl-6 space-y-1">
              <li>You are responsible for any costs incurred with the LLM provider</li>
              <li>You must comply with the LLM provider&apos;s terms of service</li>
              <li>We encrypt and securely store your API keys but are not liable for any misuse</li>
            </ul>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">6. Pricing and Payment</h2>
            <p>
              The Service offers both free and paid tiers. For paid services:
            </p>
            <ul className="list-disc pl-6 space-y-1">
              <li>Prices are as displayed on our website at the time of purchase</li>
              <li>Payments are processed securely via Stripe</li>
              <li>Credits are non-refundable unless required by law</li>
              <li>We reserve the right to change pricing with reasonable notice</li>
            </ul>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">7. Intellectual Property</h2>
            <p>
              <strong>Your Data:</strong> You retain all rights to the schemas you create and the data
              you extract using our Service.
            </p>
            <p className="mt-2">
              <strong>Our Service:</strong> The Service, including its software, design, and documentation,
              is owned by us and protected by intellectual property laws. The core extraction engine is
              open source and available under its respective licence.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">8. Service Availability</h2>
            <p>
              We strive to provide reliable service but do not guarantee uninterrupted availability.
              The Service is provided &quot;as is&quot; and we may:
            </p>
            <ul className="list-disc pl-6 space-y-1">
              <li>Perform maintenance with or without notice</li>
              <li>Modify or discontinue features</li>
              <li>Experience downtime due to factors beyond our control</li>
            </ul>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">9. Limitation of Liability</h2>
            <p>
              To the maximum extent permitted by law:
            </p>
            <ul className="list-disc pl-6 space-y-1">
              <li>The Service is provided &quot;as is&quot; without warranties of any kind</li>
              <li>We are not liable for any indirect, incidental, or consequential damages</li>
              <li>Our total liability shall not exceed the amount you paid us in the 12 months preceding the claim</li>
              <li>We are not responsible for the accuracy of extracted data</li>
            </ul>
            <p className="mt-4">
              Nothing in these Terms excludes or limits liability for death or personal injury caused by
              negligence, fraud, or any other liability that cannot be excluded by law.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">10. Termination</h2>
            <p>
              We may suspend or terminate your account if you:
            </p>
            <ul className="list-disc pl-6 space-y-1">
              <li>Violate these Terms</li>
              <li>Engage in fraudulent or illegal activity</li>
              <li>Fail to pay for services</li>
            </ul>
            <p className="mt-4">
              You may terminate your account at any time by contacting us. Upon termination, your right
              to use the Service ceases immediately.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">11. Changes to Terms</h2>
            <p>
              We may modify these Terms at any time. We will notify you of material changes by posting
              the updated Terms on our website. Your continued use of the Service after changes constitutes
              acceptance of the new Terms.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">12. Governing Law</h2>
            <p>
              These Terms are governed by the laws of England and Wales. Any disputes shall be subject
              to the exclusive jurisdiction of the courts of England and Wales.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold mt-8 mb-4">13. Contact</h2>
            <p>
              If you have any questions about these Terms, please contact us at:
            </p>
            <p className="mt-2">
              Email: legal@refyne.uk
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
