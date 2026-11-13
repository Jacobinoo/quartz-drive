import type { NextConfig } from "next";
import "./src/config/env.ts";

const nextConfig: NextConfig = {
    poweredByHeader: false,
    async headers() {
        return [
            {
                source: "/(.*)",
                headers: [
                    {
                        key: "X-Frame-Options",
                        value: "DENY",
                    },
                    {
                        key: "X-Content-Type-Options",
                        value: "nosniff",
                    },
                    {
                        key: "Referrer-Policy",
                        value: "strict-origin-when-cross-origin",
                    },
                    {
                        key: "Strict-Transport-Security",
                        value: "max-age=31536000; includeSubDomains; preload",
                    },
                    {
                        key: "X-DNS-Prefetch-Control",
                        value: "off",
                    },
                    {
                        key: "X-Permitted-Cross-Domain-Policies",
                        value: "none",
                    },
                    {
                        key: "Cross-Origin-Opener-Policy",
                        value: "same-origin",
                    },
                    {
                        key: "Permissions-Policy",
                        value: "camera=(), microphone=(), geolocation=(), payment=(), usb=(), bluetooth=()",
                    },
                ],
            },
        ];
    },
};

export default nextConfig;
