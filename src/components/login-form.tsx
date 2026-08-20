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
import {useState} from "react";
import { signIn } from "@/signin";
import { Turnstile } from '@marsidev/react-turnstile'
import type { TurnstileInstance } from '@marsidev/react-turnstile'
import { useRef } from 'react'
import { config } from "@/config/env";


export function LoginForm({
  className,
  ...props
}: React.ComponentProps<"div">) {
  const router = useRouter()

  const [email, setEmail] = useState<string>("");
  const [password, setPassword] = useState<string>("");
  const turnstileRef = useRef<TurnstileInstance | null>(null)

  return (
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
              <Field>
                <FieldLabel htmlFor="email">Email</FieldLabel>
                <Input
                  id="email"
                  type="email"
                  placeholder="m@example.com"
                  required
                  onChange={(e)=> setEmail(e.target.value)}
                />
              </Field>
              <Field>
                <div className="flex items-center">
                  <FieldLabel htmlFor="password">Password</FieldLabel>
                  <a
                    href="#"
                    onClick={(e) => { e.preventDefault(); router.push('/forgot-password'); }}
                    className="ml-auto text-sm underline-offset-4 hover:underline"
                  >
                    Forgot your password?
                  </a>
                </div>
                <Input id="password"
                       type="password"
                       required
                       onChange={(e)=> setPassword(e.target.value)}
                />
              </Field>
              <Turnstile
                ref={turnstileRef}
                siteKey={config.turnstileSitekey}
              />
              <Field>
                <Button type="submit" onClick={(e)=> {
                  e.preventDefault()

                  const token = turnstileRef.current?.getResponse()
                  if (!token) {
                    throw new Error("turnstile verification error")
                  }

                  signIn(email, password, token)
                      .then(() => {
                        console.log("Sign in successful");
                        router.push("/drive");
                      })
                      .catch((err: Error) => {
                        console.error(`Error occured on sign in: ${err.message}`);
                      })
                      .finally(() => {
                        turnstileRef.current?.reset() // Reset after submission
                      })
                }}>Login</Button>
                <FieldDescription className="text-center">
                  New to Quartz? <a href="#" onClick={() => {router.push('/signup')}}>Create account</a>
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
