"use client";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
    Card,
    CardContent,
    CardDescription,
    CardHeader,
    CardTitle,
} from "@/components/ui/card";
import {
    Field,
    FieldContent,
    FieldDescription,
    FieldGroup,
    FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useRouter, useSearchParams } from "next/navigation";
import React, { useEffect, useState } from "react";
import { signIn } from "@/signin";
import { Turnstile } from "@marsidev/react-turnstile";
import type { TurnstileInstance } from "@marsidev/react-turnstile";
import { useRef } from "react";
import { config } from "@/config/env";
import { Checkbox } from "./ui/checkbox";
import { _email } from "zod/v4/core";
import { Check, CircleCheckBig, UserCheck } from "lucide-react";

export function LoginForm({
    className,
    ...props
}: React.ComponentProps<"div">) {
    const router = useRouter();
    const params = useSearchParams();

    const [email, setEmail] = useState<string>("");
    const [password, setPassword] = useState<string>("");
    const turnstileRef = useRef<TurnstileInstance | null>(null);
    const [checked, setChecked] = React.useState(false);
    const [emailVerified, setEmailVerified] = useState(false);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        if (!params.get("verified")) return;
        let _email = sessionStorage.getItem("email");
        if (!_email) return;
        sessionStorage.removeItem("email");
        setEmailVerified(true);
        setEmail(_email);
    }, []);

    function KeepMeSignedInCheckbox() {
        return (
            <FieldGroup className="mx-auto">
                <Field orientation="horizontal">
                    <Checkbox
                        id="session-checkbox-desc"
                        name="session-checkbox-desc"
                        checked={checked}
                        className="cursor-pointer"
                        onCheckedChange={(checked) =>
                            setChecked(checked === true)
                        }
                    />
                    <FieldContent className="gap-0">
                        <FieldLabel
                            htmlFor="session-checkbox-desc"
                            className="cursor-pointer"
                        >
                            Keep me signed in
                        </FieldLabel>
                        <FieldDescription>
                            Recommended only on trusted devices
                        </FieldDescription>
                    </FieldContent>
                </Field>
            </FieldGroup>
        );
    }

    function EmailVerifiedView() {
        return (
            <div className={cn("flex flex-col gap-6", className)} {...props}>
                <Card>
                    <CardHeader className="text-center">
                        <CardTitle className="text-xl">
                            Your email has been verified
                        </CardTitle>
                        <CardDescription>
                            Enter your password to continue
                        </CardDescription>
                    </CardHeader>
                    <CardContent className={"mt-3"}>
                        <form action="#" method="POST">
                            <FieldGroup>
                                {error && (
                                    <div className="text-sm font-medium text-destructive text-center bg-destructive/10 p-2 rounded-md">
                                        {error}
                                    </div>
                                )}
                                <Field className={"hidden"}>
                                    <Input
                                        id="email"
                                        hidden
                                        autoComplete="off"
                                        type="email"
                                        required
                                        disabled={true}
                                        value={email}
                                    />
                                </Field>
                                <Field>
                                    <div className="flex items-center">
                                        <FieldLabel htmlFor="password">
                                            Password
                                        </FieldLabel>
                                        <a
                                            href="#"
                                            onClick={(e) => {
                                                e.preventDefault();
                                                router.push("/forgot-password");
                                            }}
                                            className="ml-auto text-sm underline-offset-4 hover:underline"
                                        >
                                            Forgot your password?
                                        </a>
                                    </div>
                                    <Input
                                        id="password"
                                        type="password"
                                        autoComplete="current-password"
                                        required
                                        onChange={(e) =>
                                            setPassword(e.target.value)
                                        }
                                    />
                                </Field>
                                <Turnstile
                                    ref={turnstileRef}
                                    siteKey={config.turnstileSitekey}
                                    className={"self-center"}
                                />
                                <KeepMeSignedInCheckbox />
                                <Field>
                                    <Button
                                        type="submit"
                                        onClick={(e) => {
                                            e.preventDefault();
                                            setError(null);

                                            const token =
                                                turnstileRef.current?.getResponse();
                                            if (!token) {
                                                throw new Error(
                                                    "turnstile verification error"
                                                );
                                            }

                                            signIn(
                                                email,
                                                password,
                                                token,
                                                checked
                                            )
                                                .then((result) => {
                                                    console.log(
                                                        "Sign in successful"
                                                    );
                                                    if (
                                                        result == "ok" ||
                                                        result ==
                                                            "recovery_needed"
                                                    ) {
                                                        router.push("/drive");
                                                    } else {
                                                        console.warn(
                                                            "Onboarding required. Account keys not initialized!"
                                                        );
                                                        router.push(
                                                            "/onboarding"
                                                        );
                                                    }
                                                })
                                                .catch((err: Error) => {
                                                    console.error(
                                                        `Error occured on sign in: ${err.message}`
                                                    );
                                                    if (
                                                        err.name ===
                                                            "TypeError" ||
                                                        err.message.includes(
                                                            "is not a"
                                                        ) ||
                                                        err.message.includes(
                                                            "undefined"
                                                        )
                                                    ) {
                                                        setError(
                                                            "An internal error occurred. Please ensure your browser is up to date and try again."
                                                        );
                                                    } else {
                                                        setError(
                                                            err.message ||
                                                                "An unexpected error occurred."
                                                        );
                                                    }
                                                })
                                                .finally(() => {
                                                    turnstileRef.current?.reset(); // Reset after submission
                                                });
                                        }}
                                    >
                                        Login as {email}
                                    </Button>
                                </Field>
                            </FieldGroup>
                        </form>
                    </CardContent>
                </Card>
                <FieldDescription className="px-6 text-center">
                    By clicking continue, you agree to our{" "}
                    <a href="/terms">Terms of Service</a> and{" "}
                    <a href="/privacy">Privacy Policy</a>.
                </FieldDescription>
            </div>
        );
    }

    return !emailVerified ? (
        <div className={cn("flex flex-col gap-6", className)} {...props}>
            <Card>
                <CardHeader className="text-center">
                    <CardTitle className="text-xl">Sign in</CardTitle>
                    <CardDescription>
                        To continue to Quartz Drive
                    </CardDescription>
                </CardHeader>
                <CardContent>
                    <form action="#" method="POST">
                        <FieldGroup>
                            {error && (
                                <div className="text-sm font-medium text-destructive text-center bg-destructive/10 p-2 rounded-md">
                                    {error}
                                </div>
                            )}
                            <Field>
                                <FieldLabel htmlFor="email">Email</FieldLabel>
                                <Input
                                    id="email"
                                    autoComplete="email"
                                    type="email"
                                    placeholder="m@example.com"
                                    required
                                    value={email}
                                    onChange={(e) => setEmail(e.target.value)}
                                />
                            </Field>
                            <Field>
                                <div className="flex items-center">
                                    <FieldLabel htmlFor="password">
                                        Password
                                    </FieldLabel>
                                    <a
                                        href="#"
                                        onClick={(e) => {
                                            e.preventDefault();
                                            router.push("/forgot-password");
                                        }}
                                        className="ml-auto text-sm underline-offset-4 hover:underline"
                                    >
                                        Trouble signing in?
                                    </a>
                                </div>
                                <Input
                                    id="password"
                                    type="password"
                                    autoComplete="current-password"
                                    required
                                    onChange={(e) =>
                                        setPassword(e.target.value)
                                    }
                                />
                            </Field>
                            <Turnstile
                                ref={turnstileRef}
                                siteKey={config.turnstileSitekey}
                            />
                            <KeepMeSignedInCheckbox />
                            <Field>
                                <Button
                                    type="submit"
                                    onClick={(e) => {
                                        e.preventDefault();
                                        setError(null);

                                        const token =
                                            turnstileRef.current?.getResponse();
                                        if (!token) {
                                            throw new Error(
                                                "turnstile verification error"
                                            );
                                        }

                                        signIn(email, password, token, checked)
                                            .then((result) => {
                                                console.log(
                                                    "Sign in successful"
                                                );
                                                if (
                                                    result == "ok" ||
                                                    result == "recovery_needed"
                                                ) {
                                                    router.push("/drive");
                                                } else {
                                                    console.warn(
                                                        "Onboarding required. Account keys not initialized!"
                                                    );
                                                    router.push("/onboarding");
                                                }
                                            })
                                            .catch((err: Error) => {
                                                console.error(
                                                    `Error occured on sign in: ${err.message}`
                                                );
                                                if (
                                                    err.name === "TypeError" ||
                                                    err.message.includes(
                                                        "is not a"
                                                    ) ||
                                                    err.message.includes(
                                                        "undefined"
                                                    )
                                                ) {
                                                    setError(
                                                        "An internal error occurred. Please ensure your browser is up to date and try again."
                                                    );
                                                } else {
                                                    setError(
                                                        err.message ||
                                                            "An unexpected error occurred."
                                                    );
                                                }
                                            })
                                            .finally(() => {
                                                turnstileRef.current?.reset(); // Reset after submission
                                            });
                                    }}
                                >
                                    Login
                                </Button>
                                <FieldDescription className="text-center">
                                    New to Quartz?{" "}
                                    <a
                                        href="#"
                                        onClick={() => {
                                            router.push("/signup");
                                        }}
                                    >
                                        Create account
                                    </a>
                                </FieldDescription>
                            </Field>
                        </FieldGroup>
                    </form>
                </CardContent>
            </Card>
            <FieldDescription className="px-6 text-center">
                By clicking continue, you agree to our{" "}
                <a href="/terms">Terms of Service</a> and{" "}
                <a href="/privacy">Privacy Policy</a>.
            </FieldDescription>
        </div>
    ) : (
        EmailVerifiedView()
    );
}
