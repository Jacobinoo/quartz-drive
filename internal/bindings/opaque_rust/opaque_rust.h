#include <stdarg.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>

/**
 * Generate a new ServerSetup and return serialized bytes.
 */
int opaque_server_setup(unsigned char **out_ptr, size_t *out_len);

/**
 * Registration Step 1: Server processes RegistrationRequest, returns RegistrationResponse.
 * In opaque-ke v4, registration is STATELESS on the server — no state to persist.
 */
int opaque_start_registration(const unsigned char *setup_ptr,
                              size_t setup_len,
                              const unsigned char *req_ptr,
                              size_t req_len,
                              const unsigned char *user_id_ptr,
                              size_t user_id_len,
                              unsigned char **out_response_ptr,
                              size_t *out_response_len);

/**
 * Registration Step 2: Server receives RegistrationUpload, returns the password file record.
 * This is a static operation — just transforms the upload into a storable record.
 */
int opaque_finish_registration(const unsigned char *upload_ptr,
                               size_t upload_len,
                               unsigned char **out_record_ptr,
                               size_t *out_record_len);

/**
 * Login Step 1: Server processes CredentialRequest, returns CredentialResponse + ServerLogin state.
 * The state MUST be persisted (e.g. in Redis) for finish_login.
 */
int opaque_start_login(const unsigned char *setup_ptr,
                       size_t setup_len,
                       const unsigned char *record_ptr,
                       size_t record_len,
                       const unsigned char *req_ptr,
                       size_t req_len,
                       const unsigned char *user_id_ptr,
                       size_t user_id_len,
                       unsigned char **out_response_ptr,
                       size_t *out_response_len,
                       unsigned char **out_state_ptr,
                       size_t *out_state_len);

/**
 * Login Step 2: Server verifies CredentialFinalization, returns session key.
 */
int opaque_finish_login(const unsigned char *state_ptr,
                        size_t state_len,
                        const unsigned char *finalization_ptr,
                        size_t finalization_len,
                        unsigned char **out_session_key_ptr,
                        size_t *out_session_key_len);

/**
 * Free a buffer that was allocated by any of the above functions.
 * Go MUST call this for every out_ptr it received.
 */
void free_opaque_buffer(unsigned char *ptr, size_t len);
