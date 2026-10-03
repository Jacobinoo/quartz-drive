"use client";

import { useEffect, useState } from "react";
import { getAccessToken } from "@/lib/authStore";
import { signOut } from "@/signout";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";

export function UserMenu() {
    const [userEmail, setUserEmail] = useState("Loading...");

    useEffect(() => {
        const token = getAccessToken();
        if (token) {
            try {
                const payload = JSON.parse(atob(token.split(".")[1]));
                if (payload.email) setUserEmail(payload.email);
            } catch (e) {
                console.warn("Failed to decode token");
            }
        }
    }, []);

    const firstLetter =
        userEmail !== "Loading..." && userEmail
            ? userEmail.charAt(0).toUpperCase()
            : "?";
    const name =
        userEmail !== "Loading..."
            ? userEmail.split("@")[0].replace(".", " ")
            : "Loading...";

    return (
        <DropdownMenu>
            <DropdownMenuTrigger asChild>
                <div className="h-8 w-8 rounded-full bg-blue-600 text-white flex items-center justify-center font-semibold cursor-pointer hover:bg-blue-700 transition-colors select-none">
                    {firstLetter}
                </div>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
                <DropdownMenuLabel className="font-normal">
                    <div className="flex flex-col space-y-1">
                        <p className="text-sm font-medium leading-none capitalize">
                            {name}
                        </p>
                        <p className="text-xs leading-none text-muted-foreground">
                            {userEmail}
                        </p>
                    </div>
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                    onClick={async () => {
                        await signOut();
                        window.location.href = "/signin";
                    }}
                    className="text-red-500 focus:text-red-500 focus:bg-red-500/10 cursor-pointer"
                >
                    Sign Out
                </DropdownMenuItem>
            </DropdownMenuContent>
        </DropdownMenu>
    );
}
