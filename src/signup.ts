import { config } from "@/config/env";
import * as opaque from '@serenity-kit/opaque'
import * as bip39 from 'bip39'
import {
    KeyRegisterMaterial
} from "@/KeyRegisterMaterial";
import M3ServerPayload from "@/KeyRegisterMaterial";
import {Base64String} from "@/UtilTypes";
import {getSodium} from "@/lib/crypto/sodium";
import { quantumSeal } from "./crypto/kem";

async function registerKeyMaterial(email: string, exportKey: string): Promise<KeyRegisterMaterial> {
    const sodium = await getSodium();

    // Layer 0 - Master Key (derived from OPAQUE exportKey via fast KDF)
    // We first hash the exportKey to exactly 32 bytes (crypto_kdf_KEYBYTES)
    const exportKeyBytes = typeof exportKey === "string" ? sodium.from_string(exportKey) : exportKey;
    const kdfRootKey = sodium.crypto_generichash(
        sodium.crypto_kdf_KEYBYTES,
        exportKeyBytes,
        null
    );

    // Then we use domain separation to derive the specific master key for Account Keys
    const derivedMasterKey = sodium.crypto_kdf_derive_from_key(
        sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES,
        1,              // Subkey ID 1
        "QMaster!",     // 8-byte context string
        kdfRootKey
    );

    // We still generate a random salt because the Recovery Phrase (which has low entropy)
    // still requires the slow Argon2id algorithm.
    const masterSalt = sodium.randombytes_buf(sodium.crypto_pwhash_SALTBYTES);

    // Layer 1 - Account Identity
    const accountEncryptionKeyPair = sodium.crypto_kem_keypair();
    const accountSigningKeyPair = sodium.crypto_sign_keypair();

    const accountSigningPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const encryptedAccountSigningPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        accountSigningKeyPair.privateKey,
        sodium.from_string(email.toLowerCase()+"_sign"),
        null,
        accountSigningPrivNonce,
        derivedMasterKey
    );

    const accountEncryptionPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const encryptedAccountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        accountEncryptionKeyPair.privateKey,
        sodium.from_string(email.toLowerCase()+"_encrypt"),
        null,
        accountEncryptionPrivNonce,
        derivedMasterKey
    );

    // Layer 1.5 - Recovery Keys
    const recoveryPhrase = bip39.generateMnemonic(128); // 12-word phrase

    const derivedRecoveryKey = sodium.crypto_pwhash(
        sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES,
        recoveryPhrase,
        masterSalt,
        sodium.crypto_pwhash_OPSLIMIT_INTERACTIVE,
        sodium.crypto_pwhash_MEMLIMIT_INTERACTIVE,
        sodium.crypto_pwhash_ALG_ARGON2ID13
    );

    const recoveryAccountSigningPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const recoveryEncryptedAccountSigningPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        accountSigningKeyPair.privateKey,
        sodium.from_string(email.toLowerCase()+"_sign"),
        null,
        recoveryAccountSigningPrivNonce,
        derivedRecoveryKey
    );

    const recoveryAccountEncryptionPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const recoveryEncryptedAccountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        accountEncryptionKeyPair.privateKey,
        sodium.from_string(email.toLowerCase()+"_encrypt"),
        null,
        recoveryAccountEncryptionPrivNonce,
        derivedRecoveryKey
    );

    // Layer 2 - Session Persistence
    const sessionKey = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES);
    const sessionNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);

    // Layer 3 - Default Share
    const shareKeyPair = sodium.crypto_kem_keypair();
    const sharePassphrase = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES);

    const sharePrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const wrappedSharePrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        shareKeyPair.privateKey,
        sodium.from_string("DefaultShare:"+email.toLowerCase()),
        null,
        sharePrivNonce,
        sharePassphrase
    );


    const encryptedSharePassphrase = await quantumSeal(
        sharePassphrase,
        accountEncryptionKeyPair.publicKey,
    );
    const signedEncryptedSharePassphrase = sodium.crypto_sign_detached(
        encryptedSharePassphrase,
        accountSigningKeyPair.privateKey,
    );


    // Layer 4 - Root Node (My Drive)
    const rootNodeKeyPair = sodium.crypto_kem_keypair();
    const rootNodePassphrase = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES);

    const rootNodePrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const wrappedRootNodePrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        rootNodeKeyPair.privateKey,
        sodium.from_string("RootNode:"+email.toLowerCase()),
        null,
        rootNodePrivNonce,
        rootNodePassphrase
    );

    const encryptedRootNodePassphrase = await quantumSeal(
        rootNodePassphrase,
        shareKeyPair.publicKey
    );
    const signedEncryptedRootNodePassphrase = sodium.crypto_sign_detached(
        encryptedRootNodePassphrase,
        accountSigningKeyPair.privateKey
    );

    return {
        masterKdfSalt: sodium.to_base64(masterSalt),

        accountEncryptionPublicKey: sodium.to_base64(accountEncryptionKeyPair.publicKey),
        encAccountEncryptionPrivateKey: sodium.to_base64(encryptedAccountEncryptionPrivateKey),
        accountEncryptionKeyNonce: sodium.to_base64(accountEncryptionPrivNonce),

        accountSigningPublicKey: sodium.to_base64(accountSigningKeyPair.publicKey),
        encAccountSigningPrivateKey: sodium.to_base64(encryptedAccountSigningPrivateKey),
        accountSigningKeyNonce: sodium.to_base64(accountSigningPrivNonce),

        sharePublicKey: sodium.to_base64(shareKeyPair.publicKey),
        wrappedSharePrivateKey: sodium.to_base64(wrappedSharePrivateKey),
        sharePrivNonce: sodium.to_base64(sharePrivNonce),
        encryptedSharePassphraseForOwner: sodium.to_base64(encryptedSharePassphrase),
        signedEncryptedSharePassphraseForOwner: sodium.to_base64(signedEncryptedSharePassphrase),

        rootNodePublicKey: sodium.to_base64(rootNodeKeyPair.publicKey),
        wrappedRootNodePrivateKey: sodium.to_base64(wrappedRootNodePrivateKey),
      rootNodePrivNonce: sodium.to_base64(rootNodePrivNonce),

        encryptedRootNodePassphrase: sodium.to_base64(encryptedRootNodePassphrase),
        signedEncryptedRootNodePassphrase: sodium.to_base64(signedEncryptedRootNodePassphrase),

        recoveryPhrase: recoveryPhrase,
        recoveryEncAccountEncryptionPrivateKey: sodium.to_base64(recoveryEncryptedAccountEncryptionPrivateKey),
        recoveryAccountEncryptionKeyNonce: sodium.to_base64(recoveryAccountEncryptionPrivNonce),
        recoveryEncAccountSigningPrivateKey: sodium.to_base64(recoveryEncryptedAccountSigningPrivateKey),
        recoveryAccountSigningKeyNonce: sodium.to_base64(recoveryAccountSigningPrivNonce),
    };
}

export async function signUp(email: string, password: string, captchaToken:string) {
    if (!email || !password || !captchaToken) throw new Error("Email, password and captcha required");

    await opaque.ready;

    const { clientRegistrationState, registrationRequest } = opaque.client.startRegistration({ password });

    const payload = {
        email,
        registrationRequest: registrationRequest,
    };

    const response = await fetch(`${config.apiUrl}/v1/signup`, {
        method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Verify-Token": captchaToken
      },
        body: JSON.stringify(payload)
    });

    const data = await response.json();
    let registrationResponse: string;
    let registrationNonce: string;

    if((data.registrationResponse && data.registrationResponse != "") && (data.nonce && data.nonce != "")) {
        registrationResponse = data.registrationResponse;
        registrationNonce = data.nonce;
    } else {
        throw Error("Cannot find registration response in payload.")
    }

    const { registrationRecord, exportKey, serverStaticPublicKey } = opaque.client.finishRegistration({
        clientRegistrationState,
        registrationResponse,
      password,
        // identifiers: {
        //     server: "server-identity",
        //     client: email
        // }
    })
    if (serverStaticPublicKey !== process.env.NEXT_PUBLIC_SERVER_PUBLIC_KEY) {
        throw new Error("Server identity verification failed. Aborting registration.");
    }


    const km: KeyRegisterMaterial = await registerKeyMaterial(email, exportKey);

  const m3: M3ServerPayload = {
        user: {
            email: email,
            aPAKE: {
                registrationRecord: registrationRecord,
                registrationNonce: registrationNonce,
            },
            keys: {
                masterKdfSalt: km.masterKdfSalt,

                accountEncryptionPublicKey: km.accountEncryptionPublicKey,
                encAccountEncryptionPrivateKey: km.encAccountEncryptionPrivateKey,
                accountEncryptionKeyNonce: km.accountEncryptionKeyNonce,

                accountSigningPublicKey: km.accountSigningPublicKey,
                encAccountSigningPrivateKey: km.encAccountSigningPrivateKey,
                accountSigningKeyNonce: km.accountSigningKeyNonce,

                recoveryEncAccountEncryptionPrivateKey: km.recoveryEncAccountEncryptionPrivateKey,
                recoveryAccountEncryptionKeyNonce: km.recoveryAccountEncryptionKeyNonce,
                recoveryEncAccountSigningPrivateKey: km.recoveryEncAccountSigningPrivateKey,
                recoveryAccountSigningKeyNonce: km.recoveryAccountSigningKeyNonce,
            },
        },
        drive: {
            defaultShare: {
                publicKey: km.sharePublicKey,
                wrappedPrivateKey: km.wrappedSharePrivateKey,
                privKeyNonce: km.sharePrivNonce,

                encryptedPassphraseForOwner: km.encryptedSharePassphraseForOwner,
                signedEncryptedPassphraseForOwner: km.signedEncryptedSharePassphraseForOwner,
            },
            rootNode: {
                publicKey: km.rootNodePublicKey,
                wrappedPrivateKey: km.wrappedRootNodePrivateKey,
                privKeyNonce: km.rootNodePrivNonce,

                encryptedPassphrase: km.encryptedRootNodePassphrase,
                signedEncryptedPassphrase: km.signedEncryptedRootNodePassphrase,
            },
        },
    };

    const m3Response = await fetch(`${config.apiUrl}/v1/signup/m3`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(m3)
    });

  const m3ResponseData = await m3Response.json();

  if (m3Response.status < 200 && m3Response.status >= 300) {
    throw new Error("Could not sign up.")
  }

  console.log(m3ResponseData)

  return {
    recoveryPhrase: km.recoveryPhrase
  }
}
