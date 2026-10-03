"use client";

import { Folder } from "lucide-react";

import { ResetPasswordForm } from "@/components/reset-password-form";
import { Suspense } from "react";

export default function ResetPasswordPage() {
    return (
        <div className="bg-muted flex min-h-svh flex-col items-center justify-center gap-6 p-6 md:p-10">
            <div className="flex w-full max-w-sm flex-col gap-6">
                <a
                    href="#"
                    className="flex items-center gap-2 self-center font-medium"
                >
                    <div className="bg-primary text-primary-foreground flex size-6 items-center justify-center rounded-md">
                        <Folder className="size-4" />
                    </div>
                    Quartz Drive
                </a>
                <Suspense fallback={<div>Loading...</div>}>
                    <ResetPasswordForm />
                </Suspense>
            </div>
        </div>
    );
}
