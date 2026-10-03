"use client";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
    Card,
    CardContent,
    CardDescription,
    CardHeader,
    CardTitle,
} from "@/components/ui/card";
import * as opaque from "@serenity-kit/opaque";
import { Input } from "@/components/ui/input";
import { useRouter } from "next/navigation";
import { useState, useEffect } from "react";
import {
    registerKeyMaterial,
    initializeAccountKeysToServer,
    KeyRegisterMaterial,
} from "@/KeyRegisterMaterial";
import {
    getOpaqueExportKey,
    getUserEmail,
    setAccountPrivateKeys,
    setOpaqueInitData,
} from "@/lib/authStore";
import { generateDeviceKeyPair, bufferToBase64 } from "@/signin";
import { saveDevicePrivateKey } from "@/DeviceKeyStore";
import { customFetch } from "@/lib/api";
import { config } from "@/config/env";
import { refreshSession } from "@/refresh";
import {
    ShieldCheck,
    Mail,
    KeyRound,
    AlertTriangle,
    Download,
    Printer,
} from "lucide-react";
import { signOut } from "@/signout";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "./ui/field";
import { getSodium } from "@/lib/crypto/sodium";

export function OnboardingView({
    className,
    ...props
}: React.ComponentProps<"div">) {
    const router = useRouter();

    // UX Steps:
    // 0: Initializing (Generating keys locally)
    // 1: Recovery Overview
    // 2: Display Phrase
    // 3: Verify Phrase
    // 4: Uploading
    // 5: Success
    const [step, setStep] = useState<number>(0);

    const [error, setError] = useState<string>("");
    const [reauthError, setReauthError] = useState<string>("");
    const [keyMaterial, setKeyMaterial] = useState<KeyRegisterMaterial | null>(
        null
    );
    const [verifyPhraseInput, setVerifyPhraseInput] = useState<string>("");
    const [verifyError, setVerifyError] = useState<string>("");
    const [password, setPassword] = useState<string>("");

    // Step 0: Generate keys automatically on mount
    const generateKeys = async () => {
        try {
            const email = getUserEmail();
            const exportKey = getOpaqueExportKey();

            if (!email || !exportKey) {
                throw new Error(`Your encryption keys are not yet initialized.
          To generate them, please enter your password.
          `);
            }

            console.log("registering key material");
            const km = await registerKeyMaterial(
                email,
                exportKey as string | Uint8Array
            );
            setKeyMaterial(km);
            setStep(1); // Move to overview
        } catch (err: any) {
            setError(err.message || "Failed to generate keys");
        }
    };

    useEffect(() => {
        if (step === 0) {
            generateKeys();
        }
    }, [step]);

    const handleDownload = () => {
        if (!keyMaterial) return;
        const element = document.createElement("a");
        const file = new Blob([keyMaterial.recoveryPhrase], {
            type: "text/plain",
        });
        element.href = URL.createObjectURL(file);
        element.download = "Quartz_Recovery_Phrase.txt";
        document.body.appendChild(element);
        element.click();
        document.body.removeChild(element);
    };

    const handlePrint = () => {
        if (!keyMaterial) return;
        const printWindow = window.open("", "", "width=600,height=400");
        if (printWindow) {
            printWindow.document.write(
                `<html><body><h2>Quartz Recovery Phrase</h2><p style="font-size: 24px; font-family: monospace;">${keyMaterial.recoveryPhrase}</p><p>Keep this safe!</p></body></html>`
            );
            printWindow.document.close();
            printWindow.print();
        }
    };

    const handleVerify = async () => {
        if (!keyMaterial) return;
        if (verifyPhraseInput.trim() !== keyMaterial.recoveryPhrase.trim()) {
            setVerifyError(
                "The phrase you entered does not match. Please try again."
            );
            return;
        }
        setVerifyError("");
        setStep(4); // Uploading

        try {
            const email = getUserEmail();
            if (!email) throw new Error("Missing email");

            // Upload to server
            await initializeAccountKeysToServer(keyMaterial, email, "", "");

            // Save keys to memory and register device
            console.log(
                "Setting account private keys in JS memory:",
                keyMaterial.rawAccountEncryptionPrivateKey,
                keyMaterial.rawAccountSigningPrivateKey
            );
            setAccountPrivateKeys(
                keyMaterial.rawAccountEncryptionPrivateKey,
                keyMaterial.rawAccountSigningPrivateKey
            );

            const deviceKeyPair = await generateDeviceKeyPair();
            const combinedKeys = new Uint8Array([
                ...keyMaterial.rawAccountSigningPrivateKey,
                ...keyMaterial.rawAccountEncryptionPrivateKey,
            ]);

            const pubKeyBuffer = Uint8Array.from(
                atob(deviceKeyPair.devicePublicKey),
                (c) => c.charCodeAt(0)
            );
            const importedPubKey = await crypto.subtle.importKey(
                "spki",
                pubKeyBuffer,
                { name: "RSA-OAEP", hash: "SHA-256" },
                true,
                ["encrypt"]
            );
            const wrappedAccountKeysBuffer = await crypto.subtle.encrypt(
                { name: "RSA-OAEP" },
                importedPubKey,
                combinedKeys
            );
            const wrappedAccountKeys = bufferToBase64(wrappedAccountKeysBuffer);

            await saveDevicePrivateKey(deviceKeyPair.devicePrivateKey);

            await customFetch(`${config.apiUrl}/v1/devices/register`, {
                method: "POST",
                body: JSON.stringify({
                    devicePublicKey: deviceKeyPair.devicePublicKey,
                    wrappedAccountKeys: wrappedAccountKeys,
                }),
                credentials: "include",
            });

            // Force a session refresh to get an updated JWT with keysInitialized=true
            await refreshSession();

            setStep(5); // Success
        } catch (err: any) {
            setError(err.message || "Failed to upload keys to server.");
            setStep(3); // Go back to verify step so they can try again
        }
    };

    if (error) {
        return (
            <Card className={cn("w-full", className)} {...props}>
                <CardHeader className="text-center">
                    <CardTitle className="text-xl text-destructive flex items-center justify-center gap-2">
                        <AlertTriangle className="size-5" /> Reauthentication
                        required
                    </CardTitle>
                    <CardDescription>{error}</CardDescription>
                </CardHeader>
                <CardContent>
                    <form
                        action="#"
                        method="POST"
                        className="flex flex-col w-full gap-4"
                    >
                        <FieldGroup>
                            <Field className={"text-center"}>
                                <FieldLabel htmlFor="password">
                                    Password
                                </FieldLabel>
                                <Input
                                    id="password"
                                    type="password"
                                    autoComplete="current-password"
                                    required
                                    onChange={(e) =>
                                        setPassword(e.target.value)
                                    }
                                />
                                <FieldDescription
                                    className={"text-destructive"}
                                >
                                    {reauthError}
                                </FieldDescription>
                            </Field>
                            <Field>
                                <Button
                                    type="submit"
                                    onClick={(e) => {
                                        e.preventDefault();

                                        let _password = password;

                                        reAuthenticate(_password)
                                            .then(() => {
                                                console.log(
                                                    "Reauth successful!"
                                                );
                                                setError("");
                                                setReauthError("");
                                                setPassword("");
                                                generateKeys();
                                            })
                                            .catch((err: Error) => {
                                                console.error(
                                                    `Error occured on sign in: ${err.message}`
                                                );
                                                setReauthError(err.message);
                                            });
                                    }}
                                >
                                    Continue
                                </Button>
                            </Field>
                        </FieldGroup>
                    </form>
                </CardContent>
            </Card>
        );
    }

    if (step === 0) {
        return (
            <Card className={cn("w-full text-center", className)} {...props}>
                <CardHeader>
                    <CardTitle className="text-xl">
                        Preparing your encryption keys...
                    </CardTitle>
                    <CardDescription>
                        We are generating your encryption keys locally on your
                        device.
                    </CardDescription>
                </CardHeader>
                <CardContent className="flex justify-center py-8">
                    <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
                </CardContent>
            </Card>
        );
    }

    if (step === 1) {
        return (
            <Card className={cn("w-full", className)} {...props}>
                <CardHeader>
                    <CardTitle className="text-xl">
                        Add your recovery methods
                    </CardTitle>
                    <CardDescription>
                        These methods will help you regain access to your
                        account and files if you forget your password.
                    </CardDescription>
                </CardHeader>
                <CardContent className="flex flex-col gap-6">
                    <div className="flex flex-col gap-2">
                        <h3 className="font-semibold text-sm">
                            Account Recovery
                        </h3>
                        <p className="text-xs text-muted-foreground mb-2">
                            Can reset your password, but won't recover your
                            files.
                        </p>
                        <div className="flex items-center justify-between p-4 border rounded-lg bg-muted/50">
                            <div className="flex items-center gap-3">
                                <Mail className="size-5 text-muted-foreground" />
                                <div className="flex flex-col">
                                    <span className="text-sm font-medium">
                                        Email Recovery
                                    </span>
                                    <span className="text-xs text-muted-foreground">
                                        Enabled by default (cannot be turned
                                        off)
                                    </span>
                                </div>
                            </div>
                            <span className="text-xs font-semibold text-green-600 bg-green-100 px-2 py-1 rounded-full dark:bg-green-900/30 dark:text-green-400">
                                Enabled
                            </span>
                        </div>
                    </div>

                    <div className="flex flex-col gap-2">
                        <h3 className="font-semibold text-sm">Data Recovery</h3>
                        <p className="text-xs text-muted-foreground mb-2">
                            The only way to recover your files if you forget
                            your password.
                        </p>
                        <div className="flex items-center justify-between p-4 border rounded-lg">
                            <div className="flex items-center gap-3">
                                <KeyRound className="size-5 text-primary" />
                                <div className="flex flex-col">
                                    <span className="text-sm font-medium">
                                        Recovery Phrase{" "}
                                        <span className="text-destructive">
                                            *
                                        </span>
                                    </span>
                                    <span className="text-xs text-muted-foreground">
                                        A 12-word secret phrase
                                    </span>
                                </div>
                            </div>
                            <Button size="sm" onClick={() => setStep(2)}>
                                Set up
                            </Button>
                        </div>
                        <button
                            className={
                                "mt-3 text-xs cursor-pointer text-muted-foreground hover:underline"
                            }
                            onClick={async () => {
                                await signOut();
                                window.location.href = "/signin";
                            }}
                        >
                            Sign out
                        </button>
                    </div>
                </CardContent>
            </Card>
        );
    }

    if (step === 2) {
        return (
            <Card className={cn("w-full", className)} {...props}>
                <CardHeader>
                    <CardTitle className="text-xl">
                        Save your Recovery Phrase
                    </CardTitle>
                    <CardDescription>
                        This is your 12-word recovery phrase. In case you get
                        locked out, you can use it to securely recover your
                        data. In the next step we will ask you for it, to ensure
                        you have it saved properly.
                    </CardDescription>
                </CardHeader>
                <CardContent className="flex flex-col gap-6">
                    <div className="bg-muted p-4 rounded-lg flex flex-wrap gap-2 justify-center border">
                        {keyMaterial?.recoveryPhrase
                            .split(" ")
                            .map((word, i) => (
                                <span
                                    key={i}
                                    className="bg-background px-3 py-1 rounded-md text-sm font-mono border"
                                >
                                    <span className="text-muted-foreground text-xs mr-2">
                                        {i + 1}.
                                    </span>
                                    {word}
                                </span>
                            ))}
                    </div>

                    <div className="flex gap-4 justify-center">
                        <Button
                            variant="outline"
                            size="sm"
                            onClick={handleDownload}
                        >
                            <Download className="size-4 mr-2" /> Download
                        </Button>
                        <Button
                            variant="outline"
                            size="sm"
                            onClick={handlePrint}
                        >
                            <Printer className="size-4 mr-2" /> Print
                        </Button>
                    </div>

                    <Button className="w-full mt-4" onClick={() => setStep(3)}>
                        I have saved my recovery phrase
                    </Button>
                </CardContent>
            </Card>
        );
    }

    if (step === 3) {
        return (
            <Card className={cn("w-full", className)} {...props}>
                <CardHeader>
                    <CardTitle className="text-xl">
                        Verify Recovery Phrase
                    </CardTitle>
                    <CardDescription>
                        Please type in your recovery phrase to ensure it's valid
                        and properly saved.
                    </CardDescription>
                </CardHeader>
                <CardContent className="flex flex-col gap-4">
                    <Input
                        placeholder="Type your 12-word phrase here..."
                        value={verifyPhraseInput}
                        onChange={(e) => setVerifyPhraseInput(e.target.value)}
                    />
                    {verifyError && (
                        <p className="text-sm text-destructive">
                            {verifyError}
                        </p>
                    )}
                    <div className="flex gap-4 mt-2">
                        <Button variant="ghost" onClick={() => setStep(2)}>
                            Back
                        </Button>
                        <Button className="flex-1" onClick={handleVerify}>
                            Verify & Continue
                        </Button>
                    </div>
                </CardContent>
            </Card>
        );
    }

    if (step === 4) {
        return (
            <Card className={cn("w-full text-center", className)} {...props}>
                <CardHeader>
                    <CardTitle className="text-xl">Updating keys...</CardTitle>
                    <CardDescription>
                        Setting up your recovery methods on the server.
                    </CardDescription>
                </CardHeader>
                <CardContent className="flex justify-center py-8">
                    <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
                </CardContent>
            </Card>
        );
    }

    if (step === 5) {
        return (
            <Card className={cn("w-full text-center", className)} {...props}>
                <CardHeader>
                    <div className="flex justify-center mb-4">
                        <ShieldCheck className="size-16 text-green-500" />
                    </div>
                    <CardTitle className="text-2xl">Account Secured!</CardTitle>
                    <CardDescription>
                        Your encryption keys and recovery phrase have been
                        successfully registered.
                    </CardDescription>
                </CardHeader>
                <CardContent>
                    <Button
                        className="w-full"
                        size="lg"
                        onClick={() => router.push("/drive")}
                    >
                        Finish setting up your account
                    </Button>
                </CardContent>
            </Card>
        );
    }

    return null;
}

export async function reAuthenticate(password: string) {
    if (!password) throw new Error("Password is required to reauthenticate");

    const sodium = await getSodium();
    await opaque.ready;

    const { clientLoginState, startLoginRequest } = opaque.client.startLogin({
        password,
    });

    const m1 = {
        loginRequest: startLoginRequest,
    };

    // send opaque m1 and fetch m2 from response
    const res = await customFetch(`${config.apiUrl}/v1/account/reauth`, {
        method: "POST",
        body: JSON.stringify(m1),
    });

    const m2 = await res.json();

    let loginResponse: string;
    let email: string;

    if (m2.loginResponse && m2.loginResponse != "") {
        loginResponse = m2.loginResponse;
        email = m2.email;
    } else {
        throw Error("Something went wrong on our servers.");
    }

    const loginResult = opaque.client.finishLogin({
        clientLoginState,
        loginResponse,
        password,
    });
    if (!loginResult) {
        throw new Error("Password is invalid");
    }

    const { exportKey, serverStaticPublicKey } = loginResult;

    if (serverStaticPublicKey !== process.env.NEXT_PUBLIC_SERVER_PUBLIC_KEY) {
        throw new Error("Server identity verification failed. Aborting login.");
    }

    setOpaqueInitData(exportKey, email);
}
