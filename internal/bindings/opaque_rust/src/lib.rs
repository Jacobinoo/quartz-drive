use argon2::Argon2;
use generic_array::{ArrayLength, GenericArray};
use libc::{c_int, c_uchar, size_t};
use opaque_ke::ciphersuite::CipherSuite;
use opaque_ke::errors::InternalError;
use opaque_ke::ksf::Ksf;
use opaque_ke::{
    ClientRegistration, ClientRegistrationFinishParameters,
    CredentialFinalization, CredentialRequest, Identifiers, RegistrationRequest,
    RegistrationUpload, Ristretto255, ServerLogin, ServerLoginParameters, ServerRegistration,
    ServerSetup,
};
use rand::rngs::OsRng;
use sha2::Sha512;
use std::slice;

// ─── CipherSuite (must match @serenity-kit/opaque exactly) ───────────────────

pub(crate) struct DefaultCipherSuite;

impl CipherSuite for DefaultCipherSuite {
    type OprfCs = Ristretto255;
    type KeyExchange = opaque_ke::TripleDh<Ristretto255, Sha512>;
    type Ksf = CustomKsf;
}

/// Matches serenity-kit's CustomKsf: Argon2id with default "MemoryConstrained"
/// params (t=3, m=2^16, p=4). The KSF is only invoked client-side, but we need
/// the same type to satisfy the CipherSuite generic parameter.
#[derive(Default)]
pub(crate) struct CustomKsf {
    argon: Argon2<'static>,
}

impl Ksf for CustomKsf {
    fn hash<L: ArrayLength<u8>>(
        &self,
        input: GenericArray<u8, L>,
    ) -> Result<GenericArray<u8, L>, InternalError> {
        let mut output = GenericArray::default();
        self.argon
            .hash_password_into(&input, &[0; argon2::RECOMMENDED_SALT_LEN], &mut output)
            .map_err(|_| InternalError::KsfError)?;
        Ok(output)
    }
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

/// Write a Vec<u8> out through C double-pointer + length, forgetting the Vec
/// so Go owns the memory (must call free_opaque_buffer to reclaim).
unsafe fn write_vec_to_c(vec: &mut Vec<u8>, out_ptr: *mut *mut c_uchar, out_len: *mut size_t) {
    vec.shrink_to_fit();
    *out_ptr = vec.as_mut_ptr();
    *out_len = vec.len();
    std::mem::forget(std::mem::take(vec));
}

// ─── FFI Functions ───────────────────────────────────────────────────────────

/// Generate a new ServerSetup and return serialized bytes.
#[no_mangle]
pub extern "C" fn opaque_server_setup(
    out_ptr: *mut *mut c_uchar,
    out_len: *mut size_t,
) -> c_int {
    let mut rng = OsRng;
    let server_setup = ServerSetup::<DefaultCipherSuite>::new(&mut rng);
    let mut vec = server_setup.serialize().to_vec();
    unsafe { write_vec_to_c(&mut vec, out_ptr, out_len) };
    0
}

/// Registration Step 1: Server processes RegistrationRequest, returns RegistrationResponse.
/// In opaque-ke v4, registration is STATELESS on the server — no state to persist.
#[no_mangle]
pub extern "C" fn opaque_start_registration(
    setup_ptr: *const c_uchar,
    setup_len: size_t,
    req_ptr: *const c_uchar,
    req_len: size_t,
    user_id_ptr: *const c_uchar,
    user_id_len: size_t,
    out_response_ptr: *mut *mut c_uchar,
    out_response_len: *mut size_t,
) -> c_int {
    let setup_bytes = unsafe { slice::from_raw_parts(setup_ptr, setup_len) };
    let req_bytes = unsafe { slice::from_raw_parts(req_ptr, req_len) };
    let user_id = unsafe { slice::from_raw_parts(user_id_ptr, user_id_len) };

    let setup = match ServerSetup::<DefaultCipherSuite>::deserialize(setup_bytes) {
        Ok(s) => s,
        Err(_) => return 1,
    };
    let req = match RegistrationRequest::deserialize(req_bytes) {
        Ok(r) => r,
        Err(_) => return 2,
    };

    let result = match ServerRegistration::<DefaultCipherSuite>::start(&setup, req, user_id) {
        Ok(r) => r,
        Err(_) => return 3,
    };

    let mut response_vec = result.message.serialize().to_vec();
    unsafe { write_vec_to_c(&mut response_vec, out_response_ptr, out_response_len) };
    0
}

/// Registration Step 2: Server receives RegistrationUpload, returns the password file record.
/// This is a static operation — just transforms the upload into a storable record.
#[no_mangle]
pub extern "C" fn opaque_finish_registration(
    upload_ptr: *const c_uchar,
    upload_len: size_t,
    out_record_ptr: *mut *mut c_uchar,
    out_record_len: *mut size_t,
) -> c_int {
    let upload_bytes = unsafe { slice::from_raw_parts(upload_ptr, upload_len) };

    let upload = match RegistrationUpload::<DefaultCipherSuite>::deserialize(upload_bytes) {
        Ok(u) => u,
        Err(_) => return 1,
    };

    let record = ServerRegistration::<DefaultCipherSuite>::finish(upload);
    let mut rec_vec = record.serialize().to_vec();
    unsafe { write_vec_to_c(&mut rec_vec, out_record_ptr, out_record_len) };
    0
}

/// Login Step 1: Server processes CredentialRequest, returns CredentialResponse + ServerLogin state.
/// The state MUST be persisted (e.g. in Redis) for finish_login.
#[no_mangle]
pub extern "C" fn opaque_start_login(
    setup_ptr: *const c_uchar,
    setup_len: size_t,
    record_ptr: *const c_uchar,
    record_len: size_t,
    req_ptr: *const c_uchar,
    req_len: size_t,
    user_id_ptr: *const c_uchar,
    user_id_len: size_t,
    out_response_ptr: *mut *mut c_uchar,
    out_response_len: *mut size_t,
    out_state_ptr: *mut *mut c_uchar,
    out_state_len: *mut size_t,
) -> c_int {
    let setup_bytes = unsafe { slice::from_raw_parts(setup_ptr, setup_len) };
    let record_bytes = unsafe { slice::from_raw_parts(record_ptr, record_len) };
    let req_bytes = unsafe { slice::from_raw_parts(req_ptr, req_len) };
    let user_id = unsafe { slice::from_raw_parts(user_id_ptr, user_id_len) };

    let setup = match ServerSetup::<DefaultCipherSuite>::deserialize(setup_bytes) {
        Ok(s) => s,
        Err(_) => return 1,
    };
    let record = match ServerRegistration::<DefaultCipherSuite>::deserialize(record_bytes) {
        Ok(r) => Some(r),
        Err(_) => return 2,
    };
    let req = match CredentialRequest::deserialize(req_bytes) {
        Ok(r) => r,
        Err(_) => return 3,
    };

    let mut rng = OsRng;
    let params = ServerLoginParameters {
        identifiers: Identifiers {
            client: None,
            server: None,
        },
        context: None,
    };

    let result = match ServerLogin::start(&mut rng, &setup, record, req, user_id, params) {
        Ok(r) => r,
        Err(_) => return 4,
    };

    let mut response_vec = result.message.serialize().to_vec();
    let mut state_vec = result.state.serialize().to_vec();
    unsafe {
        write_vec_to_c(&mut response_vec, out_response_ptr, out_response_len);
        write_vec_to_c(&mut state_vec, out_state_ptr, out_state_len);
    }
    0
}

/// Login Step 2: Server verifies CredentialFinalization, returns session key.
#[no_mangle]
pub extern "C" fn opaque_finish_login(
    state_ptr: *const c_uchar,
    state_len: size_t,
    finalization_ptr: *const c_uchar,
    finalization_len: size_t,
    out_session_key_ptr: *mut *mut c_uchar,
    out_session_key_len: *mut size_t,
) -> c_int {
    let state_bytes = unsafe { slice::from_raw_parts(state_ptr, state_len) };
    let finalization_bytes = unsafe { slice::from_raw_parts(finalization_ptr, finalization_len) };

    let state = match ServerLogin::<DefaultCipherSuite>::deserialize(state_bytes) {
        Ok(s) => s,
        Err(_) => return 1,
    };
    let finalization = match CredentialFinalization::<DefaultCipherSuite>::deserialize(finalization_bytes) {
        Ok(f) => f,
        Err(_) => return 2,
    };

    let params = ServerLoginParameters {
        identifiers: Identifiers {
            client: None,
            server: None,
        },
        context: None,
    };

    let result = match state.finish(finalization, params) {
        Ok(r) => r,
        Err(_) => return 3,
    };

    let mut sk_vec = result.session_key.to_vec();
    unsafe { write_vec_to_c(&mut sk_vec, out_session_key_ptr, out_session_key_len) };
    0
}

/// Free a buffer that was allocated by any of the above functions.
/// Go MUST call this for every out_ptr it received.
#[no_mangle]
pub extern "C" fn free_opaque_buffer(ptr: *mut c_uchar, len: size_t) {
    if !ptr.is_null() {
        unsafe {
            let _ = Vec::from_raw_parts(ptr, len, len);
        }
    }
}

/// Generate a fake registration record for dummy login (timing attack prevention)
#[no_mangle]
pub extern "C" fn opaque_generate_fake_registration_record(
    setup_ptr: *const c_uchar,
    setup_len: size_t,
    out_record_ptr: *mut *mut c_uchar,
    out_record_len: *mut size_t,
) -> c_int {
    let setup_bytes = unsafe { slice::from_raw_parts(setup_ptr, setup_len) };
    let setup = match ServerSetup::<DefaultCipherSuite>::deserialize(setup_bytes) {
        Ok(s) => s,
        Err(_) => return 1,
    };

    let mut rng = OsRng;
    let fake_password = b"this-is-a-fake-password-never-stored-anywhere";
    let fake_identifier = b"fake-user-identifier";
    
    // M1
    let client_reg_start = match ClientRegistration::<DefaultCipherSuite>::start(&mut rng, fake_password) {
        Ok(r) => r,
        Err(_) => return 2,
    };
    
    // M2
    let server_reg_start = match ServerRegistration::<DefaultCipherSuite>::start(
        &setup,
        client_reg_start.message,
        fake_identifier,
    ) {
        Ok(r) => r,
        Err(_) => return 3,
    };
    
    // M3
    let client_reg_finish = match client_reg_start.state.finish(
        &mut rng,
        fake_password,
        server_reg_start.message,
        ClientRegistrationFinishParameters::default(),
    ) {
        Ok(r) => r,
        Err(_) => return 4,
    };
    
    // Server finish
    let registration_record = ServerRegistration::<DefaultCipherSuite>::finish(client_reg_finish.message);
    let mut rec_vec = registration_record.serialize().to_vec();
    unsafe { write_vec_to_c(&mut rec_vec, out_record_ptr, out_record_len) };
    0
}

mod tests;
