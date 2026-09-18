import { Base64String } from "@/UtilTypes";
import * as bip39 from 'bip39'
import { quantumSeal } from "./crypto/kem";
import { getSodium } from "./lib/crypto/sodium";


type SharePermission<Read extends true = true,
    Write extends boolean = boolean,
    AddMembers extends boolean = boolean>
    = {
    read: Read;
    write: Read extends true ? Write : false;
    addMembers: Read extends true ? AddMembers : false;
};

type SharePermissions = SharePermission<true, boolean, boolean>;

type NodeMetadata = {
    size: number;
    attributes: object,
    createTime: number;
    modifyTime: number;
}

type KeyRegisterMaterial = {
    masterKdfSalt: Base64String;

    accountEncryptionPublicKey: Base64String;
    encAccountEncryptionPrivateKey: Base64String;
    accountEncryptionKeyNonce: Base64String;

    accountSigningPublicKey: Base64String;
    encAccountSigningPrivateKey: Base64String;
    accountSigningKeyNonce: Base64String;

    recoveryEncAccountEncryptionPrivateKey: Base64String;
    recoveryAccountEncryptionKeyNonce: Base64String;
    recoveryEncAccountSigningPrivateKey: Base64String;
    recoveryAccountSigningKeyNonce: Base64String;

    sharePublicKey: Base64String;
    wrappedSharePrivateKey: Base64String;
    sharePrivNonce: Base64String;
    encryptedSharePassphraseForOwner: Base64String;
    signedEncryptedSharePassphraseForOwner: Base64String;

    rootNodePublicKey: Base64String;
    wrappedRootNodePrivateKey: Base64String;
  rootNodePrivNonce: Base64String;

    encryptedRootNodePassphrase: Base64String;
    signedEncryptedRootNodePassphrase: Base64String;

    recoveryPhrase: string;
    recoveryIdHex: string;

    rawAccountEncryptionPrivateKey: Uint8Array;
    rawAccountSigningPrivateKey: Uint8Array;
}

interface InitializeAccountKeysPayload {
    user: {
        email: string;
        aPAKE: {
            registrationRecord: Base64String;
            registrationNonce: Base64String;
        };
        keys: {
            masterKdfSalt: Base64String;

            accountEncryptionPublicKey: Base64String;
            encAccountEncryptionPrivateKey: Base64String;
            accountEncryptionKeyNonce: Base64String;

            accountSigningPublicKey: Base64String;
            encAccountSigningPrivateKey: Base64String;
            accountSigningKeyNonce: Base64String;

            recoveryEncAccountEncryptionPrivateKey: Base64String;
            recoveryAccountEncryptionKeyNonce: Base64String;
            recoveryEncAccountSigningPrivateKey: Base64String;
            recoveryAccountSigningKeyNonce: Base64String;
            recoveryIdHex: string;
        };
    };
    drive: {
        defaultShare: {
            publicKey: Base64String;
            wrappedPrivateKey: Base64String;
            privKeyNonce: Base64String;

            encryptedPassphraseForOwner: Base64String;
            signedEncryptedPassphraseForOwner: Base64String;
        };
        rootNode: {
            publicKey: Base64String;
            wrappedPrivateKey: Base64String;
            privKeyNonce: Base64String;

            encryptedPassphrase: Base64String;
            signedEncryptedPassphrase: Base64String;
        };
    };
};


interface M3ServerPayload {
  user: {
    email: string;
    aPAKE: {
      registrationRecord: Base64String;
      registrationNonce: Base64String;
    };
  };
};

export type {
  InitializeAccountKeysPayload,
  KeyRegisterMaterial,
  M3ServerPayload,
    SharePermissions,
    NodeMetadata,
}

export async function registerKeyMaterial(email: string, exportKey: string): Promise<KeyRegisterMaterial> {
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
        accountSigningKeyPair.publicKey,
        null,
        accountSigningPrivNonce,
        derivedMasterKey
    );

    const accountEncryptionPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const encryptedAccountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        accountEncryptionKeyPair.privateKey,
        accountEncryptionKeyPair.publicKey,
        null,
        accountEncryptionPrivNonce,
        derivedMasterKey
    );

    // Layer 1.5 - Recovery Keys
  // const recoveryPhrase = bip39.generateMnemonic(256); // 24-word phrase
  // const rawSeedBytes = await bip39.mnemonicToSeed(recoveryPhrase) //64-byte seed

  // // hash 64-byte seed down to 32-bytes, because the root key has to be 32-bytes
  // const recoveryRootKey = sodium.crypto_generichash(
  //     sodium.crypto_kdf_KEYBYTES,
  //     rawSeedBytes,
  //     null
  // );

  // const recoveryIDBytes = sodium.crypto_kdf_derive_from_key(
  //     sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES,
  //     1,
  //     "recov_id",
  //     recoveryRootKey
  // );
  // const hashedRecoveryID = sodium.crypto_hash_sha256(
  //   recoveryIDBytes,
  //   "hex"
  // )

  // const derivedRecoveryKey = sodium.crypto_kdf_derive_from_key(
  //     sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES,
  //     2,
  //     "recovkey",
  //     recoveryRootKey
  // );

  const recoveryPhrase = bip39.generateMnemonic(256); // 24-word phrase
  
  const recoveryIdHash = sodium.crypto_generichash(32, sodium.from_string(recoveryPhrase.trim().toLowerCase()), null);
  const recoveryIdHex = sodium.to_hex(recoveryIdHash);


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
        accountSigningKeyPair.publicKey,
        null,
        recoveryAccountSigningPrivNonce,
        derivedRecoveryKey
    );

    const recoveryAccountEncryptionPrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const recoveryEncryptedAccountEncryptionPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        accountEncryptionKeyPair.privateKey,
        accountEncryptionKeyPair.publicKey,
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
        recoveryIdHex: recoveryIdHex,

        rawAccountEncryptionPrivateKey: accountEncryptionKeyPair.privateKey,
        rawAccountSigningPrivateKey: accountSigningKeyPair.privateKey,
    };
}

export async function initializeAccountKeysToServer(km: KeyRegisterMaterial, email: string, registrationRecord: string, registrationNonce: string) {
    const payload: InitializeAccountKeysPayload = {
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
                recoveryIdHex: km.recoveryIdHex,
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

    const { customFetch } = await import('./lib/api');
    const res = await customFetch(`${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"}/v1/account/initialize-keys`, {
        method: "POST",
        body: JSON.stringify(payload),
    });

    if (!res.ok) {
        throw new Error(`Failed to initialize keys on server: ${res.statusText}`);
    }

    return true;
}
