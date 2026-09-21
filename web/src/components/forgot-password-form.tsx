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
import {useState, useRef, useEffect} from "react";
import { Turnstile } from '@marsidev/react-turnstile'
import type { TurnstileInstance } from '@marsidev/react-turnstile'
import { KeyRound, Mail } from "lucide-react";
import { Textarea } from "./ui/textarea";
import { getSodium } from "@/lib/crypto/sodium";
import * as opaque from '@serenity-kit/opaque';
type RecoveryMethod = 'EMAIL' | 'RECOVERY_PHRASE' | null

export function ForgotPasswordForm({
  className,
  ...props
}: React.ComponentProps<"div">) {
  const router = useRouter();

  const [method, setMethod] = useState<RecoveryMethod>(null)
  const [recoveryPhrase, setRecoveryPhrase] = useState<string>("")
  const [phraseStep, setPhraseStep] = useState<'ENTER_PHRASE' | 'SET_PASSWORD' | 'SUCCESS'>('ENTER_PHRASE')
  const [newPassword, setNewPassword] = useState("")
  const [sessionID, setSessionID] = useState("")
  const [encryptedKeys, setEncryptedKeys] = useState<any>(null)
  const [recoveryKey, setRecoveryKey] = useState<Uint8Array | null>(null)

  const [email, setEmail] = useState<string>("");
  const [recoveredEmail, setRecoveredEmail] = useState<string>("");
  const [passwordScore, setPasswordScore] = useState<number | null>(null);
  const [passwordFeedback, setPasswordFeedback] = useState<string>("");
  const [submitted, setSubmitted] = useState<boolean>(false);
  const [error, setError] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [countdown, setCountdown] = useState<number>(0);
  const turnstileRef = useRef<TurnstileInstance | null>(null)

  const handlePhraseRecoveryStart = async () => {
    try {
      setError("");
      setLoading(true);
      const turnstileToken = turnstileRef.current?.getResponse();
      if (!turnstileToken) {
        throw new Error("Please complete the security check.");
      }

      const sodium = await getSodium();
      const recoveryIdHash = sodium.crypto_generichash(32, sodium.from_string(recoveryPhrase.trim().toLowerCase()), null);
      const recoveryIdHex = sodium.to_hex(recoveryIdHash);

      const res = await fetch(`${config.apiUrl}/v1/recovery/start`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-Verify-Token": turnstileToken },
        body: JSON.stringify({ method: "phrase", recovery_id_hex: recoveryIdHex }),
      });

      if (!res.ok) {
        const body = await res.json();
        throw new Error(body.Message || body.message || "Failed to find account");
      }

      const data = await res.json();
      setSessionID(data.session_id);
      setEncryptedKeys(data.encrypted_keys);
      if (data.email) {
        setRecoveredEmail(data.email);
      }
      
      const masterSalt = sodium.from_base64(data.encrypted_keys.masterKdfSalt);
      const derivedRecoveryKey = sodium.crypto_pwhash(
        sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES,
        recoveryPhrase.trim().toLowerCase(),
        masterSalt,
        sodium.crypto_pwhash_OPSLIMIT_INTERACTIVE,
        sodium.crypto_pwhash_MEMLIMIT_INTERACTIVE,
        sodium.crypto_pwhash_ALG_ARGON2ID13
      );
      setRecoveryKey(derivedRecoveryKey);

      const accountSigningPrivNonce = sodium.from_base64(data.encrypted_keys.recoveryAccountSigningKeyNonce);
      const accountSigningPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
        null,
        sodium.from_base64(data.encrypted_keys.recoveryEncAccountSigningPrivateKey),
        sodium.from_base64(data.encrypted_keys.accountSigningPublicKey),
        accountSigningPrivNonce,
        derivedRecoveryKey
      );

      const challengeBytes = sodium.from_hex(data.challenge);
      const signature = sodium.crypto_sign_detached(challengeBytes, accountSigningPrivateKey);
      const signatureHex = sodium.to_hex(signature);

      const verifyRes = await fetch(`${config.apiUrl}/v1/recovery/session/${data.session_id}/verify`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ challenge_signature_hex: signatureHex }),
      });

      if (!verifyRes.ok) {
        throw new Error("Failed to verify recovery phrase ownership");
      }

      setPhraseStep('SET_PASSWORD');
    } catch (err: any) {
      setError(err.message || "An unexpected error occurred");
      turnstileRef.current?.reset();
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!newPassword || newPassword.trim() === "") {
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

  const handlePhraseRecoveryComplete = async () => {
    try {
      setError("");
      setLoading(true);

      const sodium = await getSodium();
      await opaque.ready;

      const { clientRegistrationState, registrationRequest } = opaque.client.startRegistration({
        password: newPassword,
      });

      const m1Res = await fetch(`${config.apiUrl}/v1/recovery/session/${sessionID}/opaque/m1`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ registrationRequest: registrationRequest }),
      });

      if (!m1Res.ok) throw new Error("Failed to register new password (M1)");
      const m2 = await m1Res.json();

      const { registrationRecord, exportKey, serverStaticPublicKey } = opaque.client.finishRegistration({
        clientRegistrationState,
        registrationResponse: m2.registrationResponse,
        password: newPassword,
      });

      if (serverStaticPublicKey !== process.env.NEXT_PUBLIC_SERVER_PUBLIC_KEY) {
        throw new Error("Server identity verification failed. Aborting password reset.");
      }

      // Re-encrypt keys using the new export key
      const accountEncryptionPrivNonce = sodium.from_base64(encryptedKeys.recoveryAccountEncryptionKeyNonce);
      const accountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
        null,
        sodium.from_base64(encryptedKeys.recoveryEncAccountEncryptionPrivateKey),
        sodium.from_base64(encryptedKeys.accountEncryptionPublicKey),
        accountEncryptionPrivNonce,
        recoveryKey!
      );

      const accountSigningPrivNonce = sodium.from_base64(encryptedKeys.recoveryAccountSigningKeyNonce);
      const accountSigningPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
        null,
        sodium.from_base64(encryptedKeys.recoveryEncAccountSigningPrivateKey),
        sodium.from_base64(encryptedKeys.accountSigningPublicKey),
        accountSigningPrivNonce,
        recoveryKey!
      );

      // Derive the NEW master key from the exportKey
      const exportKeyBytes = typeof exportKey === "string" ? sodium.from_string(exportKey) : exportKey;
      const kdfRootKey = sodium.crypto_generichash(
          sodium.crypto_kdf_KEYBYTES,
          exportKeyBytes,
          null
      );
      const derivedMasterKey = sodium.crypto_kdf_derive_from_key(
          sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES,
          1,
          "QMaster!",
          kdfRootKey
      );

      const newAccountEncryptionPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
      const newEncryptedAccountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        accountEncryptionPrivateKey,
        sodium.from_base64(encryptedKeys.accountEncryptionPublicKey),
        null,
        newAccountEncryptionPrivNonce,
        derivedMasterKey
      );

      const newAccountSigningPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
      const newEncryptedAccountSigningPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        accountSigningPrivateKey,
        sodium.from_base64(encryptedKeys.accountSigningPublicKey),
        null,
        newAccountSigningPrivNonce,
        derivedMasterKey
      );

      const completeRes = await fetch(`${config.apiUrl}/v1/recovery/session/${sessionID}/complete`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          new_opaque_record: registrationRecord,
          re_encrypted_keys: {
            masterKdfSalt: encryptedKeys.masterKdfSalt,
            accountEncryptionPublicKey: encryptedKeys.accountEncryptionPublicKey,
            encAccountEncryptionPrivateKey: sodium.to_base64(newEncryptedAccountEncryptionPrivateKey),
            accountEncryptionKeyNonce: sodium.to_base64(newAccountEncryptionPrivNonce),
            accountSigningPublicKey: encryptedKeys.accountSigningPublicKey,
            encAccountSigningPrivateKey: sodium.to_base64(newEncryptedAccountSigningPrivateKey),
            accountSigningKeyNonce: sodium.to_base64(newAccountSigningPrivNonce),
            
            // These stay the same (encrypted with recoveryKey)
            recoveryEncAccountEncryptionPrivateKey: encryptedKeys.recoveryEncAccountEncryptionPrivateKey,
            recoveryAccountEncryptionKeyNonce: encryptedKeys.recoveryAccountEncryptionKeyNonce,
            recoveryEncAccountSigningPrivateKey: encryptedKeys.recoveryEncAccountSigningPrivateKey,
            recoveryAccountSigningKeyNonce: encryptedKeys.recoveryAccountSigningKeyNonce,
          }
        }),
      });

      if (!completeRes.ok) throw new Error("Failed to finalize recovery");

      setPhraseStep('SUCCESS');

    } catch (err: any) {
      setError(err.message || "An unexpected error occurred");
    } finally {
      setLoading(false);
    }
  };

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

  switch (method) {

    case null: return (
      <div className={cn("flex flex-col gap-6", className)} {...props}>
        <Card>
          <CardHeader className="text-center">
            <CardTitle className="text-xl">Account & Data Recovery</CardTitle>
            <CardDescription>
              Choose a recovery method to regain access to your account.
            </CardDescription>
          </CardHeader>
          <CardContent className={"flex flex-col gap-3"}>
            <button className="flex items-center justify-between text-left p-4 border rounded-lg bg-muted/50 cursor-pointer hover:bg-gray-200/40 dark:hover:bg-muted/35" onClick={() => {setMethod('RECOVERY_PHRASE')}}>
              <div className="flex items-center gap-3">
                <KeyRound className="size-5 text-primary w-1/6" />
                <div className="flex flex-col gap-0.5">
                  <div className={"flex items-center justify-between"}>
                    <span className="text-sm font-medium">Recovery Phrase</span>
                    <span className="text-xs font-semibold text-blue-600 bg-blue-100 px-2 py-0.5 rounded-full dark:bg-blue-900/30 dark:text-blue-400">Recommended</span>
                  </div>
                  <span className="text-xs text-muted-foreground">Can be used for both resetting your password and recovering your data</span>
                </div>
              </div>
            </button>
            <button className="flex items-center justify-between text-left p-4 border rounded-lg bg-muted/50 cursor-pointer hover:bg-gray-200/40 dark:hover:bg-muted/35" onClick={() => {setMethod('EMAIL')}}>
              <div className="flex items-center gap-3">
                <Mail className="size-5 text-muted-foreground w-1/6" />
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium">Email Recovery</span>
                  <span className="text-xs text-muted-foreground">Can reset your password to allow you to sign in, but won't recover your data without the recovery phrase.</span>
                </div>
              </div>
            </button>
            <form action="#" method="POST">
              <Field>
                <FieldDescription className="text-center">
                  Remember your credentials? <a href="#" onClick={(e) => { e.preventDefault(); router.push('/signin') }}>Sign in</a>
                </FieldDescription>
              </Field>
            </form>
          </CardContent>
        </Card>
      </div>
    )
    case 'EMAIL':
      return (
        <div className={cn("flex flex-col gap-6", className)} {...props}>
          <Card>
            <CardHeader className="text-center">
              <CardTitle className="text-xl">Email Recovery</CardTitle>
              <CardDescription>
                We will send you an email with a magic link, click it to reset your password.
              </CardDescription>
              <p className={"text-xs px-3 py-1 bg-amber-500/20 text-amber-800 dark:bg-amber-900/20 dark:text-amber-500 rounded-md"}>Email recovery will allow you to regain access to your account, but you will still need a recovery phrase to recover your files! <a href="#" className={"underline"} onClick={() => {
                setMethod("RECOVERY_PHRASE")
              }}>Have a recovery phrase?</a></p>
            </CardHeader>
            <CardContent>
              <form action="#" method="POST">
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="email">Enter your email</FieldLabel>
                    <Input
                      id="email"
                      type="email"
                      autoComplete="email"
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

                      fetch(`${config.apiUrl}/v1/recovery/start`, {
                        method: "POST",
                        headers: { "Content-Type": "application/json", "X-Verify-Token": turnstileToken },
                        body: JSON.stringify({ method: "email", email })
                      })
                        .then((res) => {
                          if (res.ok) {
                            setSubmitted(true);
                            setCountdown(70);
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
                            const body = res.json()
                            body.then((d) => {
                              setError(`Failed to send reset link: ${d.Message || d.message || "Unknown error"}`);
                              turnstileRef.current?.reset();
                            })
                          }
                        })
                        .catch((err: Error) => {
                          setError("Failed to connect to server.");
                          turnstileRef.current?.reset();
                        })
                    }}>Send Recovery Link</Button>
                    <FieldDescription className="text-center">
                      <a href="#" onClick={(e) => { e.preventDefault(); setMethod(null) }}>Choose a different recovery method</a>
                    </FieldDescription>
                  </Field>
                </FieldGroup>
              </form>
            </CardContent>
          </Card>
        </div>
      )
    case 'RECOVERY_PHRASE':
      return (
        <div className={cn("flex flex-col gap-6", className)} {...props}>
          <Card>
            <CardHeader className="text-center">
              <CardTitle className="text-xl">Use your Recovery Phrase</CardTitle>
              <CardDescription>
                Recovery phrase consists of 24 words. Enter it below to reset your password and recover your data.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form action="#" method="POST">
                <FieldGroup>
                  {phraseStep === 'ENTER_PHRASE' && (
                    <>
                      <Field>
                        <FieldLabel htmlFor="phrase">Recovery Phrase</FieldLabel>
                        <Textarea
                          id="phrase"
                          placeholder="Type your 24-word phrase here..."
                          className={"resize-none"}
                          required
                          autoComplete="off"
                          value={recoveryPhrase}
                          onChange={(e) => setRecoveryPhrase(e.target.value)}
                        />
                      </Field>

                      <Field>
                        <Turnstile
                          ref={turnstileRef}
                          siteKey={config.turnstileSitekey}
                        />
                      </Field>
                    </>
                  )}

                  {error && <div className="text-red-500 text-sm text-center">{error}</div>}
                  {phraseStep === 'ENTER_PHRASE' && (
                    <>
                      <Field>
                        <Button type="submit" disabled={loading || !recoveryPhrase.trim()} onClick={(e) => {
                          e.preventDefault();
                          handlePhraseRecoveryStart();
                        }}>{loading ? "Verifying..." : "Continue"}</Button>
                        <FieldDescription className="text-center mt-2">
                          <a href="#" onClick={(e) => { e.preventDefault(); setMethod(null) }}>Choose a different recovery method</a>
                        </FieldDescription>
                      </Field>
                    </>
                  )}
                  
                  {phraseStep === 'SET_PASSWORD' && (
                    <>
                      {recoveredEmail && (
                        <div className="bg-muted text-muted-foreground text-sm p-3 rounded-md text-center mb-2 flex items-center justify-center gap-2">
                            <Mail className="w-4 h-4"/>
                            Recovering account: <span className="font-medium text-foreground">{recoveredEmail}</span>
                        </div>
                      )}
                      <Field>
                        <FieldLabel htmlFor="newPassword">New Password</FieldLabel>
                        <Input
                          id="newPassword"
                          type="password"
                          autoComplete="new-password"
                          required
                          value={newPassword}
                          onChange={(e) => setNewPassword(e.target.value)}
                        />
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
                      <Field>
                        <Button type="submit" disabled={loading || !newPassword.trim() || passwordScore === null || passwordScore < 3} onClick={(e) => {
                          e.preventDefault();
                          handlePhraseRecoveryComplete();
                        }}>{loading ? "Resetting Password..." : "Set New Password"}</Button>
                      </Field>
                    </>
                  )}

                  {phraseStep === 'SUCCESS' && (
                    <div className="flex flex-col items-center justify-center gap-4 py-6">
                      <div className="w-12 h-12 rounded-full bg-green-500/20 flex items-center justify-center text-green-500">
                        <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="lucide lucide-check"><path d="M20 6 9 17l-5-5"/></svg>
                      </div>
                      <p className="text-center text-sm text-muted-foreground">
                        Your password has been successfully reset.
                      </p>
                      <Button className="w-full mt-2" onClick={() => router.push('/signin')}>
                        Go to Sign In
                      </Button>
                    </div>
                  )}
                </FieldGroup>
              </form>
            </CardContent>
          </Card>
        </div>
      )
  }
}
