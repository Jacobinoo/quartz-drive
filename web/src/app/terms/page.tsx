import React from "react";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";

export default function TermsPage() {
  return (
    <div className="min-h-screen bg-white dark:bg-[#0b0b0c] text-gray-900 dark:text-gray-900 dark:text-white/90 font-sans selection:bg-blue-500/30 flex flex-col">
      <div className="w-full max-w-3xl mx-auto px-6 py-20 flex flex-col gap-8 flex-1">
        <a
          href="/"
          className="inline-flex items-center gap-2 text-sm text-gray-500 dark:text-gray-500 dark:text-white/50 hover:text-gray-900 dark:hover:text-white transition-colors w-fit"
        >
          <ArrowLeft className="w-4 h-4" />
          Back to Home
        </a>
        
        <header className="border-b border-gray-200 dark:border-white/10 pb-8 mt-4">
          <h1 className="text-4xl font-semibold tracking-tight">Terms of Service</h1>
          <p className="text-gray-500 dark:text-white/50 mt-4 text-sm uppercase tracking-wider font-medium">Last Updated: September 2026 (Demo)</p>
        </header>

        <main className="flex flex-col gap-8 text-gray-600 dark:text-white/70 leading-relaxed text-[15px]">
          <section>
            <h2 className="text-xl font-semibold text-gray-900 dark:text-white/90 mb-3">1. Introduction</h2>
            <p>
              Welcome to Quartz Drive ("we," "our," or "us"). By accessing or using our end-to-end encrypted storage platform, you agree to be bound by these Terms of Service. If you do not agree to these terms, please do not use our services.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold text-gray-900 dark:text-white/90 mb-3">2. End-to-End Encryption</h2>
            <p className="mb-3">
              Quartz Drive is designed with privacy by default. We employ zero-knowledge, end-to-end encryption (E2EE) using the OPAQUE protocol. This means:
            </p>
            <ul className="list-disc list-inside space-y-2 ml-2">
              <li>We do not have access to your raw passwords.</li>
              <li>We cannot decrypt, view, or recover your files.</li>
              <li>If you lose your password and recovery phrases, your data is permanently inaccessible.</li>
            </ul>
          </section>

          <section>
            <h2 className="text-xl font-semibold text-gray-900 dark:text-white/90 mb-3">3. User Responsibilities</h2>
            <p>
              You are solely responsible for maintaining the confidentiality of your account credentials. You agree not to use Quartz Drive to store or distribute material that is illegal, infringes on intellectual property rights, or violates the privacy of others.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold text-gray-900 dark:text-white/90 mb-3">4. Service Availability</h2>
            <p>
              While we strive for 100% uptime, Quartz Drive is provided on an "as is" and "as available" basis. We reserve the right to modify, suspend, or discontinue the service with or without notice.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold text-gray-900 dark:text-white/90 mb-3">5. Demo Notice</h2>
            <p>
              Please note that this is currently a demonstration environment. Data durability is not guaranteed, and the system may be reset without warning.
            </p>
          </section>
        </main>

        <footer className="mt-12 pt-8 border-t border-gray-200 dark:border-white/10 text-center text-sm text-gray-400 dark:text-white/40">
          &copy; {new Date().getFullYear()} Quartz Drive
        </footer>
      </div>
    </div>
  );
}
