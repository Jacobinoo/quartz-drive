"use client";
import {
    Card,
    CardDescription,
    CardHeader,
    CardTitle,
} from "@/components/ui/card";
import { config } from "@/config/env";
import { cn } from "@/lib/utils";
import { Folder } from "lucide-react";
import { useRouter } from "next/navigation";
import React, { useEffect, useRef, useState } from "react";

export default function VerifyEmailPage() {
    const token = window.location.hash.slice(1).split("token=").at(1);

    const router = useRouter();
    const [invalidToken, setInvalidToken] = useState<boolean>(false);
    const hasFired = useRef(false);

    useEffect(() => {
        if (token == null || token == "") {
            setInvalidToken(true);
            return;
        }

        if (hasFired.current) return;
        hasFired.current = true;

        verifyEmail(token)
            .then((d) => {
                if (d != "") {
                    sessionStorage.setItem("email", d);
                    router.push("/signin?verified=1");
                    return;
                }
            })
            .catch((err) => {
                throw new Error(err);
            });
    }, []);

    function verifying() {
        return (
            <div className={"flex flex-col gap-6"}>
                <Card>
                    <CardHeader className="flex flex-col text-center items-center">
                        <div className="h-8 w-8 rounded-full border-2 border-primary/15 border-t-primary/60 animate-spin mb-2" />
                        <CardTitle className="text-xl">
                            Verifying your email
                        </CardTitle>
                    </CardHeader>
                </Card>
            </div>
        );
    }

    function invalid() {
        return (
            <div className={"flex flex-col gap-6"}>
                <Card>
                    <CardHeader className="flex flex-col text-center items-center">
                        <CardTitle className="text-xl">
                            Verification link is invalid
                        </CardTitle>
                    </CardHeader>
                </Card>
            </div>
        );
    }

    return (
        <div className="bg-muted flex min-h-svh flex-col items-center justify-center gap-6 p-6 md:p-10">
            <div className="flex w-full max-w-sm flex-col gap-6">
                <div className="flex items-center gap-2 self-center font-medium">
                    <div className="bg-primary text-primary-foreground flex size-6 items-center justify-center rounded-md">
                        <Folder className="size-4" />
                    </div>
                    Quartz Drive
                </div>
                {invalidToken ? invalid() : verifying()}
            </div>
        </div>
    );
}

async function verifyEmail(token: string): Promise<string> {
    try {
        const res = await fetch(`${config.apiUrl}/v1/account/verify-email`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                token: token,
            }),
        });

        if (!res.ok) {
            console.error("something went wrong with verify email response");
            throw new Error("Failed to verify email");
        }
        let json = await res.json();

        return json.email;
    } catch (e) {
        console.error(e);
        return "";
    }
}
