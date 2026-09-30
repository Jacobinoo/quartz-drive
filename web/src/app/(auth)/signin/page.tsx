"use client";

import { Folder} from "lucide-react"

import {DemoForm} from "@/components/demo-form";

export default function LoginPage() {
  return (
    <>
      <div className="w-full bg-blue-500/90 text-blue-50 px-4 py-2 text-center text-sm font-medium flex items-center justify-center gap-2 fixed top-0 z-50">
        <span>This is a demo environment. Demo sessions can last for up to 1 hour and are automatically deleted. Don't upload sensitive or important data. Access can be revoked anytime.</span>
      </div>
      <div className="bg-muted flex min-h-svh flex-col items-center justify-center gap-6 p-6 md:p-10 pt-16">
        <div className="flex w-full max-w-sm flex-col gap-6">
        <a href="#" className="flex items-center gap-2 self-center font-medium">
          <div className="bg-primary text-primary-foreground flex size-6 items-center justify-center rounded-md">
            <Folder className="size-4" />
          </div>
          Quartz Drive
        </a>
        {/*<LoginForm />*/}
        <DemoForm />
      </div>
    </div>
    </>
  )
}
