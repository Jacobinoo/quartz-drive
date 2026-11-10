import type { NextConfig } from "next";
import "./src/config/env.ts";

const nextConfig: NextConfig = {
    output: "export",
    images: {
        unoptimized: true,
    },
    poweredByHeader: false,
};

export default nextConfig;
