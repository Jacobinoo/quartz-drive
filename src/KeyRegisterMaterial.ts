import {Base64String} from "@/UtilTypes";

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
}

export default interface M3ServerPayload {
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

            sessionPrivateKey: Base64String;
            sessionNonce: Base64String;
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

export type {
    KeyRegisterMaterial,
    SharePermissions,
    NodeMetadata,
}