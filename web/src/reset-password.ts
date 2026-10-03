import { config } from "@/config/env";
import * as opaque from "@serenity-kit/opaque";

export async function verifyToken(sessionId: string, token: string) {
    const verifyResponse = await fetch(
        `${config.apiUrl}/v1/recovery/session/${sessionId}/verify`,
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ token }),
        }
    );

    if (!verifyResponse.ok) {
        throw new Error("Invalid or expired reset link");
    }

    return true;
}

export async function resetPassword(sessionId: string, newPassword: string) {
    await opaque.ready;

    // 1. Initialize OPAQUE registration with the new password
    const { clientRegistrationState, registrationRequest } =
        opaque.client.startRegistration({ password: newPassword });

    // 2. Send M1
    const m1Response = await fetch(
        `${config.apiUrl}/v1/recovery/session/${sessionId}/opaque/m1`,
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ registrationRequest }),
        }
    );

    if (!m1Response.ok) {
        throw new Error("Failed to initialize password reset (M1)");
    }

    const { registrationResponse } = await m1Response.json();

    // 3. Finish OPAQUE registration locally
    const { registrationRecord, exportKey } = opaque.client.finishRegistration({
        clientRegistrationState,
        registrationResponse,
        password: newPassword,
    });

    // 4. Send M3 (Complete)
    const m3Response = await fetch(
        `${config.apiUrl}/v1/recovery/session/${sessionId}/complete`,
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                new_opaque_record: registrationRecord,
                // Re-encrypted keys are NOT sent here for Email Recovery!
            }),
        }
    );

    if (!m3Response.ok) {
        throw new Error("Failed to finalize password reset (M3)");
    }

    return true;
}
