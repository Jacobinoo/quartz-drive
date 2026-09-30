'use client'

import { useEffect } from "react"

import posthog from 'posthog-js'
import { PostHogProvider as PHProvider } from '@posthog/react'
import {config} from "@/config/env";

export function PostHogProvider({ children }: { children: React.ReactNode }) {

    if (process.env.NODE_ENV !== "production") return children;

    useEffect(() => {
        posthog.init(config.posthogToken as string, {
            cookieless_mode: "always",
            api_host: config.posthogHost,
            defaults: '2026-05-30'
        })
    }, [])

    return (
        <PHProvider client={posthog}>
            {children}
        </PHProvider>
    )
}