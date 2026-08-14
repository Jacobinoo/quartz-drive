import type { NextConfig } from "next";
import "./src/config/env.ts";

const nextConfig: NextConfig = {
    /* config options here */
    experimental: {
        sri: {
            algorithm: 'sha384'
        }
    }
};

export default nextConfig;
