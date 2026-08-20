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
import {useRouter} from "next/navigation";
import {FormEvent, useState} from "react";
import {signUp} from "@/signup";
import HCaptcha from "@hcaptcha/react-hcaptcha";
import { config } from "@/config/env";

export function SignupForm({
  className,
  ...props
}: React.ComponentProps<"div">) {
  const router = useRouter();
  const [email, setEmail] = useState<string>("");
  const [password, setPassword] = useState<string>("");

  const [recoveryPhrase, setRecoveryPhrase] = useState<string>("");

  if (recoveryPhrase) {
      return (
          <div className={cn("flex flex-col gap-6", className)} {...props}>
              <Card>
                  <CardHeader className="text-center">
                      <CardTitle className="text-xl">Save Your Recovery Phrase</CardTitle>
                      <CardDescription>
                          This is the <strong>ONLY</strong> way to recover your account if you forget your password. We cannot reset your password for you.
                      </CardDescription>
                  </CardHeader>
                  <CardContent className="flex flex-col gap-4">
                      <div className="p-4 bg-muted text-center rounded font-mono text-lg break-all">
                          {recoveryPhrase}
                      </div>
                      <Button onClick={() => router.push("/signin")}>
                          I have saved my recovery phrase
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
          <CardTitle className="text-xl">Create your account</CardTitle>
          <CardDescription>
            Private by design, for everyone
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
                <FieldLabel htmlFor="password">Password</FieldLabel>
                <Input
                    id="password"
                    type="password"
                    required
                    onChange={(e) => setPassword(e.target.value)} />
              </Field>
              <HCaptcha
                sitekey={config.captchaSitekey}
                onVerify={(token,ekey) => handleVerificationSuccess(token, ekey)}
              />
              <Field>
                <Button type="submit" onClick={(e) => {
                  e.preventDefault();
                  signUp(email, password)
                      .then((res) => {
                        console.log("Sign up successfully");
                        setRecoveryPhrase(res.recoveryPhrase);
                      })
                      .catch((err: Error) => {
                        console.error(`Error occured on sign up: ${err.message}`);
                      })
                }}>Create Account</Button>
                <FieldDescription className="text-center">
                  Already have an account? <a href="#" onClick={()=>{router.push('/signin')}}>Sign in</a>
                </FieldDescription>
              </Field>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>
      <FieldDescription className="px-6 text-center">
        By clicking continue, you agree to our <a href="#">Terms of Service</a>{" "}
        and <a href="#">Privacy Policy</a>.
      </FieldDescription>
    </div>
  )
}

function handleVerificationSuccess(token: string, ekey: string) {
  
}