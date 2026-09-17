"use client";

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
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {useRouter, useSearchParams} from "next/navigation";
import {useState, useEffect, Suspense} from "react";
import {resetPassword, verifyToken} from "@/reset-password";

export function ResetPasswordForm({
  className,
  ...props
}: React.ComponentProps<"div">) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const sessionId = searchParams.get("session_id");
  const token = searchParams.get("token");

  const [isVerifying, setIsVerifying] = useState<boolean>(true);
  const [verifyError, setVerifyError] = useState<string>("");

  const [newPassword, setNewPassword] = useState<string>("");
  const [passwordScore, setPasswordScore] = useState<number | null>(null);
  const [passwordFeedback, setPasswordFeedback] = useState<string>("");
  const [error, setError] = useState<string>("");
  const [loading, setLoading] = useState<boolean>(false);
  const [success, setSuccess] = useState<boolean>(false);

  useEffect(() => {
    if (!sessionId || !token) {
      setVerifyError("Missing session ID or token.");
      setIsVerifying(false);
      return;
    }

    verifyToken(sessionId, token)
      .then(() => {
        setIsVerifying(false);
      })
      .catch((err: Error) => {
        setVerifyError(err.message || "Provided token is invalid.");
        setIsVerifying(false);
      });
  }, [sessionId, token]);

  useEffect(() => {
    if (!newPassword) {
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
        import("@zxcvbn-ts/matcher-pwned")
      ]).then(([zxcvbnCore, common, en, matcherPwned]) => {
        if (!isMounted) return;

        const { ZxcvbnFactory } = zxcvbnCore;
        const { matcherPwnedFactory } = matcherPwned;

        const options = {
          dictionary: {
            ...common.dictionary,
            ...en.dictionary,
          },
          graphs: common.adjacencyGraphs,
          translations: en.translations,
        };

        const matcher = matcherPwnedFactory(window.fetch);
        const zxcvbn = new ZxcvbnFactory(options, { pwned: matcher });

        zxcvbn.checkAsync(newPassword, ["Quartz", "QuartzDrive", "quartzapp.top"]).then((result) => {
          if (isMounted) {
            setPasswordScore(result.score);
            if (result.feedback.warning) {
                setPasswordFeedback(result.feedback.warning);
            } else if (result.feedback.suggestions.length > 0) {
                setPasswordFeedback(result.feedback.suggestions[0]);
            } else {
                setPasswordFeedback("");
            }
          }
        });
      }).catch(console.error);
    }, 400);

    return () => {
      isMounted = false;
      clearTimeout(timer);
    };
  }, [newPassword]);

  if (isVerifying) {
    return (
      <div className={cn("flex flex-col gap-6", className)} {...props}>
        <Card>
          <CardHeader className="text-center">
            <CardTitle className="text-xl">Verifying Link...</CardTitle>
            <CardDescription>
              Please wait while we verify your password reset link.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center py-6">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (!sessionId || !token || verifyError) {
    return (
        <div className={cn("flex flex-col gap-6", className)} {...props}>
            <Card>
                <CardHeader className="text-center">
                    <CardTitle className="text-xl">Invalid Link</CardTitle>
                    <CardDescription>
                        {verifyError || "This password reset link is invalid or missing required parameters."}
                    </CardDescription>
                </CardHeader>
                <CardContent className="flex flex-col gap-4">
                    <Button onClick={() => router.push("/forgot-password")}>
                        Request a new link
                    </Button>
                </CardContent>
            </Card>
        </div>
    )
  }

  if (success) {
      return (
          <div className={cn("flex flex-col gap-6", className)} {...props}>
              <Card>
                  <CardHeader className="text-center">
                      <CardTitle className="text-xl">Password Reset Successful</CardTitle>
                      <CardDescription>
                          Your password has been successfully reset. 
                          <br /><br />
                          <strong>Note:</strong> Since you used Email Recovery, your data remains encrypted with your old password. 
                          You will be prompted to enter your Recovery Phrase to unlock your data after signing in.
                      </CardDescription>
                  </CardHeader>
                  <CardContent className="flex flex-col gap-4">
                      <Button onClick={() => router.push("/signin")}>
                          Sign in with new password
                      </Button>
                  </CardContent>
              </Card>
          </div>
      )
  }

  function submitForm(){
    if (!sessionId || !token) {
      throw new Error("verify response or token is missing")
    }

    setError("");
    setLoading(true);

    resetPassword(sessionId, newPassword)
        .then(() => {
          setSuccess(true);
          setLoading(false);
        })
        .catch((err: Error) => {
          setError(err.message)
          setLoading(false);
        })
  }

  return (
    <div className={cn("flex flex-col gap-6", className)} {...props}>
      <Card>
        <CardHeader className="text-center">
          <CardTitle className="text-xl">Create New Password</CardTitle>
          <CardDescription>
            Enter a strong new password for your account.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form action="#" method="POST" onSubmit={(e)=>e.preventDefault()}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="newPassword">New Password</FieldLabel>
                <Input
                    id="newPassword"
                  type="password"
                  autoComplete="new-password"
                    required
                    onChange={(e) => setNewPassword(e.target.value)} />
                {newPassword && (
                  <div className="mt-2 flex flex-col gap-1">
                    <div className="flex h-1.5 w-full overflow-hidden rounded-full bg-secondary">
                      {[0, 1, 2, 3].map((index) => {
                        let bgColor = "bg-transparent";
                        if (passwordScore !== null) {
                          let litSegments = 0;
                          if (passwordScore <= 1) litSegments = 1;
                          else if (passwordScore === 2) litSegments = 2;
                          else if (passwordScore === 3) litSegments = 3;
                          else if (passwordScore === 4) litSegments = 4;

                          if (index < litSegments) {
                            if (passwordScore <= 1) bgColor = "bg-destructive";
                            else if (passwordScore === 2) bgColor = "bg-orange-500";
                            else if (passwordScore === 3) bgColor = "bg-yellow-500";
                            else bgColor = "bg-green-500";
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
                      <span className="text-xs text-muted-foreground">{passwordFeedback}</span>
                    )}
                  </div>
                )}
              </Field>
              {error && <div className="text-red-500 text-sm text-center">{error}</div>}
              <Field>
                <Button type="submit" disabled={loading || passwordScore === null || passwordScore < 3} onClick={(e) => submitForm()}>
                  {loading ? "Resetting..." : "Reset Password"}
                </Button>
              </Field>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
