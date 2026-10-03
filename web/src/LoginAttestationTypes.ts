import { UUID } from "crypto";
import { KDFParams } from "@/UtilTypes";

export type LoginAttestationHeader = {
    status: "ok" | "error";
    attestation: boolean;
};

export type TrustedUserData = {
    createdAt: any;
    updatedAt: any;
    deletedAt: any;

    id: UUID;
    email: string;

    kdfParams: KDFParams;
    encryptionVersion: number;

    masterKdfSalt: string;

    accountEncryptionPublicKey: string;
    encryptedAccountEncryptionPrivateKey: string;
    accountEncryptionKeyNonce: string;

    accountSigningPublicKey: string;
    encryptedAccountSigningPrivateKey: string;
    accountSigningKeyNonce: string;

    sessionPrivateKey: string;
    token: string;
    csrfToken: string;
};

export type LoginAttestationConfirmed = LoginAttestationHeader &
    TrustedUserData;
