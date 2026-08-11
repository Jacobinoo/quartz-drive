import * as opaque from '@serenity-kit/opaque'
import { LoginAttestationConfirmed } from "@/LoginAttestationTypes";
import {getSodium} from "@/lib/crypto/sodium";
import { createDpopProof, generateAndStoreDpopKey, getDpopPrivateKey } from "@/lib/dpop";
import { Base64String } from './UtilTypes';
import { setAccountPrivateKeys, setAuthState } from './lib/authStore';
import { saveDevicePrivateKey } from './DeviceKeyStore';
import { customFetch } from './lib/api';

export async function signIn(email: string, password: string) {
    if (!email || !password) throw new Error("Email and password required");

    const sodium = await getSodium();
    await opaque.ready;

    const { clientLoginState, startLoginRequest } = opaque.client.startLogin({
        password,
    });

    const m1 = {
        email: email,
        loginRequest: startLoginRequest
    }

    // send opaque m1 and fetch m2 from response
    const res = await fetch(`https://localhost:3100/v1/signin`, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
        },
        body: JSON.stringify(m1)
    });

    const m2 = await res.json();

    let loginResponse: string;
    let loginNonce: string;

    if((m2.loginResponse && m2.loginResponse != "") && (m2.nonce && m2.nonce != "")) {
        loginResponse = m2.loginResponse;
        loginNonce = m2.nonce;
    } else {
        throw Error("Cannot find login response in payload.")
    }

    const loginResult = opaque.client.finishLogin({
        clientLoginState,
        loginResponse,
        password,
        // identifiers: {
        //     server: "server-identity",
        //     client: email
        // }
    });
    if (!loginResult) {
        throw new Error("Login failed");
    }

    const { finishLoginRequest, exportKey } = loginResult;
    const opaqueSessionKey = loginResult.sessionKey;

    console.log("Agreed session key, client is done, waiting for trust attestation from server. SK: " + opaqueSessionKey) //session key agreed, waiting for server trust attestation

    //DPoP
    const dpopPublicKey = await generateAndStoreDpopKey();
    const dpopPrivateKey = await getDpopPrivateKey();
    const dpopProof = await createDpopProof(dpopPrivateKey, dpopPublicKey, "POST", "https://localhost:3100/v1/signin/m3");

  console.log(dpopProof);

  const deviceKeyPair = await generateDeviceKeyPair();

    const m3 = {
      finishLoginRequest: finishLoginRequest,
      nonce: loginNonce,
    }

    // send opaque m3
    const res3 = await fetch(`https://localhost:3100/v1/signin/m3`, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            "DPoP": dpopProof,
        },
        body: JSON.stringify(m3),
        credentials: "include",
    });

    const loginAttestationRaw = await res3.json();

    let attestationConfirmed = false

    if((loginAttestationRaw.status && loginAttestationRaw.status == "ok") && (loginAttestationRaw.attestation && loginAttestationRaw.attestation == true)) {
        attestationConfirmed = true
    }

    if (!attestationConfirmed) {
        throw Error("Login failed")
    }

    console.log("Opaque SessionKey received trust attestation. SK: " + opaqueSessionKey)

    const loginAttestationData: LoginAttestationConfirmed = {
        accountEncryptionKeyNonce: loginAttestationRaw.accountEncryptionKeyNonce,
        accountEncryptionPublicKey: loginAttestationRaw.accountEncryptionPublicKey,
        accountSigningKeyNonce: loginAttestationRaw.accountSigningKeyNonce,
        accountSigningPublicKey: loginAttestationRaw.accountSigningPublicKey,
        attestation: loginAttestationRaw.attestation,
        createdAt: loginAttestationRaw.createdAt,
        csrfToken: loginAttestationRaw.csrfToken,
        deletedAt: loginAttestationRaw.deletedAt,
        email: loginAttestationRaw.email,
        encryptedAccountEncryptionPrivateKey: loginAttestationRaw.encryptedAccountEncryptionPrivateKey,
        encryptedAccountSigningPrivateKey: loginAttestationRaw.encryptedAccountSigningPrivateKey,
        encryptionVersion: loginAttestationRaw.encryptionVersion,
        id: loginAttestationRaw.id,
        kdfParams: loginAttestationRaw.kdfParams,
        masterKdfSalt: loginAttestationRaw.masterKdfSalt,
        sessionPrivateKey: loginAttestationRaw.sessionPrivateKey,
        status: loginAttestationRaw.status,
        token: loginAttestationRaw.token,
        updatedAt: loginAttestationRaw.updatedAt,
        }

    setAuthState(
        loginAttestationData.token,
        loginAttestationData.csrfToken
    );
  console.log("session priv key:", loginAttestationData.sessionPrivateKey)
    const kdfSalt = sodium.from_base64(loginAttestationData.masterKdfSalt);
        // 1. Derive master key from OPAQUE exportKey via Argon2id
        const derivedMasterKey = sodium.crypto_pwhash(
            sodium.crypto_secretbox_KEYBYTES,
            exportKey,
            kdfSalt,
            sodium.crypto_pwhash_OPSLIMIT_INTERACTIVE,
            sodium.crypto_pwhash_MEMLIMIT_INTERACTIVE,
            sodium.crypto_pwhash_ALG_ARGON2ID13
        );
        // 2. Decrypt Account Encryption Private Key
        const accountEncryptionPrivNonce = sodium.from_base64(loginAttestationData.accountEncryptionKeyNonce);
        const encryptedAccountEncryptionPrivateKey = sodium.from_base64(loginAttestationData.encryptedAccountEncryptionPrivateKey);
        const accountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
            null,
            encryptedAccountEncryptionPrivateKey,
            sodium.from_string(loginAttestationData.email.toLowerCase() + "_encrypt"),
            accountEncryptionPrivNonce,
            derivedMasterKey
        );
        // 3. Decrypt Account Signing Private Key
        const accountSigningPrivNonce = sodium.from_base64(loginAttestationData.accountSigningKeyNonce);
        const encryptedAccountSigningPrivateKey = sodium.from_base64(loginAttestationData.encryptedAccountSigningPrivateKey);
        const accountSigningPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
            null,
            encryptedAccountSigningPrivateKey,
            sodium.from_string(loginAttestationData.email.toLowerCase() + "_sign"),
            accountSigningPrivNonce,
            derivedMasterKey
        );
        if (!accountEncryptionPrivateKey || !accountSigningPrivateKey) {
            throw new Error("Failed to decrypt account private keys. Invalid password or corrupted key material!");
        }
        // 4. Save decrypted keys to JS memory in authStore!
        setAccountPrivateKeys(accountEncryptionPrivateKey, accountSigningPrivateKey);
        console.log("Zero-Knowledge account keys successfully decrypted into JS memory!");
        //Encrypt the Libsodium Root Keys using Web Crypto RSA-OAEP
                const combinedKeys = new Uint8Array([...accountSigningPrivateKey, ...accountEncryptionPrivateKey]);

                // Re-import the public key string back into a CryptoKey for encryption
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

                // 7. Save the Non-Extractable Private Key to IndexedDB
                await saveDevicePrivateKey(deviceKeyPair.devicePrivateKey);
  console.log("Non-extractable Device Private Key written to IndexedDB.");

  await customFetch(`https://localhost:3100/v1/devices/register`, {
              method: "POST",
              body: JSON.stringify({
                  devicePublicKey: deviceKeyPair.devicePublicKey,
                  wrappedAccountKeys: wrappedAccountKeys
              }),
              credentials: "include",
          });
    }

//     const kdfSalt = sodium.from_base64(loginAttestationData.masterKdfSalt);
//     // const accountNonce = sodium.from_base64(loginAttestationData.);
//     // const encryptedAccountPrivateKey = sodium.from_base64(loginAttestationData.encryptedAccountPrivateKey);

//     // console.log(`Encrypted Account Private Key: ${loginAttestationData.encryptedAccountPrivateKey}`);

//     // derive master key
//     const derivedMasterKey = sodium.crypto_pwhash(
//         sodium.crypto_secretbox_KEYBYTES,
//         password,
//         kdfSalt,
//         sodium.crypto_pwhash_OPSLIMIT_INTERACTIVE,
//         sodium.crypto_pwhash_MEMLIMIT_INTERACTIVE,
//         sodium.crypto_pwhash_ALG_ARGON2ID13
//     );

//     // decrypt account private key
//     // const privateAccountKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
//     //     null,
//     //     encryptedAccountPrivateKey,
//     //     null,
//     //     accountNonce,
//     //     derivedMasterKey
//     // );

//     // if (!privateAccountKey) {
//     //     throw new Error("Invalid password");
//     // }

//     // const accountPub = sodium.from_base64(loginAttestationData.accountPublicKey);

//     // const unwrappedDriveKey = sodium.crypto_box_seal_open(
//     //     wrappedDriveKey,
//     //     accountPub,
//     //     privateAccountKey
//     //     );
//     // if (!unwrappedDriveKey) {
//     //     throw new Error("Failed to unwrap drive key (seal_open -> false)");
//     // }
//     //Unwrapped drive key in memory!
//     // console.log(`Unwrapped Drive Key: ${unwrappedDriveKey}`);

//     const deviceKeyPair = sodium.crypto_box_keypair();
//     const sessionKey = sodium.randombytes_buf(32) //256-bit symmetric key

//     // const wrappedDriveKeyForDevice = sodium.crypto_box_seal(
//     //     unwrappedDriveKey,
//     //     deviceKeyPair.publicKey
//     // )

//     const deviceNonce = sodium.randombytes_buf(sodium.crypto_secretbox_NONCEBYTES)
//     const encSessDeviceKey = sodium.crypto_secretbox_easy(
//         deviceKeyPair.privateKey,
//         deviceNonce,
//         sessionKey
//     )

//     // await storeKeys(
//     //     wrappedDriveKeyForDevice,
//     //     encSessDeviceKey,
//     //     deviceNonce,
//     //     deviceKeyPair
//     // );
// }


// Helper to convert ArrayBuffer to Base64
function bufferToBase64(buffer: ArrayBuffer): string {
    return btoa(String.fromCharCode(...new Uint8Array(buffer)));
}

async function generateDeviceKeyPair(): Promise<{
    devicePublicKey: string,
    devicePrivateKey: CryptoKey
}> {
    // Generate Non-Extractable RSA KeyPair
    const keyPair = await crypto.subtle.generateKey(
        {
            name: "RSA-OAEP",
            modulusLength: 4096,
            publicExponent: new Uint8Array([1, 0, 1]),
            hash: "SHA-256",
        },
        false, // EXTRACTABLE: FALSE !!
        ["encrypt", "decrypt"]
    );

    // Export only the Public Key to send to the server
    const exportedPubKey = await crypto.subtle.exportKey("spki", keyPair.publicKey);
    return {
        devicePublicKey: bufferToBase64(exportedPubKey),
        devicePrivateKey: keyPair.privateKey
    };
}
