import type { NextConfig } from "next";

const isDev = process.env.NODE_ENV === 'development';

const cspHeader =` 
    default-src 'self';
    script-src 'self' ${isDev ? "'unsafe-eval' 'unsafe-inline' blob:" : "'strict-dynamic'"};
    style-src 'self' ${isDev ? "'unsafe-inline'" : ''};
    img-src 'self' blob: data:;
    font-src 'self';
    object-src 'none';
    connect-src ${isDev ? "https://localhost:3000/ https://localhost:3100/ https://localhost:8333/ wss://localhost:3000/ " : "'self'"};
    base-uri 'self';
    form-action 'self';
    frame-ancestors 'none';
    upgrade-insecure-requests;
`

const nextConfig: NextConfig = {
    /* config options here */
    experimental: {
        sri: {
            algorithm: 'sha384'
        }
    },
    headers: async () => {
        return [
            {
                source: '/(.*)',
                headers: [
                    {
                        key: 'Content-Security-Policy',
                        value: cspHeader.replace(/\n/g, '')
                    },
                ]
            },
        ]
    }
};

export default nextConfig;
