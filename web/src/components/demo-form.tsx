"use client";

import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Field,
  FieldDescription,
  FieldGroup,
} from "@/components/ui/field"
import {useRouter} from "next/navigation";
import React, { useState} from "react";
import { Turnstile } from '@marsidev/react-turnstile'
import type { TurnstileInstance } from '@marsidev/react-turnstile'
import { useRef } from 'react'
import { config } from "@/config/env";
import {demoStart} from "@/demo";


export function DemoForm({
  className,
  ...props
}: React.ComponentProps<"div">) {
  const router = useRouter()

  const turnstileRef = useRef<TurnstileInstance | null>(null)
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  return (
    <div className={cn("flex flex-col gap-6", className)} {...props}>
      <Card>
        <CardHeader className="text-center">
          <CardTitle className="text-xl">Live Demo</CardTitle>
        </CardHeader>
        <CardContent>
          <form action="#" method="POST">
            <FieldGroup>
              {error && (
                <div className="text-sm font-medium text-destructive text-center bg-destructive/10 p-2 rounded-md">
                  {error}
                </div>
              )}
              <Turnstile
                ref={turnstileRef}
                siteKey={config.turnstileSitekey}
              />
              <Field>
                <Button type="submit" disabled={isLoading} onClick={(e)=> {
                  e.preventDefault()
                  setError(null)
                  setIsLoading(true)

                  const token = turnstileRef.current?.getResponse()
                  if (!token) {
                    setIsLoading(false)
                    setError("turnstile verification error")
                    return
                  }

                  demoStart(token)
                    .then((result) => {
                      console.log("Sign in successful");
                      router.push("/drive");
                      })
                      .catch((err: Error) => {
                        console.error(`Error occured on sign in: ${err.message}`);
                        setError(err.message);
                      })
                      .finally(() => {
                        setIsLoading(false)
                        turnstileRef.current?.reset() // Reset after submission
                      })
                }}>
                  {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  {isLoading ? "Preparing demo account..." : "Start a demo session"}
                </Button>
              </Field>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>
      <FieldDescription className="px-6 text-center">
        By continuing, you agree to our <a href="/terms">Terms of Service</a> and <a href="/privacy">Privacy Policy</a>.
      </FieldDescription>
    </div>
  )
}
