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

export function SignupForm({
  className,
  ...props
}: React.ComponentProps<"div">) {
  const router = useRouter();
  const [email, setEmail] = useState<string>("");
  const [password, setPassword] = useState<string>("");

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
              <Field>
                <Button type="submit" onClick={(e) => {
                  e.preventDefault();
                  signUp(email, password)
                      .then(() => {
                        console.log("Sign up successfully");
                        router.push("/drive");
                      })
                      .catch((err: Error) => {
                        console.error(`Error occured on sign up: ${err.message}}`);
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