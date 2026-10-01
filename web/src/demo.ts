import {getSodium} from "@/lib/crypto/sodium";
import * as opaque from "@serenity-kit/opaque";
import {config} from "@/config/env";
import {createDpopProof, generateAndStoreDpopKey, getDpopPrivateKey} from "@/lib/dpop";
import {LoginAttestationConfirmed} from "@/LoginAttestationTypes";
import {setAccountPrivateKeys, setAuthState, setOpaqueInitData} from "@/lib/authStore";
import {saveDevicePrivateKey} from "@/DeviceKeyStore";
import {customFetch} from "@/lib/api";
import {bufferToBase64, generateDeviceKeyPair} from "@/signin";

export async function demoStart(token: string) {
    if (!token) throw new Error("Bot verification required");

    const sodium = await getSodium();

    const dpopPublicKey = await generateAndStoreDpopKey();
    const dpopPrivateKey = await getDpopPrivateKey();
    const dpopProof = await createDpopProof(dpopPrivateKey, dpopPublicKey, "POST", `${config.apiUrl}/v1/demo/start`);

    console.log(dpopProof);

    const deviceKeyPair = await generateDeviceKeyPair();

    const res = await fetch(`${config.apiUrl}/v1/demo/start`, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            "DPoP": dpopProof,
            "X-Verify-Token": token,
        },
        credentials: "include",
    });

    const loginAttestationRaw = await res.json();

    let attestationConfirmed = false

    if((loginAttestationRaw.status && loginAttestationRaw.status == "ok") && (loginAttestationRaw.attestation && loginAttestationRaw.attestation == true)) {
        attestationConfirmed = true
    }

    if (!attestationConfirmed) {
        throw Error("Login failed")
    }

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

    const exportKeyKdfRootKey = sodium.from_base64("_n2gXdneMpPGuOwwpPJK0Yv_DcnXZqpsdLSsBm08fV8", sodium.base64_variants.URLSAFE_NO_PADDING);

    const derivedMasterKey = sodium.crypto_kdf_derive_from_key(
        sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES,
        1,              // Subkey ID 1
        "QMaster!",     // 8-byte context string
        exportKeyKdfRootKey
    );

    const accountEncryptionPrivNonce = sodium.from_base64(loginAttestationData.accountEncryptionKeyNonce);
    const encryptedAccountEncryptionPrivateKey = sodium.from_base64(loginAttestationData.encryptedAccountEncryptionPrivateKey);

    const accountSigningPrivNonce = sodium.from_base64(loginAttestationData.accountSigningKeyNonce);
    const encryptedAccountSigningPrivateKey = sodium.from_base64(loginAttestationData.encryptedAccountSigningPrivateKey);
    let accountSigningPrivateKey: Uint8Array | null = null;
    let accountEncryptionPrivateKey: Uint8Array | null = null;

    try {
        accountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
            null,
            encryptedAccountEncryptionPrivateKey,
            sodium.from_base64(loginAttestationData.accountEncryptionPublicKey),
            accountEncryptionPrivNonce,
            derivedMasterKey
        );

        accountSigningPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
            null,
            encryptedAccountSigningPrivateKey,
            sodium.from_base64(loginAttestationData.accountSigningPublicKey),
            accountSigningPrivNonce,
            derivedMasterKey
        );
    } catch (e) {
        console.warn("Failed to decrypt account private keys.");
    }

    setAccountPrivateKeys(accountEncryptionPrivateKey, accountSigningPrivateKey);
    console.log("Zero-Knowledge demo account keys successfully decrypted into JS memory!");

    if (!accountSigningPrivateKey || !accountEncryptionPrivateKey) {
        throw new Error("Failed to decrypt account private keys");
    }

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

    await saveDevicePrivateKey(deviceKeyPair.devicePrivateKey);
    console.log("Non-extractable Device Private Key written to IndexedDB (demo).");

    await customFetch(`${config.apiUrl}/v1/devices/register`, {
        method: "POST",
        body: JSON.stringify({
            devicePublicKey: deviceKeyPair.devicePublicKey,
            wrappedAccountKeys: wrappedAccountKeys
        }),
        credentials: "include",
    });
}
