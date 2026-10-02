'use client'

import posthog from 'posthog-js'
import { PostHogProvider as PHProvider } from '@posthog/react'
import { config } from "@/config/env";

if (typeof window !== 'undefined') {
    posthog.init(config.posthogToken as string, {
        api_host: config.posthogApiHost,
        ui_host: config.posthogUiHost,

        cookieless_mode: "on_reject",
        opt_out_capturing_by_default: true,

        autocapture: false,
        disable_session_recording: true,
        enable_heatmaps: false,
        capture_dead_clicks: false,
        
        capture_exceptions: true,
        before_send: (event) => {
            if (!event) return event;
            if (event.event === '$exception' && event.properties) {
                const redact = (str: string) => {
                    if (typeof str !== 'string') return str;
                    return str
                        // Redact Emails
                        .replace(/[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/g, '[EMAIL]')
                        // Redact Base64 encoded keys (length > 40)
                        .replace(/(?:[A-Za-z0-9+/]{4}){10,}(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?/g, '[BASE64_KEY]')
                        // Redact Hex encoded keys (length >= 32)
                        .replace(/\b[a-fA-F0-9]{32,}\b/g, '[HEX_KEY]')
                        // Redact potential filenames in quotes
                        .replace(/['"]([^'"]+\.[a-zA-Z0-9]{2,4})['"]/g, "'[FILE]'");
                };

                const redactDeep = (obj: any) => {
                    if (!obj || typeof obj !== 'object') return;
                    for (const key in obj) {
                        if (typeof obj[key] === 'string') {
                            obj[key] = redact(obj[key]);
                        } else if (typeof obj[key] === 'object') {
                            redactDeep(obj[key]);
                        }
                    }
                };

                redactDeep(event.properties);
            }
            return event;
        }
    })
}

export function PostHogProvider({ children }: { children: React.ReactNode }) {
    return (
        <PHProvider client={posthog}>
            {children}
        </PHProvider>
    )
}