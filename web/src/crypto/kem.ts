import _sodium from "libsodium-wrappers-sumo";
import { getSodium } from "@/lib/crypto/sodium";

export async function quantumSeal(
    payload: Uint8Array,
    recipientPublicKey: Uint8Array
): Promise<Uint8Array> {
    const sodium = await getSodium();

    // Generate the Quantum-Proof Shared Secret and the KEM Ciphertext
    const kem = sodium.crypto_kem_enc(recipientPublicKey);

    // 2. Generate a random nonce for the symmetric encryption
    const nonce = sodium.randombytes_buf(
        sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES
    );

    // 3. Encrypt the actual payload (your file keys/folder keys) using the KEM's shared secret
    const aeadCiphertext = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        payload,
        null,
        null,
        nonce,
        kem.sharedSecret
    );

    // 4. Glue them together: [ KEM_Ciphertext | Nonce | AEAD_Ciphertext ]
    const result = new Uint8Array(
        kem.ciphertext.length + nonce.length + aeadCiphertext.length
    );
    result.set(kem.ciphertext, 0);
    result.set(nonce, kem.ciphertext.length);
    result.set(aeadCiphertext, kem.ciphertext.length + nonce.length);

    return result;
}

export async function quantumSealOpen(
    sealed: Uint8Array,
    recipientPrivateKey: Uint8Array
): Promise<Uint8Array> {
    const sodium = await getSodium();

    // X-Wing constants are accessible via the libsodium API
    const ctLen = sodium.crypto_kem_CIPHERTEXTBYTES;
    const nonceLen = sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES;

    // 1. Split the glued array back into its parts
    const kemCt = sealed.slice(0, ctLen);
    const nonce = sealed.slice(ctLen, ctLen + nonceLen);
    const aeadCt = sealed.slice(ctLen + nonceLen);

    // 2. Decapsulate the Quantum-Proof Shared Secret
    const sharedSecret = sodium.crypto_kem_dec(kemCt, recipientPrivateKey);

    // 3. Decrypt the actual payload using the shared secret
    return sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
        null,
        aeadCt,
        null,
        nonce,
        sharedSecret
    );
}
