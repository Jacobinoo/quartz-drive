import { config } from "@/config/env";
import * as opaque from '@serenity-kit/opaque'
import { M3ServerPayload } from "./KeyRegisterMaterial";

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

    if (!response.ok) {
        throw new Error(data.message || "An unknown error occurred.");
    }
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


    // const km: KeyRegisterMaterial = await registerKeyMaterial(email, exportKey);

  // const m3: M3ServerPayload = {
  //       user: {
  //           email: email,
  //           aPAKE: {
  //               registrationRecord: registrationRecord,
  //               registrationNonce: registrationNonce,
  //           },
  //           keys: {
  //               masterKdfSalt: km.masterKdfSalt,

  //               accountEncryptionPublicKey: km.accountEncryptionPublicKey,
  //               encAccountEncryptionPrivateKey: km.encAccountEncryptionPrivateKey,
  //               accountEncryptionKeyNonce: km.accountEncryptionKeyNonce,

  //               accountSigningPublicKey: km.accountSigningPublicKey,
  //               encAccountSigningPrivateKey: km.encAccountSigningPrivateKey,
  //               accountSigningKeyNonce: km.accountSigningKeyNonce,

  //               recoveryEncAccountEncryptionPrivateKey: km.recoveryEncAccountEncryptionPrivateKey,
  //               recoveryAccountEncryptionKeyNonce: km.recoveryAccountEncryptionKeyNonce,
  //               recoveryEncAccountSigningPrivateKey: km.recoveryEncAccountSigningPrivateKey,
  //               recoveryAccountSigningKeyNonce: km.recoveryAccountSigningKeyNonce,
  //           },
  //       },
  //       drive: {
  //           defaultShare: {
  //               publicKey: km.sharePublicKey,
  //               wrappedPrivateKey: km.wrappedSharePrivateKey,
  //               privKeyNonce: km.sharePrivNonce,

  //               encryptedPassphraseForOwner: km.encryptedSharePassphraseForOwner,
  //               signedEncryptedPassphraseForOwner: km.signedEncryptedSharePassphraseForOwner,
  //           },
  //           rootNode: {
  //               publicKey: km.rootNodePublicKey,
  //               wrappedPrivateKey: km.wrappedRootNodePrivateKey,
  //               privKeyNonce: km.rootNodePrivNonce,

  //               encryptedPassphrase: km.encryptedRootNodePassphrase,
  //               signedEncryptedPassphrase: km.signedEncryptedRootNodePassphrase,
  //           },
  //       },
  //   };

  const m3: M3ServerPayload = {
        user: {
            email: email,
            aPAKE: {
                registrationRecord: registrationRecord,
                registrationNonce: registrationNonce,
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
}
