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
    FieldDescription,
    FieldGroup,
    FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useRouter } from "next/navigation";
import { FormEvent, useEffect, useState } from "react";
import { signUp } from "@/signup";
import HCaptcha from "@hcaptcha/react-hcaptcha";
import { config } from "@/config/env";

export function SignupForm({
    className,
    ...props
}: React.ComponentProps<"div">) {
    const router = useRouter();
    const [email, setEmail] = useState<string>("");
    const [password, setPassword] = useState<string>("");
    const [captchaToken, setCaptchaToken] = useState<string | null>(null);
    const [passwordScore, setPasswordScore] = useState<number | null>(null);
    const [passwordFeedback, setPasswordFeedback] = useState<string>("");
    const [error, setError] = useState<string>("");

    const [signupComplete, setSignupComplete] = useState<boolean>(false);
    const [devVerificationUrl, setDevVerificationUrl] = useState<string | null>(
        null
    );

    useEffect(() => {
        if (!password) {
            setPasswordScore(null);
            setPasswordFeedback("");
            return;
        }

        let isMounted = true;

        const timer = setTimeout(() => {
            Promise.all([
                import("@zxcvbn-ts/core"),
                import("@zxcvbn-ts/language-common"),
                import("@zxcvbn-ts/language-en"),
                import("@zxcvbn-ts/language-pl"),
                import("@zxcvbn-ts/matcher-pwned"),
            ])
                .then(([zxcvbnCore, common, en, pl, matcherPwned]) => {
                    if (!isMounted) return;

                    const { ZxcvbnFactory } = zxcvbnCore;
                    const { matcherPwnedFactory } = matcherPwned;

                    const options = {
                        dictionary: {
                            ...common.dictionary,
                            ...en.dictionary,
                            ...pl.dictionary,
                        },
                        graphs: common.adjacencyGraphs,
                        translations: en.translations,
                    };

                    const matcher = matcherPwnedFactory(window.fetch);
                    const zxcvbn = new ZxcvbnFactory(options, {
                        pwned: matcher,
                    });

                    zxcvbn
                        .checkAsync(password, [
                            email,
                            deriveUsernameFromEmail(email),
                            "Quartz",
                            "QuartzDrive",
                            "quartzapp.top",
                        ])
                        .then((result) => {
                            if (isMounted) {
                                setPasswordScore(result.score);
                                if (result.feedback.warning) {
                                    setPasswordFeedback(
                                        result.feedback.warning
                                    );
                                } else if (
                                    result.feedback.suggestions.length > 0
                                ) {
                                    setPasswordFeedback(
                                        result.feedback.suggestions[0]
                                    );
                                } else {
                                    setPasswordFeedback("");
                                }
                            }
                        });
                })
                .catch(console.error);
        }, 400);

        return () => {
            isMounted = false;
            clearTimeout(timer);
        };
    }, [password]);

    if (signupComplete) {
        return (
            <div className={cn("flex flex-col gap-6", className)} {...props}>
                <Card>
                    <CardHeader className="text-center">
                        <CardTitle className="text-xl">
                            Check your email
                        </CardTitle>
                        <CardDescription>
                            We just sent a verification link to{" "}
                            <span className="font-medium text-foreground">
                                {email}
                            </span>
                            .
                        </CardDescription>
                    </CardHeader>
                    {devVerificationUrl ? (
                        <CardContent className="pt-0">
                            <div className="p-4 bg-amber-500/10 border border-amber-500/20 rounded-xl text-xs space-y-3 text-left">
                                <div className="font-semibold text-amber-600 dark:text-amber-400 flex items-center gap-1.5 text-sm">
                                    Local Development
                                </div>
                                <p className="text-muted-foreground leading-relaxed">
                                    No real email was sent. Since you are
                                    running in local development mode, you can
                                    verify your account immediately:
                                </p>
                                <a
                                    href={devVerificationUrl}
                                    className="inline-flex items-center justify-center w-full px-4 py-2.5 bg-amber-500 hover:bg-amber-600 active:bg-amber-700 text-white font-medium text-sm rounded-lg transition-colors shadow-sm"
                                >
                                    Verify Account Now &rarr;
                                </a>
                                <p className="text-[11px] text-muted-foreground/80 text-center">
                                    Alternatively, the link is also printed in
                                    your server logs.
                                </p>
                            </div>
                        </CardContent>
                    ) : process.env.NODE_ENV === "development" ? (
                        <CardContent className="pt-0">
                            <div className="p-3 bg-zinc-100 dark:bg-zinc-800/60 border border-zinc-200 dark:border-zinc-700 rounded-lg text-xs text-muted-foreground text-left">
                                <span className="font-semibold text-foreground flex items-center gap-1.5 mb-1">
                                    Development Mode
                                </span>
                                No email was sent. Check server logs:
                                <pre className="mt-1.5 p-1.5 bg-black/5 dark:bg-black/30 rounded font-mono text-[11px] select-all">
                                    docker logs server-quartz-api-1
                                </pre>
                            </div>
                        </CardContent>
                    ) : null}
                </Card>
            </div>
        );
    }

    return (
        <div className={cn("flex flex-col gap-6", className)} {...props}>
            <Card>
                <CardHeader className="text-center">
                    <CardTitle className="text-xl">
                        Create your account
                    </CardTitle>
                    <CardDescription>
                        Private by design, for everyone
                    </CardDescription>
                </CardHeader>
                <CardContent>
                    <form
                        action="#"
                        method="POST"
                        onSubmit={(e) => {
                            e.preventDefault();
                            setError("");
                            if (captchaToken == null) {
                                setError("Please complete the captcha.");
                                return;
                            }
                            if (passwordScore === null || passwordScore < 3) {
                                setError("Please choose a stronger password.");
                                return;
                            }
                            signUp(email, password, captchaToken)
                                .then((res) => {
                                    console.log("Sign up successfully.");
                                    if (res?.devVerificationUrl) {
                                        setDevVerificationUrl(
                                            res.devVerificationUrl
                                        );
                                    }
                                    setSignupComplete(true);
                                })
                                .catch((err: Error) => {
                                    setError(
                                        `Error occurred on sign up: ${err.message}`
                                    );
                                    console.error(
                                        `Error occurred on sign up: ${err.message}`
                                    );
                                });
                        }}
                    >
                        <FieldGroup>
                            <Field>
                                <FieldLabel htmlFor="email">Email</FieldLabel>
                                <Input
                                    id="email"
                                    type="email"
                                    autoComplete="email"
                                    placeholder="m@example.com"
                                    required
                                    onChange={(e) => setEmail(e.target.value)}
                                />
                            </Field>
                            <Field>
                                <FieldLabel htmlFor="password">
                                    Password
                                </FieldLabel>
                                <Input
                                    id="password"
                                    type="password"
                                    autoComplete="new-password"
                                    required
                                    onChange={(e) =>
                                        setPassword(e.target.value)
                                    }
                                />
                                {password && (
                                    <div className="mt-2 flex flex-col gap-1">
                                        <div className="flex h-1.5 w-full overflow-hidden rounded-full bg-secondary">
                                            {[0, 1, 2, 3].map((index) => {
                                                let bgColor = "bg-transparent";
                                                if (passwordScore !== null) {
                                                    let litSegments = 0;
                                                    if (passwordScore <= 1)
                                                        litSegments = 1;
                                                    else if (
                                                        passwordScore === 2
                                                    )
                                                        litSegments = 2;
                                                    else if (
                                                        passwordScore === 3
                                                    )
                                                        litSegments = 3;
                                                    else if (
                                                        passwordScore === 4
                                                    )
                                                        litSegments = 4;

                                                    if (index < litSegments) {
                                                        if (passwordScore <= 1)
                                                            bgColor =
                                                                "bg-destructive";
                                                        else if (
                                                            passwordScore === 2
                                                        )
                                                            bgColor =
                                                                "bg-orange-500";
                                                        else if (
                                                            passwordScore === 3
                                                        )
                                                            bgColor =
                                                                "bg-yellow-500";
                                                        else
                                                            bgColor =
                                                                "bg-green-500";
                                                    }
                                                }
                                                return (
                                                    <div
                                                        key={index}
                                                        className={`flex-1 transition-colors duration-300 ${bgColor} ${index > 0 ? "border-l border-background/20" : ""}`}
                                                    />
                                                );
                                            })}
                                        </div>
                                        {passwordFeedback && (
                                            <span className="text-xs text-muted-foreground">
                                                {passwordFeedback}
                                            </span>
                                        )}
                                    </div>
                                )}
                            </Field>
                            <HCaptcha
                                sitekey={config.captchaSitekey}
                                onVerify={(token, ekey) => {
                                    setCaptchaToken(token);
                                }}
                            />
                            <Field>
                                {error && (
                                    <div className="text-red-500 text-sm text-center font-medium">
                                        {error}
                                    </div>
                                )}
                                <Button
                                    type="submit"
                                    disabled={
                                        passwordScore === null ||
                                        passwordScore < 3
                                    }
                                >
                                    Create Account
                                </Button>
                                <FieldDescription className="text-center">
                                    Already have an account?{" "}
                                    <a
                                        href="#"
                                        onClick={() => {
                                            router.push("/signin");
                                        }}
                                    >
                                        Sign in
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
    );
}

function deriveUsernameFromEmail(email: string) {
    const _email = email.trim();
    if (email.trim() == "") {
        return "";
    }

    const username = _email.split("@")[0];
    if (username == null) {
        return "";
    }
    const usernameTrimmed = username.trim();

    if (usernameTrimmed == "") {
        return "";
    }

    return usernameTrimmed;
}
