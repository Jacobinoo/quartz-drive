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
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {useRouter, useSearchParams} from "next/navigation";
import {useState, useEffect} from "react";
import {resetPassword, VerifyResponse, verifyToken} from "@/reset-password";

export function ResetPasswordForm({
  className,
  ...props
}: React.ComponentProps<"div">) {
  const router = useRouter();
  const token = window.location.hash.slice(1).split("token=").at(1)


  const [verifyResponse, setVerifyResponse] = useState<VerifyResponse | null>(null)
  const [isVerifying, setIsVerifying] = useState<boolean>(true);
  const [verifyError, setVerifyError] = useState<string>("");

  const [recoveryPhrase, setRecoveryPhrase] = useState<string>("");
  const [newPassword, setNewPassword] = useState<string>("");
  const [error, setError] = useState<string>("");
  const [loading, setLoading] = useState<boolean>(false);
  const [success, setSuccess] = useState<boolean>(false);

  useEffect(() => {
    if (!token) {
      setIsVerifying(false);
      return;
    }

    verifyToken(token)
      .then((d) => {
        setVerifyResponse(d);
        setIsVerifying(false);
      })
      .catch((err: Error) => {
        setVerifyError(err.message || "Provided token is invalid.");
        setIsVerifying(false);
      });
  }, [token]);

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

  if (!token || verifyError || !verifyResponse) {
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
                          Your password has been securely reset using your recovery phrase.
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
    if (verifyResponse == null || !token) {
      throw new Error("verify response or token is missing")
    }

    setError("");
    setLoading(true);

    resetPassword(verifyResponse, token, recoveryPhrase, newPassword)
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
          <CardTitle className="text-xl">Recover Account</CardTitle>
          <CardDescription>
            Enter your 12-word recovery phrase and a new password.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form action="#" method="POST" onSubmit={(e)=>e.preventDefault()}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="recoveryPhrase">Recovery Phrase (12 Words)</FieldLabel>
                <Input
                  id="recoveryPhrase"
                  type="text"
                  placeholder="apple banana cherry..."
                  required
                  onChange={(e) => setRecoveryPhrase(e.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="newPassword">New Password</FieldLabel>
                <Input
                    id="newPassword"
                    type="password"
                    required
                    onChange={(e) => setNewPassword(e.target.value)} />
              </Field>
              {error && <div className="text-red-500 text-sm text-center">{error}</div>}
              <Field>
                <Button type="submit" disabled={loading} onClick={(e) => submitForm()}>
                  {loading ? "Recovering..." : "Reset Password"}
                </Button>
              </Field>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
