import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

export function proxy(request: NextRequest) {
    if (request.nextUrl.hostname === "quartzapp.top") {
        const url = request.nextUrl.clone();
        url.hostname = "www.quartzapp.top";

        const response = NextResponse.redirect(url, 308);
        // force the preload header directly onto the redirect
        response.headers.set(
            "Strict-Transport-Security",
            "max-age=31536000; includeSubDomains; preload"
        );
        return response;
    }

    const nonce = Buffer.from(crypto.randomUUID()).toString("base64");

    const isDev = process.env.NODE_ENV === "development";
    const s3Endpoint = process.env.NEXT_PUBLIC_S3_ENDPOINT || "";
    const apiOrigin = process.env.NEXT_PUBLIC_API_URL || "";
    const passwordPwnedCheckApi = "https://api.pwnedpasswords.com";

    const devOrigins = [
        "https://localhost:3000",
        process.env.NEXT_PUBLIC_API_URL || "",
        "wss://localhost:3000",
    ];

    const connectSrc = isDev
        ? [
              "'self'",
              ...devOrigins,
              s3Endpoint,
              passwordPwnedCheckApi,
              "https://*.hcaptcha.com",
              "https://challenges.cloudflare.com",
              "https://*.challenges.cloudflare.com",
              "https://eu-assets.i.posthog.com",
              "https://eu.i.posthog.com",
              "https://a.quartzapp.top",
          ].join(" ")
        : [
              "'self'",
              apiOrigin,
              s3Endpoint,
              passwordPwnedCheckApi,
              "https://*.hcaptcha.com",
              "https://challenges.cloudflare.com",
              "https://*.challenges.cloudflare.com",
              "https://eu-assets.i.posthog.com",
              "https://eu.i.posthog.com",
              "https://a.quartzapp.top",
          ].join(" ");

    const reportUri =
        process.env.NEXT_PUBLIC_CSP_REPORT_URI || "/api/csp-report";

    const cspHeader = `
    default-src 'self';
    script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' blob: https://js.hcaptcha.com https://*.hcaptcha.com https://challenges.cloudflare.com https://*.challenges.cloudflare.com https://eu-assets.i.posthog.com https://a.quartzapp.top ${isDev ? "'unsafe-eval'" : ""};
    worker-src 'self' blob:;
    child-src 'self' blob:;
    style-src 'self' 'unsafe-inline' https://js.hcaptcha.com https://*.hcaptcha.com;
    img-src 'self' blob: data:;
    media-src 'self' blob:;
    font-src 'self';
    object-src 'none';
    connect-src ${connectSrc};
    base-uri 'self';
    form-action 'self';
    frame-src 'self' https://*.hcaptcha.com https://hcaptcha.com https://*.challenges.cloudflare.com https://challenges.cloudflare.com;
    frame-ancestors 'none';
    upgrade-insecure-requests;
    report-uri ${reportUri};
    report-to csp-endpoint;
  `;
    // Replace newline characters and spaces
    const contentSecurityPolicyHeaderValue = cspHeader
        .replace(/\s{2,}/g, " ")
        .trim();

    const requestHeaders = new Headers(request.headers);
    requestHeaders.set("x-nonce", nonce);
    requestHeaders.set(
        "Content-Security-Policy",
        contentSecurityPolicyHeaderValue
    );
    requestHeaders.set(
        "Report-To",
        `{"group":"csp-endpoint","max_age":10886400,"endpoints":[{"url":"${reportUri}"}]}`
    );

    const response = NextResponse.next({
        request: {
            headers: requestHeaders,
        },
    });
    response.headers.set(
        "Content-Security-Policy",
        contentSecurityPolicyHeaderValue
    );
    response.headers.set(
        "Report-To",
        `{"group":"csp-endpoint","max_age":10886400,"endpoints":[{"url":"${reportUri}"}]}`
    );

    return response;
}

export const config = {
    matcher: [
        /*
         * Match all request paths except for the ones starting with:
         * - api (API routes)
         * - _next/static (static files)
         * - _next/image (image optimization files)
         * - favicon.ico (favicon file)
         */
        {
            source: "/((?!api|_next/static|_next/image|favicon.ico).*)",
            missing: [
                { type: "header", key: "next-router-prefetch" },
                { type: "header", key: "purpose", value: "prefetch" },
            ],
        },
    ],
};
