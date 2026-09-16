"use client";
import { config } from "@/config/env";

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {useRouter} from "next/navigation";
import {useState, useRef} from "react";
import { Turnstile } from '@marsidev/react-turnstile'
import type { TurnstileInstance } from '@marsidev/react-turnstile'
import { KeyRound, Mail } from "lucide-react";
import { Textarea } from "./ui/textarea";

type RecoveryMethod = 'EMAIL' | 'RECOVERY_PHRASE' | null

export function ForgotPasswordForm({
  className,
  ...props
}: React.ComponentProps<"div">) {
  const router = useRouter();

  const [method, setMethod] = useState<RecoveryMethod>(null)
  const [recoveryPhrase, setRecoveryPhrase] = useState<string>("")

  const [email, setEmail] = useState<string>("");
  const [submitted, setSubmitted] = useState<boolean>(false);
  const [error, setError] = useState<string>("");
  const [countdown, setCountdown] = useState<number>(0);
  const turnstileRef = useRef<TurnstileInstance | null>(null)

  if (submitted) {
    return (
      <div className={cn("flex flex-col gap-6", className)} {...props}>
        <Card>
          <CardHeader className="text-center">
            <CardTitle className="text-xl">Check your email</CardTitle>
            <CardDescription>
              We've sent a magic link to {email}. Click the link to reset your password.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Button onClick={() => router.push("/signin")}>
                Return to sign in
            </Button>
            <Button
                variant="outline"
                disabled={countdown > 0}
                onClick={() => {
                    if (countdown === 0) {
                        setSubmitted(false);
                        turnstileRef.current?.reset();
                    }
                }}
            >
                {countdown > 0 ? `Retry in ${countdown}s` : "Didn't receive it? Try again"}
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  switch (method) {

    case null: return (
      <div className={cn("flex flex-col gap-6", className)} {...props}>
        <Card>
          <CardHeader className="text-center">
            <CardTitle className="text-xl">Account & Data Recovery</CardTitle>
            <CardDescription>
              Choose a recovery method to regain access to your account.
            </CardDescription>
          </CardHeader>
          <CardContent className={"flex flex-col gap-3"}>
            <button className="flex items-center justify-between text-left p-4 border rounded-lg bg-muted/50 cursor-pointer hover:bg-gray-200/40 dark:hover:bg-muted/35" onClick={() => {setMethod('RECOVERY_PHRASE')}}>
              <div className="flex items-center gap-3">
                <KeyRound className="size-5 text-primary w-1/6" />
                <div className="flex flex-col gap-0.5">
                  <div className={"flex items-center justify-between"}>
                    <span className="text-sm font-medium">Recovery Phrase</span>
                    <span className="text-xs font-semibold text-blue-600 bg-blue-100 px-2 py-0.5 rounded-full dark:bg-blue-900/30 dark:text-blue-400">Recommended</span>
                  </div>
                  <span className="text-xs text-muted-foreground">Can be used for both resetting your password and recovering your data</span>
                </div>
              </div>
            </button>
            <button className="flex items-center justify-between text-left p-4 border rounded-lg bg-muted/50 cursor-pointer hover:bg-gray-200/40 dark:hover:bg-muted/35" onClick={() => {setMethod('EMAIL')}}>
              <div className="flex items-center gap-3">
                <Mail className="size-5 text-muted-foreground w-1/6" />
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium">Email Recovery</span>
                  <span className="text-xs text-muted-foreground">Can reset your password to allow you to sign in, but won't recover your data without the recovery phrase.</span>
                </div>
              </div>
            </button>
            <form action="#" method="POST">
              <Field>
                <FieldDescription className="text-center">
                  Remember your credentials? <a href="#" onClick={(e) => { e.preventDefault(); router.push('/signin') }}>Sign in</a>
                </FieldDescription>
              </Field>
            </form>
          </CardContent>
        </Card>
      </div>
    )
    case 'EMAIL':
      return (
        <div className={cn("flex flex-col gap-6", className)} {...props}>
          <Card>
            <CardHeader className="text-center">
              <CardTitle className="text-xl">Email Recovery</CardTitle>
              <CardDescription>
                We will send you an email with a magic link, click it to reset your password.
              </CardDescription>
              <p className={"text-xs px-3 py-1 bg-amber-500/20 text-amber-800 dark:bg-amber-900/20 dark:text-amber-500 rounded-md"}>Email recovery will allow you to regain access to your account, but you will still need a recovery phrase to recover your files! <a href="#" className={"underline"} onClick={() => {
                setMethod("RECOVERY_PHRASE")
              }}>Have a recovery phrase?</a></p>
            </CardHeader>
            <CardContent>
              <form action="#" method="POST">
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="email">Enter your email</FieldLabel>
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
                    <Turnstile
                      ref={turnstileRef}
                      siteKey={config.turnstileSitekey}
                    />
                  </Field>

                  {error && <div className="text-red-500 text-sm text-center">{error}</div>}
                  <Field>
                    <Button type="submit" onClick={(e) => {
                      e.preventDefault();
                      setError("");

                      const turnstileToken = turnstileRef.current?.getResponse()
                      if (!turnstileToken) {
                        setError("Please complete the security check.");
                        return;
                      }

                      fetch(`${config.apiUrl}/v1/recovery/start`, {
                        method: "POST",
                        headers: { "Content-Type": "application/json", "X-Verify-Token": turnstileToken },
                        body: JSON.stringify({ method: "email", email })
                      })
                        .then((res) => {
                          if (res.ok) {
                            setSubmitted(true);
                            setCountdown(70);
                            const interval = setInterval(() => {
                              setCountdown((prev) => {
                                if (prev <= 1) {
                                  clearInterval(interval);
                                  return 0;
                                }
                                return prev - 1;
                              });
                            }, 1000);
                          } else {
                            const body = res.json()
                            body.then((d) => {
                              setError(`Failed to send reset link: ${d.message}`);
                              turnstileRef.current?.reset();
                            })
                          }
                        })
                        .catch((err: Error) => {
                          setError("Failed to connect to server.");
                          turnstileRef.current?.reset();
                        })
                    }}>Send Recovery Link</Button>
                    <FieldDescription className="text-center">
                      <a href="#" onClick={(e) => { e.preventDefault(); setMethod(null) }}>Choose a different recovery method</a>
                    </FieldDescription>
                  </Field>
                </FieldGroup>
              </form>
            </CardContent>
          </Card>
        </div>
      )
    case 'RECOVERY_PHRASE':
      return (
        <div className={cn("flex flex-col gap-6", className)} {...props}>
          <Card>
            <CardHeader className="text-center">
              <CardTitle className="text-xl">Use your Recovery Phrase</CardTitle>
              <CardDescription>
                Recovery phrase consists of 24 words. Enter it below to reset your password and recover your data.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form action="#" method="POST">
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="email">Recovery Phrase</FieldLabel>
                    <Textarea
                      placeholder="Type your 24-word phrase here..."
                      className={"resize-none"}
                      required
                      autoComplete="off"
                      value={recoveryPhrase}
                      onChange={(e) => setRecoveryPhrase(e.target.value)}
                    />
                  </Field>

                  <Field>
                    <Turnstile
                      ref={turnstileRef}
                      siteKey={config.turnstileSitekey}
                    />
                  </Field>

                  {error && <div className="text-red-500 text-sm text-center">{error}</div>}
                  <Field>
                    <Button type="submit" onClick={(e) => {
                      e.preventDefault();
                      setError("");

                      const turnstileToken = turnstileRef.current?.getResponse()
                      if (!turnstileToken) {
                        setError("Please complete the security check.");
                        return;
                      }

                      fetch(`${config.apiUrl}/v1/account/forgot-password`, {
                        method: "POST",
                        headers: { "Content-Type": "application/json", "X-Verify-Token": turnstileToken },
                        body: JSON.stringify({ email })
                      })
                        .then((res) => {
                          if (res.ok) {
                            setSubmitted(true);
                            setCountdown(70);
                            const interval = setInterval(() => {
                              setCountdown((prev) => {
                                if (prev <= 1) {
                                  clearInterval(interval);
                                  return 0;
                                }
                                return prev - 1;
                              });
                            }, 1000);
                          } else {
                            const body = res.json()
                            body.then((d) => {
                              setError(`Failed to send reset link: ${d.message}`);
                              turnstileRef.current?.reset();
                            })
                          }
                        })
                        .catch((err: Error) => {
                          setError("Failed to connect to server.");
                          turnstileRef.current?.reset();
                        })
                    }}>Continue</Button>
                    <FieldDescription className="text-center">
                      <a href="#" onClick={(e) => { e.preventDefault(); setMethod(null) }}>Choose a different recovery method</a>
                    </FieldDescription>
                  </Field>
                </FieldGroup>
              </form>
            </CardContent>
          </Card>
        </div>
      )
  }
}
