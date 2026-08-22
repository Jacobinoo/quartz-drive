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

export function ForgotPasswordForm({
  className,
  ...props
}: React.ComponentProps<"div">) {
  const router = useRouter();
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

  return (
    <div className={cn("flex flex-col gap-6", className)} {...props}>
      <Card>
        <CardHeader className="text-center">
          <CardTitle className="text-xl">Reset Password</CardTitle>
          <CardDescription>
            Enter your email to receive a recovery link.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form action="#" method="POST">
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="email">Email</FieldLabel>
                <Input
                  id="email"
                  type="email"
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

                  fetch(`${config.apiUrl}/v1/account/forgot-password`, {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ email, turnstileToken })
                  })
                  .then((res) => {
                    if (res.ok) {
                        setSubmitted(true);
                        setCountdown(60);
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
                        setError("Failed to send reset link.");
                        turnstileRef.current?.reset();
                    }
                  })
                  .catch((err: Error) => {
                    setError("Failed to connect to server.");
                    turnstileRef.current?.reset();
                  })
                }}>Send Recovery Link</Button>
                <FieldDescription className="text-center">
                  Remember your password? <a href="#" onClick={(e)=>{e.preventDefault(); router.push('/signin')}}>Sign in</a>
                </FieldDescription>
              </Field>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
