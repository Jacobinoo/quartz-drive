'use client'

import posthog from 'posthog-js'
import { PostHogProvider as PHProvider } from '@posthog/react'
import { config } from "@/config/env";

if (typeof window !== 'undefined') {
    posthog.init(config.posthogToken as string, {
        api_host: config.posthogApiHost,
        ui_host: config.posthogUiHost,

        persistence: 'memory',
        opt_out_capturing_by_default: false,

        autocapture: false,
        disable_session_recording: true,
        enable_heatmaps: false,
        capture_dead_clicks: false,
    })
}

export function PostHogProvider({ children }: { children: React.ReactNode }) {
    return (
        <PHProvider client={posthog}>
            {children}
        </PHProvider>
    )
}