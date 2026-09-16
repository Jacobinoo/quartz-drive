"use client";

import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { AlertCircle } from "lucide-react";
import { getAccountEncryptionPrivateKey, getOpaqueExportKey, getUserEmail, setAccountPrivateKeys } from "@/lib/authStore";
import { customFetch } from "@/lib/api";
import { getSodium } from "@/lib/crypto/sodium";
import { saveDevicePrivateKey } from "@/DeviceKeyStore";
import { config } from "@/config/env";
import { generateDeviceKeyPair, bufferToBase64 } from "@/signin";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { useRouter } from "next/navigation";

export function RecoveryBanner() {
  const [needsRecovery, setNeedsRecovery] = useState(false);
  const [open, setOpen] = useState(false);
  const [phrase, setPhrase] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [missingExportKey, setMissingExportKey] = useState(false);
  const [success, setSuccess] = useState(false);
  const router = useRouter();

  useEffect(() => {
    // If we are logged in but missing the account key, we need recovery
    const key = getAccountEncryptionPrivateKey();
    if (!key) {
      // Small delay to ensure auth store is populated if we just logged in
      setTimeout(() => {
        if (!getAccountEncryptionPrivateKey()) {
          setNeedsRecovery(true);
          if (!getOpaqueExportKey()) {
              setMissingExportKey(true);
          }
        }
      }, 100);
    } else {
      setNeedsRecovery(false);
    }
  }, []);

  if (!needsRecovery) return null;

  async function handleRecover() {
    setError("");
    setLoading(true);

    try {
      let email = getUserEmail();
      let exportKey = getOpaqueExportKey();

      if (!exportKey || !email) {
          if (!password) {
              throw new Error("Missing authentication context. Please provide your password.");
          }
          // Perform Reauthentication to get the exportKey
          const opaque = await import("@serenity-kit/opaque");
          await opaque.ready;
          const { clientLoginState, startLoginRequest } = opaque.client.startLogin({ password });

          const reauthRes = await customFetch(`${config.apiUrl}/v1/account/reauth`, {
              method: "POST",
              body: JSON.stringify({ loginRequest: startLoginRequest }),
              credentials: "include"
          });

          if (!reauthRes.ok) {
              throw new Error("Invalid password. Reauthentication failed.");
          }
          const m2 = await reauthRes.json();
          email = m2.email;

          const loginResult = opaque.client.finishLogin({
              clientLoginState,
              loginResponse: m2.loginResponse,
              password
          });

          if (!loginResult) {
              throw new Error("Failed to finish login during reauthentication");
          }
          exportKey = loginResult.exportKey;
      }

      // 1. Fetch encrypted keys from server
      const keysRes = await customFetch(`${config.apiUrl}/v1/account/keys`);
      if (!keysRes.ok) {
        throw new Error("Failed to fetch account keys from server.");
      }
      const keys = await keysRes.json();

      const sodium = await getSodium();
      const oldMasterSalt = sodium.from_base64(keys.masterKdfSalt);

      // 2. Derive the recovery key using the phrase and old master salt
      const derivedRecoveryKey = sodium.crypto_pwhash(
        sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES,
        phrase.trim(),
        oldMasterSalt,
        sodium.crypto_pwhash_OPSLIMIT_INTERACTIVE,
        sodium.crypto_pwhash_MEMLIMIT_INTERACTIVE,
        sodium.crypto_pwhash_ALG_ARGON2ID13
      );

      // 3. Decrypt the account private keys
      let accountSigningPrivateKey: Uint8Array;
      let accountEncryptionPrivateKey: Uint8Array;

      try {
          accountSigningPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
            null,
            sodium.from_base64(keys.recoveryEncAccountSigningPrivateKey),
            sodium.from_string(email.toLowerCase() + "_sign"),
            sodium.from_base64(keys.recoveryAccountSigningKeyNonce),
            derivedRecoveryKey
          );

          accountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
            null,
            sodium.from_base64(keys.recoveryEncAccountEncryptionPrivateKey),
            sodium.from_string(email.toLowerCase() + "_encrypt"),
            sodium.from_base64(keys.recoveryAccountEncryptionKeyNonce),
            derivedRecoveryKey
          );
      } catch (err) {
          throw new Error("Invalid Recovery Phrase. Please check your spelling and order.");
      }

      // 4. Re-derive the NEW master key from the exportKey
      const exportKeyBytes = typeof exportKey === "string" ? sodium.from_string(exportKey) : exportKey;
      const kdfRootKey = sodium.crypto_generichash(
          sodium.crypto_kdf_KEYBYTES,
          exportKeyBytes,
          null
      );
      const newDerivedMasterKey = sodium.crypto_kdf_derive_from_key(
          sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES,
          1,
          "QMaster!",
          kdfRootKey
      );

      // 5. Re-encrypt the keys with the NEW master key
      const newAccountSigningPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
      const newEncryptedAccountSigningPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
          accountSigningPrivateKey,
          sodium.from_string(email.toLowerCase()+"_sign"),
          null,
          newAccountSigningPrivNonce,
          newDerivedMasterKey
      );

      const newAccountEncryptionPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
      const newEncryptedAccountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
          accountEncryptionPrivateKey,
          sodium.from_string(email.toLowerCase()+"_encrypt"),
          null,
          newAccountEncryptionPrivNonce,
          newDerivedMasterKey
      );

      // 6. Sign the payload using AccountSigningPrivateKey
      const encAccountEncryptionPrivateKey = sodium.to_base64(newEncryptedAccountEncryptionPrivateKey);
      const accountEncryptionKeyNonce = sodium.to_base64(newAccountEncryptionPrivNonce);
      const encAccountSigningPrivateKey = sodium.to_base64(newEncryptedAccountSigningPrivateKey);
      const accountSigningKeyNonce = sodium.to_base64(newAccountSigningPrivNonce);

      const payloadObj = {
          encAccountEncryptionPrivateKey,
          accountEncryptionKeyNonce,
          encAccountSigningPrivateKey,
          accountSigningKeyNonce
      };

      const signedPayload = encAccountEncryptionPrivateKey + accountEncryptionKeyNonce + encAccountSigningPrivateKey + accountSigningKeyNonce;
      const payloadBytes = sodium.from_string(signedPayload);
      const signature = sodium.crypto_sign_detached(payloadBytes, accountSigningPrivateKey);

      const payloadObjWithSignature = {
          ...payloadObj,
          signatureHex: sodium.to_hex(signature)
      };

      // 7. Update keys on the server
      const updateRes = await customFetch(`${config.apiUrl}/v1/account/keys`, {
          method: "PUT",
          body: JSON.stringify(payloadObjWithSignature)
      });

      if (!updateRes.ok) {
          throw new Error("Failed to update keys on the server.");
      }

      // 8. Device Registration (skipped during signin)
      const deviceKeyPair = await generateDeviceKeyPair();
      const combinedKeys = new Uint8Array([...accountSigningPrivateKey, ...accountEncryptionPrivateKey]);
      const pubKeyBuffer = Uint8Array.from(atob(deviceKeyPair.devicePublicKey), c => c.charCodeAt(0));
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
              wrappedAccountKeys: wrappedAccountKeys
          }),
          credentials: "include",
      });

      // 9. Finalize!
      setAccountPrivateKeys(accountEncryptionPrivateKey, accountSigningPrivateKey);
      setSuccess(true);
    } catch (err: any) {
      setError(err.message || "An unexpected error occurred.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <div className="w-full bg-destructive/15 text-destructive px-4 py-2 flex items-center justify-between border-b border-destructive/20 text-sm">
        <div className="flex items-center gap-2">
          <AlertCircle className="h-4 w-4" />
          <span>Your data is locked. You recently reset your password and need to recover your encryption keys to view your files.</span>
        </div>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button variant="destructive" size="sm">Recover Data</Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{success ? "Data Unlocked" : "Unlock Your Data"}</DialogTitle>
              <DialogDescription>
                {success 
                  ? "Your encryption keys have been successfully recovered and linked to your new password." 
                  : "Enter your 12-word Recovery Phrase to decrypt your files and re-link them to your new password."}
              </DialogDescription>
            </DialogHeader>
            <div className="py-4">
              {success ? (
                  <div className="flex flex-col items-center justify-center gap-4 py-6">
                    <div className="w-12 h-12 rounded-full bg-green-500/20 flex items-center justify-center text-green-500">
                      <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="lucide lucide-check"><path d="M20 6 9 17l-5-5"/></svg>
                    </div>
                    <p className="text-center text-sm text-muted-foreground">
                      Your files are now accessible.
                    </p>
                    <Button className="w-full mt-2" onClick={() => window.location.reload()}>
                      Continue to Drive
                    </Button>
                  </div>
              ) : (
                <form action="#" method="POST" onSubmit={(e) => { e.preventDefault(); handleRecover(); }}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="phrase">Recovery Phrase</FieldLabel>
                    <Input
                      id="phrase"
                      type="text"
                      placeholder="apple banana cherry..."
                      value={phrase}
                      onChange={e => setPhrase(e.target.value)}
                      required
                    />
                  </Field>
                  {missingExportKey && (
                      <Field>
                        <FieldLabel htmlFor="password">Current Password</FieldLabel>
                        <Input
                          id="password"
                          type="password"
                          placeholder="Your current password"
                          value={password}
                          onChange={e => setPassword(e.target.value)}
                          required
                        />
                      </Field>
                  )}
                  {error && <div className="text-red-500 text-sm">{error}</div>}
                  <Button type="submit" disabled={loading || !phrase.trim() || (missingExportKey && !password.trim())}>
                    {loading ? "Decrypting..." : "Unlock Data"}
                  </Button>
                </FieldGroup>
              </form>
              )}
            </div>
          </DialogContent>
        </Dialog>
      </div>
    </>
  );
}
