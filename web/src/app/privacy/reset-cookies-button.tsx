"use client";

import posthog from "posthog-js";

export function ResetCookiesButton() {
    return (
        <button
            onClick={() => {
                posthog.clear_opt_in_out_capturing();
                window.location.reload();
            }}
            className="text-blue-600 dark:text-blue-400 hover:underline font-medium"
        >
            Reset cookie preferences
        </button>
    );
}
