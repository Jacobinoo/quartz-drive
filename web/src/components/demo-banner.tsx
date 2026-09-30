"use client";

import { useEffect, useState } from "react";
import { getAccessToken } from "@/lib/authStore";
import { AlertTriangle } from "lucide-react";

function parseJwt(token: string) {
    try {
        const base64Url = token.split('.')[1];
        const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
        const jsonPayload = decodeURIComponent(window.atob(base64).split('').map(function(c) {
            return '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2);
        }).join(''));

        return JSON.parse(jsonPayload);
    } catch (e) {
        return null;
    }
}

export function DemoBanner() {
    const [timeLeft, setTimeLeft] = useState<string | null>(null);
    const [isDemo, setIsDemo] = useState(false);

    useEffect(() => {
        const token = getAccessToken();
        if (!token) return;

        const payload = parseJwt(token);
        if (!payload || !payload.isDemo) return;

        setIsDemo(true);
        
        if (!payload.demoExpiresAt) return;
        
        // demoExpiresAt is a unix timestamp (seconds), multiply by 1000 for JS Date
        const expiresAt = payload.demoExpiresAt * 1000;

        const interval = setInterval(() => {
            const now = new Date().getTime();
            const distance = expiresAt - now;

            if (distance < 0) {
                setTimeLeft("EXPIRED");
                clearInterval(interval);
                return;
            }

            const minutes = Math.floor((distance % (1000 * 60 * 60)) / (1000 * 60));
            const seconds = Math.floor((distance % (1000 * 60)) / 1000);

            setTimeLeft(`${minutes}m ${seconds}s`);
        }, 1000);

        return () => clearInterval(interval);
    }, []);

    if (!isDemo) return null;

    return (
        <div className="bg-yellow-500/90 text-yellow-950 px-4 py-2 text-center text-sm font-medium flex items-center justify-center gap-2 sticky top-0 z-50">
            <AlertTriangle className="w-4 h-4" />
            <span>
                This is a live demo session. Data loss can and will occur. Do not upload sensitive data. 
                {timeLeft && <span className="ml-2 font-bold tabular-nums">Expires in: {timeLeft}</span>}
            </span>
        </div>
    );
}
