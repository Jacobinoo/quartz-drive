import { config } from "@/config/env";
import * as opaque from '@serenity-kit/opaque'
import { Base64String } from "@/UtilTypes";
import { getSodium } from "@/lib/crypto/sodium";
import { register } from "next/dist/next-devtools/userspace/pages/pages-dev-overlay-setup";

export type VerifyResponse = {
  email: string,
  recoveryKeys: {
 			masterKdfSalt: string,
			accountEncryptionPublicKey:string,
			accountSigningPublicKey:    string,

			recoveryEncAccountEncryptionPrivateKey: string,
			recoveryAccountEncryptionKeyNonce:  string,
			recoveryEncAccountSigningPrivateKey:   string,
			recoveryAccountSigningKeyNonce:          string,
  }
}

export async function verifyToken(token: string): Promise<VerifyResponse> {
  // Verify token and get recovery keys
  const verifyResponse = await fetch(`${config.apiUrl}/v1/account/verify-reset-code`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token })
  });

  if (!verifyResponse.ok) {
      throw new Error("Invalid or expired reset link");
  }

  const { recoveryKeys, email } = await verifyResponse.json();

  let verifyRes: VerifyResponse = {
    email, recoveryKeys
  }

  return verifyRes
}

async function decryptAccountKeys(verifyResponse: VerifyResponse, recoveryPhrase: string) {

  try {
    const sodium = await getSodium();
    const oldMasterSalt = sodium.from_base64(verifyResponse.recoveryKeys.masterKdfSalt);

    // 2. Derive the recovery key using the phrase and old master salt
    const derivedRecoveryKey = sodium.crypto_pwhash(
      sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES,
      recoveryPhrase.trim(),
      oldMasterSalt,
      sodium.crypto_pwhash_OPSLIMIT_INTERACTIVE,
      sodium.crypto_pwhash_MEMLIMIT_INTERACTIVE,
      sodium.crypto_pwhash_ALG_ARGON2ID13
    );

    // 3. Decrypt the account private keys
    let accountSigningPrivateKey: Uint8Array;
    let accountEncryptionPrivateKey: Uint8Array;

    accountSigningPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
      null,
      sodium.from_base64(verifyResponse.recoveryKeys.recoveryEncAccountSigningPrivateKey),
      sodium.from_string(verifyResponse.email.toLowerCase() + "_sign"),
      sodium.from_base64(verifyResponse.recoveryKeys.recoveryAccountSigningKeyNonce),
      derivedRecoveryKey
    );

    accountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
      null,
      sodium.from_base64(verifyResponse.recoveryKeys.recoveryEncAccountEncryptionPrivateKey),
      sodium.from_string(verifyResponse.email.toLowerCase() + "_encrypt"),
      sodium.from_base64(verifyResponse.recoveryKeys.recoveryAccountEncryptionKeyNonce),
      derivedRecoveryKey
    );

    return {
      accountEncryptionPrivateKey,
      accountSigningPrivateKey
    }

  } catch (e) {
    console.error(`Libsodium error: ${e}`)
    throw new Error("Recovery phrase is invalid")
  }
}

async function registerOpaque(newPassword: string, token: string) {
  await opaque.ready;

  const { clientRegistrationState, registrationRequest } = opaque.client.startRegistration({ password: newPassword });

  const m1Response = await fetch(`${config.apiUrl}/v1/account/reset-password/m1`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token, registrationRequest })
  });

  if (!m1Response.ok) {
      throw new Error("Failed to start password reset");
  }

  const { registrationResponse, nonce } = await m1Response.json();

  const { registrationRecord, exportKey, serverStaticPublicKey } = opaque.client.finishRegistration({
      clientRegistrationState,
      registrationResponse,
      password: newPassword,
  });

  if (serverStaticPublicKey !== process.env.NEXT_PUBLIC_SERVER_PUBLIC_KEY) {
      throw new Error("Server identity verification failed. Aborting login.");
  }

  return {
    registrationRecord, exportKey, nonce
  }
}

async function reEncryptKeyMaterial(exportKey: string, accountSigningPrivateKey: Uint8Array<ArrayBufferLike>, accountEncryptionPrivateKey: Uint8Array<ArrayBufferLike>, verifyResponse: VerifyResponse, recoveryPhrase: string) {
  const sodium = await getSodium();

  try {
    // Derive new Master Key from OPAQUE exportKey via fast KDF
    const exportKeyBytes = typeof exportKey === "string" ? sodium.from_string(exportKey) : exportKey;
    const kdfRootKey = sodium.crypto_generichash(
        sodium.crypto_kdf_KEYBYTES,
        exportKeyBytes,
        null
    );

    const newDerivedMasterKey = sodium.crypto_kdf_derive_from_key(
        sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES,
        1,              // Subkey ID 1
        "QMaster!",     // 8-byte context string
        kdfRootKey
    );

    // We still generate a random salt because the Recovery Phrase
    // requires the slow Argon2id algorithm.
    const newMasterSalt = sodium.randombytes_buf(sodium.crypto_pwhash_SALTBYTES);

    // 6. Re-encrypt the Account Private Keys with the NEW Master Key
    const newAccountSigningPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const newEncryptedAccountSigningPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        accountSigningPrivateKey,
        sodium.from_string(verifyResponse.email.toLowerCase()+"_sign"),
        null,
        newAccountSigningPrivNonce,
        newDerivedMasterKey
    );

    const newAccountEncryptionPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const newEncryptedAccountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        accountEncryptionPrivateKey,
        sodium.from_string(verifyResponse.email.toLowerCase()+"_encrypt"),
        null,
        newAccountEncryptionPrivNonce,
        newDerivedMasterKey
    );

    // 6.5. Re-derive Recovery Key with NEW Master Salt and re-encrypt Recovery Keys
    const newDerivedRecoveryKey = sodium.crypto_pwhash(
        sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES,
        recoveryPhrase.trim(),
        newMasterSalt,
        sodium.crypto_pwhash_OPSLIMIT_INTERACTIVE,
        sodium.crypto_pwhash_MEMLIMIT_INTERACTIVE,
        sodium.crypto_pwhash_ALG_ARGON2ID13
    );

    const newRecoveryAccountSigningPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const newRecoveryEncryptedAccountSigningPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        accountSigningPrivateKey,
        sodium.from_string(verifyResponse.email.toLowerCase()+"_sign"),
        null,
        newRecoveryAccountSigningPrivNonce,
        newDerivedRecoveryKey
    );

    const newRecoveryAccountEncryptionPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const newRecoveryEncryptedAccountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        accountEncryptionPrivateKey,
        sodium.from_string(verifyResponse.email.toLowerCase()+"_encrypt"),
        null,
        newRecoveryAccountEncryptionPrivNonce,
        newDerivedRecoveryKey
    );

    return {
      newMasterSalt,
      newAccountSigningPrivNonce,
      newAccountEncryptionPrivNonce,
      newDerivedRecoveryKey,
      newRecoveryAccountSigningPrivNonce,
      newRecoveryAccountEncryptionPrivNonce,
      newEncryptedAccountEncryptionPrivateKey,
      newEncryptedAccountSigningPrivateKey,
      newRecoveryEncryptedAccountEncryptionPrivateKey,
      newRecoveryEncryptedAccountSigningPrivateKey
    }

  } catch (e) {
    console.error(`Libsodium re-encryption error: ${e}`)
    throw new Error("Something went wrong during password reset. Try again or contact support.")
  }
}

async function sendM3(m3: M3) {
  try {
    const m3Response = await fetch(`${config.apiUrl}/v1/account/reset-password/m3`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(m3)
    });

    if (!m3Response.ok) {
      console.error("something went wrong with m3Response")
        throw new Error("Failed to finalize password reset");
    }
    return true
  } catch (e) {
    console.error(e)
    return false
  }
}

type M3 = {
  token: string,
  user: {
      aPAKE: {
          registrationRecord: string,
          registrationNonce: string,
      },
      keys: {
          masterKdfSalt: string,

          accountEncryptionPublicKey: string,
          encAccountEncryptionPrivateKey: string,
          accountEncryptionKeyNonce: string,

          accountSigningPublicKey: string,
          encAccountSigningPrivateKey: string,
          accountSigningKeyNonce: string,

          recoveryEncAccountEncryptionPrivateKey:string,
          recoveryAccountEncryptionKeyNonce: string,
          recoveryEncAccountSigningPrivateKey: string,
          recoveryAccountSigningKeyNonce: string,
      }
  }
}

export async function resetPassword(verifyResponse: VerifyResponse, token: string, recoveryPhrase: string, newPassword: string) {
  const { accountEncryptionPrivateKey, accountSigningPrivateKey } = await decryptAccountKeys(verifyResponse, recoveryPhrase)

  const { registrationRecord, exportKey, nonce } = await registerOpaque(newPassword, token)

  const {
  newMasterSalt,
  newAccountSigningPrivNonce,
  newAccountEncryptionPrivNonce,
  newDerivedRecoveryKey,
  newRecoveryAccountSigningPrivNonce,
    newRecoveryAccountEncryptionPrivNonce,
    newEncryptedAccountEncryptionPrivateKey,
    newEncryptedAccountSigningPrivateKey,
    newRecoveryEncryptedAccountEncryptionPrivateKey,
    newRecoveryEncryptedAccountSigningPrivateKey } = await reEncryptKeyMaterial(exportKey, accountSigningPrivateKey, accountEncryptionPrivateKey, verifyResponse, recoveryPhrase)

  const sodium = await getSodium();

  const m3 = {
      token,
      user: {
          aPAKE: {
              registrationRecord: registrationRecord,
              registrationNonce: nonce,
          },
          keys: {
              masterKdfSalt: sodium.to_base64(newMasterSalt),

              accountEncryptionPublicKey: verifyResponse.recoveryKeys.accountEncryptionPublicKey,
              encAccountEncryptionPrivateKey: sodium.to_base64(newEncryptedAccountEncryptionPrivateKey),
              accountEncryptionKeyNonce: sodium.to_base64(newAccountEncryptionPrivNonce),

              accountSigningPublicKey: verifyResponse.recoveryKeys.accountSigningPublicKey,
              encAccountSigningPrivateKey: sodium.to_base64(newEncryptedAccountSigningPrivateKey),
              accountSigningKeyNonce: sodium.to_base64(newAccountSigningPrivNonce),

              recoveryEncAccountEncryptionPrivateKey: sodium.to_base64(newRecoveryEncryptedAccountEncryptionPrivateKey),
              recoveryAccountEncryptionKeyNonce: sodium.to_base64(newRecoveryAccountEncryptionPrivNonce),
              recoveryEncAccountSigningPrivateKey: sodium.to_base64(newRecoveryEncryptedAccountSigningPrivateKey),
              recoveryAccountSigningKeyNonce: sodium.to_base64(newRecoveryAccountSigningPrivNonce),
          }
      }
  };

    const success = await sendM3(m3)
    return success
}
