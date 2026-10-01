'use client';

import { useEffect, useState } from "react";
import posthog from "posthog-js";
import Link from "next/link";

export function CookieBanner() {
    const [consentGiven, setConsentGiven] = useState<string | null>(null);

    useEffect(() => {
        setConsentGiven(posthog.get_explicit_consent_status());
    }, []);

    const handleAcceptCookies = () => {
        posthog.opt_in_capturing();
        setConsentGiven('granted');
    };

    const handleDeclineCookies = () => {
        posthog.opt_out_capturing();
        setConsentGiven('denied');
    };

    if (consentGiven !== 'pending') return null;

    return (
        <div className="fixed bottom-0 inset-x-0 sm:bottom-6 sm:right-6 sm:inset-x-auto z-50 m-4 sm:m-0 max-w-md bg-white dark:bg-[#121214] border border-gray-200 dark:border-white/10 text-gray-900 dark:text-white p-5 rounded-xl shadow-xl">
            <div className="flex flex-col gap-3">
                <p className="text-sm text-gray-600 dark:text-white/70 leading-relaxed">
                    We use optional analytics cookies to understand how you interact with Quartz Drive and improve performance. No unencrypted file data or file names are ever tracked. See our{" "}
                    <Link href="/privacy" className="text-blue-600 dark:text-blue-400 underline hover:opacity-80">
                        Privacy Policy
                    </Link>.
                </p>
                <div className="flex gap-3 items-center justify-end pt-1">
                    <button 
                        type="button" 
                        onClick={handleDeclineCookies} 
                        className="px-3.5 py-1.5 text-xs font-medium text-gray-700 dark:text-white/80 hover:bg-gray-100 dark:hover:bg-white/10 border border-gray-300 dark:border-white/15 rounded-lg transition-colors"
                    >
                        Decline
                    </button>
                    <button 
                        type="button" 
                        onClick={handleAcceptCookies} 
                        className="px-3.5 py-1.5 text-xs font-medium bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors"
                    >
                        Accept cookies
                    </button>
                </div>
            </div>
        </div>
    );
}