import * as opaque from '@serenity-kit/opaque'
import { Base64String } from "@/UtilTypes";
import { getSodium } from "@/lib/crypto/sodium";

export async function resetPassword(email: string, token: string, recoveryPhrase: string, newPassword: string) {
    const sodium = await getSodium();

    // 1. Verify token and get recovery keys
    const verifyResponse = await fetch("https://localhost:3100/v1/account/verify-reset-code", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, token })
    });
    
    if (!verifyResponse.ok) {
        throw new Error("Invalid or expired reset link");
    }

    const { recoveryKeys } = await verifyResponse.json();
    const oldMasterSalt = sodium.from_base64(recoveryKeys.masterKdfSalt);

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
    
    try {
        accountSigningPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
            null,
            sodium.from_base64(recoveryKeys.recoveryEncAccountSigningPrivateKey),
            sodium.from_string(email.toLowerCase()+"_sign"),
            sodium.from_base64(recoveryKeys.recoveryAccountSigningKeyNonce),
            derivedRecoveryKey
        );

        accountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
            null,
            sodium.from_base64(recoveryKeys.recoveryEncAccountEncryptionPrivateKey),
            sodium.from_string(email.toLowerCase()+"_encrypt"),
            sodium.from_base64(recoveryKeys.recoveryAccountEncryptionKeyNonce),
            derivedRecoveryKey
        );
    } catch (e) {
        throw new Error("Invalid recovery phrase");
    }

    // 4. Start OPAQUE Registration with NEW password
    await opaque.ready;
    const { clientRegistrationState, registrationRequest } = opaque.client.startRegistration({ password: newPassword });

    const m1Response = await fetch("https://localhost:3100/v1/account/reset-password/m1", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, token, registrationRequest })
    });

    if (!m1Response.ok) {
        throw new Error("Failed to start password reset");
    }

    const { registrationResponse, nonce } = await m1Response.json();

    const { registrationRecord, exportKey } = opaque.client.finishRegistration({
        clientRegistrationState,
        registrationResponse,
        password: newPassword,
    });

    // 5. Derive NEW Master Key from OPAQUE exportKey (NOT the raw password)
    const newMasterSalt = sodium.randombytes_buf(sodium.crypto_pwhash_SALTBYTES);
    const newDerivedMasterKey = sodium.crypto_pwhash(
        sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES,
        exportKey,
        newMasterSalt,
        sodium.crypto_pwhash_OPSLIMIT_INTERACTIVE,
        sodium.crypto_pwhash_MEMLIMIT_INTERACTIVE,
        sodium.crypto_pwhash_ALG_ARGON2ID13
    );

    // 6. Re-encrypt the Account Private Keys with the NEW Master Key
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
        sodium.from_string(email.toLowerCase()+"_sign"),
        null,
        newRecoveryAccountSigningPrivNonce,
        newDerivedRecoveryKey
    );

    const newRecoveryAccountEncryptionPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const newRecoveryEncryptedAccountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        accountEncryptionPrivateKey,
        sodium.from_string(email.toLowerCase()+"_encrypt"),
        null,
        newRecoveryAccountEncryptionPrivNonce,
        newDerivedRecoveryKey
    );

    // 7. Send M3 payload (With the newly encrypted Recovery Keys so they match the new salt!)
    const m3 = {
        email,
        token,
        user: {
            email,
            aPAKE: {
                registrationRecord: registrationRecord,
                registrationNonce: nonce,
            },
            keys: {
                masterKdfSalt: sodium.to_base64(newMasterSalt),

                accountEncryptionPublicKey: recoveryKeys.accountEncryptionPublicKey,
                encAccountEncryptionPrivateKey: sodium.to_base64(newEncryptedAccountEncryptionPrivateKey),
                accountEncryptionKeyNonce: sodium.to_base64(newAccountEncryptionPrivNonce),

                accountSigningPublicKey: recoveryKeys.accountSigningPublicKey,
                encAccountSigningPrivateKey: sodium.to_base64(newEncryptedAccountSigningPrivateKey),
                accountSigningKeyNonce: sodium.to_base64(newAccountSigningPrivNonce),

                recoveryEncAccountEncryptionPrivateKey: sodium.to_base64(newRecoveryEncryptedAccountEncryptionPrivateKey),
                recoveryAccountEncryptionKeyNonce: sodium.to_base64(newRecoveryAccountEncryptionPrivNonce),
                recoveryEncAccountSigningPrivateKey: sodium.to_base64(newRecoveryEncryptedAccountSigningPrivateKey),
                recoveryAccountSigningKeyNonce: sodium.to_base64(newRecoveryAccountSigningPrivNonce),
            }
        }
    };

    const m3Response = await fetch("https://localhost:3100/v1/account/reset-password/m3", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(m3)
    });

    if (!m3Response.ok) {
        throw new Error("Failed to finalize password reset");
    }

    return true;
}
