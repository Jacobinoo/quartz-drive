package crypto

// Signup M3 APAKE Key Length
const (
	OpaqueRegistrationRequestSize = 32
	RegistrationNonceBytes        = 32
	OpaqueRegistrationRecordBytes = 192
)

const SodiumNonceBytes = 24 // XChaCha20Poly1305 Nonce

// Signup M3 User Keys byte lengths
const (
	MasterKdfSaltBytes                                = 16   // crypto_pwhash_SALTBYTES
	EncryptionPublicKeyBytes                          = 1216 // crypto_kem_PUBLICKEYBYTES (X-Wing)
	EncryptedAccountEncryptionPrivateKeyBytes         = 48   // kem_SECRETKEYBYTES (32) + AEAD MAC (16)
	AccountSigningPublicKeyBytes                      = 32   // crypto_sign_PUBLICKEYBYTES (Ed25519)
	EncryptedAccountSigningPrivateKeyBytes            = 80   // crypto_sign_SECRETKEYBYTES (64) + AEAD MAC (16)
	RecoveryEncryptedAccountEncryptionPrivateKeyBytes = 48
	RecoveryEncryptedAccountSigningPrivateKeyBytes    = 80
)

// Share Keys byte lengths
const (
	SharePublicKeyBytes                         = 1216 // crypto_kem_PUBLICKEYBYTES (X-Wing)
	ShareWrappedPrivateKeyBytes                 = 48   // kem_SECRETKEYBYTES (32) + AEAD MAC (16)
	ShareEncryptedPassphraseForOwnerBytes       = 1192 // quantumSeal(32 byte passphrase) -> KEM Ciphertext (1120) + Nonce (24) + AEAD Ciphertext (48)
	ShareSignedEncryptedPassphraseForOwnerBytes = 64   // crypto_sign_BYTES (Detached Ed25519 Signature)
)

// Node Keys byte lengths
const (
	NodePublicKeyBytes                 = 1216
	NodeWrappedPrivateKeyBytes         = 48
	NodeEncryptedPassphraseBytes       = 1192
	NodeSignedEncryptedPassphraseBytes = 64
)
