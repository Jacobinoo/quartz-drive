import React from "react";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { ResetCookiesButton } from "./reset-cookies-button";

export default function PrivacyPage() {
  return (
    <div className="min-h-screen bg-white dark:bg-[#0b0b0c] text-gray-900 dark:text-gray-900 dark:text-white/90 font-sans selection:bg-blue-500/30 flex flex-col">
      <div className="w-full max-w-3xl mx-auto px-6 py-20 flex flex-col gap-8 flex-1">
        <Link 
          href="/signin"
          className="inline-flex items-center gap-2 text-sm text-gray-500 dark:text-gray-500 dark:text-white/50 hover:text-gray-900 dark:hover:text-white transition-colors w-fit"
        >
          <ArrowLeft className="w-4 h-4" />
          Back to Home
        </Link>
        
        <div className="flex-1 mt-8 mb-24 prose dark:prose-invert max-w-none">
          <h1 className="text-4xl font-semibold tracking-tight mb-8 border-b border-gray-200 dark:border-white/10 pb-8 mt-4">Privacy Policy</h1>
          
          <h2 className="text-xl font-semibold mt-8 mb-4">1. End-to-End Encryption</h2>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            Quartz Drive is built with zero-knowledge architecture. All of your files, folders, and metadata are end-to-end encrypted on your device before they ever reach our servers. We do not have the ability to read, view, or scan your data. Only you and the people you explicitly share access with can decrypt your files.
            <br /><br />
            This however does not apply to the Live Demo. See <a href="#section-2">Section 2</a> below.
          </p>

          <h2 className="text-xl font-semibold mt-8 mb-4" id="section-2">2. Live Demo</h2>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            As an exception to our zero-knowledge architecture, <strong>Live Demo Accounts</strong> do not offer true end-to-end encryption. To facilitate a seamless demonstration of our software without requiring a full registration, demo sessions utilize a predefined set of encryption keys that are shared among all demo users and are known to our system.
          </p>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            By using a Live Demo Account, you acknowledge that any files, folders, or metadata you upload or create during the session are <strong>not</strong> private and may be technically accessible by our administrators. Please do not upload sensitive, personal, or confidential information to a Live Demo Account. All data associated with demo accounts is automatically and permanently deleted shortly after the session ends.
          </p>

          <h2 className="text-xl font-semibold mt-8 mb-4" id="section-3">3. Data We Collect</h2>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            To provide our services, we only collect the absolute minimum data required. As part of our commitment to security, <strong>we do not store user email addresses in plain text</strong> in our databases. Emails are stored in two secure forms: heavily hashed (for fast, blind lookups) and symmetrically encrypted (for sending notifications). This ensures your identity remains safe even in the highly unlikely event of a database breach.
          </p>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            Because everything else is zero-knowledge encrypted, the <em>only</em> unencrypted data associated with your account that our administrators can access is:
          </p>
          <ul className="list-disc pl-6 text-gray-600 dark:text-white/70 mb-4 space-y-2">
            <li>Your email address (which is decrypted on-the-fly strictly when necessary, such as when sending a login link).</li>
            <li>Your configured display name (which is optional).</li>
            <li>A list of devices that are currently registered and authorized on your account.</li>
            <li>Account metadata, such as the date your account was created.</li>
            <li>The raw storage sizes of your encrypted file chunks (for billing and storage quota management).</li>
            <li>Basic server access logs and IP addresses (for abuse prevention and rate-limiting).</li>
          </ul>

          <h2 className="text-xl font-semibold mt-8 mb-4">4. Bot Protection (Cloudflare Turnstile & hCaptcha)</h2>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            To protect our infrastructure from automated abuse, spam, and bot attacks, we utilize Cloudflare Turnstile and hCaptcha during account creation and authentication.
          </p>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            By using our service, you acknowledge the use of these services. For more details on what data is processed to provide this security, please refer to the <a href="https://www.cloudflare.com/turnstile-privacy-policy/" target="_blank" rel="noopener noreferrer" className="text-blue-600 dark:text-blue-400 hover:underline">Cloudflare Turnstile Privacy Addendum</a> and the <a href="https://www.hcaptcha.com/privacy" target="_blank" rel="noopener noreferrer" className="text-blue-600 dark:text-blue-400 hover:underline">hCaptcha Privacy Policy</a>.
          </p>

          <h2 className="text-xl font-semibold mt-8 mb-4">5. Infrastructure, Analytics & Third-Party Services</h2>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            We rely on secure third-party processors to operate our service. We do not sell or rent your personal information to third parties. These providers include:
          </p>
          <ul className="list-disc pl-6 text-gray-600 dark:text-white/70 mb-4 space-y-2">
            <li><strong>Google Cloud Platform (GCP):</strong> Used to securely host our compute infrastructure and application servers. See the <a href="https://cloud.google.com/terms/cloud-privacy-notice" target="_blank" rel="noopener noreferrer" className="text-blue-600 dark:text-blue-400 hover:underline">Google Cloud Privacy Notice</a>.</li>
            <li><strong>Neon:</strong> Used as our database provider to securely store the hashed and encrypted account data described in <a href="#section-3">Section 3</a>. See the <a href="https://neon.tech/privacy-policy" target="_blank" rel="noopener noreferrer" className="text-blue-600 dark:text-blue-400 hover:underline">Neon Privacy Policy</a>.</li>
            <li><strong>Backblaze:</strong> Used as our secure, S3-compatible storage provider to host your encrypted data blobs. They cannot decrypt or read your files. See the <a href="https://www.backblaze.com/company/policy/privacy" target="_blank" rel="noopener noreferrer" className="text-blue-600 dark:text-blue-400 hover:underline">Backblaze Privacy Policy</a>.</li>
            <li><strong>Cloudflare:</strong> Used for our global CDN, DNS, and DDoS protection. Cloudflare may process network data (like IP addresses) to securely route your traffic. See the <a href="https://www.cloudflare.com/privacypolicy/" target="_blank" rel="noopener noreferrer" className="text-blue-600 dark:text-blue-400 hover:underline">Cloudflare Privacy Policy</a>.</li>
            <li><strong>Stripe:</strong> Used to securely process payments and manage subscriptions. We do not store your full credit card details on our servers. See the <a href="https://stripe.com/privacy" target="_blank" rel="noopener noreferrer" className="text-blue-600 dark:text-blue-400 hover:underline">Stripe Privacy Policy</a>.</li>
            <li><strong>Resend:</strong> Used to deliver transactional emails (like login or password reset links). Your email address is shared with Resend solely for the purpose of delivering these messages. See the <a href="https://resend.com/legal/privacy-policy" target="_blank" rel="noopener noreferrer" className="text-blue-600 dark:text-blue-400 hover:underline">Resend Privacy Policy</a>.</li>
            <li><strong>PostHog:</strong> Used to understand how users interact with our application so we can improve the user experience. PostHog captures basic usage data and analytics. See the <a href="https://posthog.com/privacy" target="_blank" rel="noopener noreferrer" className="text-blue-600 dark:text-blue-400 hover:underline">PostHog Privacy Policy</a>.</li>
          </ul>

          <h2 className="text-xl font-semibold mt-8 mb-4">6. Cookies & Local Storage</h2>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            We use strictly necessary cookies and local storage to provide you with the core functionality of our service. This includes securely storing authentication tokens, managing your session state, and essential security mechanisms (like Cloudflare cookies for DDoS protection). We do not use cookies for advertising, tracking, or marketing purposes. Our analytics provider, PostHog, is configured to operate in a &quot;cookieless&quot; mode, relying on anonymous, ephemeral data rather than persistent tracking cookies.
          </p>

          <h2 className="text-xl font-semibold mt-8 mb-4">7. Security Limitations</h2>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            While we employ industry-standard end-to-end encryption and take reasonable precautions to safeguard your data, no method of transmission over the Internet or method of electronic storage is 100% secure. Therefore, while we strive to use commercially acceptable means to protect your personal information, we cannot guarantee its absolute security.
          </p>
          
          <h2 className="text-xl font-semibold mt-8 mb-4">8. Your Rights (GDPR & Data Protection)</h2>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            If you are a resident of the European Economic Area (EEA) or the United Kingdom, you have certain data protection rights under the General Data Protection Regulation (GDPR) and UK GDPR:
          </p>
          <ul className="list-disc pl-6 text-gray-600 dark:text-white/70 mb-4 space-y-2">
            <li><strong>Right to Access:</strong> You can request a copy of your personal data. Because your files are end-to-end encrypted, we cannot access their contents to provide them to you in a decrypted state.</li>
            <li><strong>Right to Rectification:</strong> You have the right to request that we correct any inaccurate personal data.</li>
            <li><strong>Right to Erasure (&quot;Right to be Forgotten&quot;):</strong> You can delete your account at any time, which permanently deletes your email, cryptographic keys, and encrypted blobs from our servers.</li>
            <li><strong>Right to Data Portability:</strong> You can download your encrypted files at any time via the application interface.</li>
            <li><strong>Right to Restrict Processing & Object:</strong> You can request that we restrict the processing of your personal information, or object to our processing.</li>
          </ul>
          
          <h2 className="text-xl font-semibold mt-8 mb-4">9. Legal Basis for Processing & Data Retention</h2>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            Our legal basis for collecting and using your personal information is to perform our contract with you (providing the Quartz Drive service) and our legitimate interests in maintaining the security and integrity of our platform. We retain your personal data and encrypted files only for as long as your account is active. Once an account is deleted, all associated data is permanently erased from our active systems within 48 hours.
          </p>

          <h2 className="text-xl font-semibold mt-8 mb-4">10. Contact Us</h2>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            If you have any questions, requests regarding your data, or complaints, please contact our Data Protection Officer at <strong><a href="mailto:help@quartzapp.top">help@quartzapp.top</a></strong>.
          </p>

          <h2 className="text-xl font-semibold mt-8 mb-4">11. Manage Cookies</h2>
          <p className="text-gray-600 dark:text-white/70 mb-4 leading-relaxed">
            You can change your mind about optional analytics cookies at any time. Clicking the button below will reset your preference and reload the page so you can make a new choice.
            <br /><br />
            <ResetCookiesButton />
          </p>
        </div>

        <footer className="pt-8 border-t border-gray-200 dark:border-white/10 text-center text-sm text-gray-400 dark:text-white/40">
          &copy; {new Date().getFullYear()} Quartz Drive
        </footer>
      </div>
    </div>
  );
}
