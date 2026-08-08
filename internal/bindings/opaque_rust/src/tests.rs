#[cfg(test)]
mod tests {
    use crate::{CustomKsf, DefaultCipherSuite};
    use opaque_ke::{
        ClientLogin, ClientLoginFinishParameters, ClientRegistration,
        ClientRegistrationFinishParameters, CredentialFinalization, CredentialRequest,
        CredentialResponse, Identifiers, RegistrationRequest, RegistrationResponse,
        RegistrationUpload, ServerLogin, ServerLoginParameters, ServerRegistration, ServerSetup,
    };
    use rand::rngs::OsRng;

    const PASSWORD: &[u8] = b"hunter2_super_secure";
    const USER_ID: &[u8] = b"test@quartz.dev";

    /// Full round-trip: Registration → Login → Session key agreement.
    /// This is the exact flow the frontend + backend will execute.
    #[test]
    fn test_full_opaque_chain() {
        let mut rng = OsRng;

        // ── Step 0: Server generates its long-term setup (done once) ──────────
        let server_setup = ServerSetup::<DefaultCipherSuite>::new(&mut rng);
        let setup_bytes = server_setup.serialize();
        println!("✓ ServerSetup generated ({} bytes)", setup_bytes.len());

        // ══════════════════════════════════════════════════════════════════════
        //  REGISTRATION FLOW
        // ══════════════════════════════════════════════════════════════════════

        // ── Step 1: Client starts registration ────────────────────────────────
        let client_reg_start =
            ClientRegistration::<DefaultCipherSuite>::start(&mut rng, PASSWORD).unwrap();
        let reg_request_bytes = client_reg_start.message.serialize();
        println!(
            "✓ Client → RegistrationRequest ({} bytes)",
            reg_request_bytes.len()
        );

        // ── Step 2: Server processes RegistrationRequest (STATELESS) ──────────
        let server_setup_deserialized =
            ServerSetup::<DefaultCipherSuite>::deserialize(&setup_bytes).unwrap();
        let server_reg_result = ServerRegistration::<DefaultCipherSuite>::start(
            &server_setup_deserialized,
            RegistrationRequest::deserialize(&reg_request_bytes).unwrap(),
            USER_ID,
        )
        .unwrap();
        let reg_response_bytes = server_reg_result.message.serialize();
        println!(
            "✓ Server → RegistrationResponse ({} bytes)",
            reg_response_bytes.len()
        );

        // ── Step 3: Client finishes registration ──────────────────────────────
        let finish_params = ClientRegistrationFinishParameters::new(
            Identifiers {
                client: None,
                server: None,
            },
            None::<&CustomKsf>, // Use default KSF
        );
        let client_reg_finish = client_reg_start
            .state
            .finish(
                &mut rng,
                PASSWORD,
                RegistrationResponse::deserialize(&reg_response_bytes).unwrap(),
                finish_params,
            )
            .unwrap();
        let registration_upload_bytes = client_reg_finish.message.serialize();
        println!(
            "✓ Client → RegistrationUpload ({} bytes)",
            registration_upload_bytes.len()
        );

        // ── Step 4: Server stores password file record ────────────────────────
        let password_file = ServerRegistration::<DefaultCipherSuite>::finish(
            RegistrationUpload::deserialize(&registration_upload_bytes).unwrap(),
        );
        let password_file_bytes = password_file.serialize();
        println!(
            "✓ Server stored password file record ({} bytes)",
            password_file_bytes.len()
        );

        // ══════════════════════════════════════════════════════════════════════
        //  LOGIN FLOW
        // ══════════════════════════════════════════════════════════════════════

        // ── Step 5: Client starts login ───────────────────────────────────────
        let client_login_start =
            ClientLogin::<DefaultCipherSuite>::start(&mut rng, PASSWORD).unwrap();
        let credential_request_bytes = client_login_start.message.serialize();
        println!(
            "✓ Client → CredentialRequest ({} bytes)",
            credential_request_bytes.len()
        );

        // ── Step 6: Server processes CredentialRequest ────────────────────────
        let server_setup_deserialized2 =
            ServerSetup::<DefaultCipherSuite>::deserialize(&setup_bytes).unwrap();
        let password_file_deserialized =
            ServerRegistration::<DefaultCipherSuite>::deserialize(&password_file_bytes).unwrap();

        let server_login_params = ServerLoginParameters {
            identifiers: Identifiers {
                client: None,
                server: None,
            },
            context: None,
        };

        let server_login_result = ServerLogin::start(
            &mut rng,
            &server_setup_deserialized2,
            Some(password_file_deserialized),
            CredentialRequest::deserialize(&credential_request_bytes).unwrap(),
            USER_ID,
            server_login_params,
        )
        .unwrap();

        let credential_response_bytes = server_login_result.message.serialize();
        let server_login_state_bytes = server_login_result.state.serialize();
        println!(
            "✓ Server → CredentialResponse ({} bytes) + ServerLoginState ({} bytes)",
            credential_response_bytes.len(),
            server_login_state_bytes.len()
        );

        // ── Step 7: Client finishes login ─────────────────────────────────────
        let client_login_finish_params = ClientLoginFinishParameters::new(
            None,
            Identifiers {
                client: None,
                server: None,
            },
            None::<&CustomKsf>, // Use default KSF
        );
        let client_login_finish = client_login_start
            .state
            .finish(
                &mut rng,
                PASSWORD,
                CredentialResponse::deserialize(&credential_response_bytes).unwrap(),
                client_login_finish_params,
            )
            .unwrap();
        let credential_finalization_bytes = client_login_finish.message.serialize();
        let client_session_key = client_login_finish.session_key;
        println!(
            "✓ Client → CredentialFinalization ({} bytes)",
            credential_finalization_bytes.len()
        );

        // ── Step 8: Server finishes login ─────────────────────────────────────
        let server_login_state =
            ServerLogin::<DefaultCipherSuite>::deserialize(&server_login_state_bytes).unwrap();

        let server_finish_params = ServerLoginParameters {
            identifiers: Identifiers {
                client: None,
                server: None,
            },
            context: None,
        };

        let server_login_finish = server_login_state
            .finish(
                CredentialFinalization::deserialize(&credential_finalization_bytes).unwrap(),
                server_finish_params,
            )
            .unwrap();
        let server_session_key = server_login_finish.session_key;

        // ── Step 9: Verify session keys match! ────────────────────────────────
        assert_eq!(
            client_session_key, server_session_key,
            "Session keys must match!"
        );
        println!("✓ Session keys match! ({} bytes)", server_session_key.len());
        println!("\n🎉 Full OPAQUE chain passed: Registration → Login → Key Agreement");
    }
}
