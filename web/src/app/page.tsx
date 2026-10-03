"use client";

import { useEffect, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";

function RedirectToDrive() {
    const router = useRouter();
    const searchParams = useSearchParams();

    useEffect(() => {
        const params = searchParams.toString();
        const query = params ? `?${params}` : "";
        router.push(`/drive${query}`);
    }, [router, searchParams]);

    return null;
}

export default function Home() {
    return (
        <Suspense fallback={null}>
            <RedirectToDrive />
        </Suspense>
    );
}
