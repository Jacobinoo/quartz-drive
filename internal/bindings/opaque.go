package bindings

/*
#cgo LDFLAGS: -L${SRCDIR}/opaque_rust/target/release -lopaque_rust -lm -ldl -framework Security -framework CoreFoundation
#cgo CFLAGS: -I${SRCDIR}/opaque_rust

#include "opaque_rust/opaque_rust.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

// rustBytesToGo copies Rust-allocated bytes into a Go slice and frees the Rust buffer.
func rustBytesToGo(ptr *C.uchar, length C.size_t) []byte {
	if ptr == nil || length == 0 {
		return nil
	}
	goBytes := C.GoBytes(unsafe.Pointer(ptr), C.int(length))
	C.free_opaque_buffer(ptr, length)
	return goBytes
}

// ─── Public API ──────────────────────────────────────────────────────────────

// GenerateServerSetup creates a brand-new OPAQUE ServerSetup.
// You typically only call this once, then persist the result as OPAQUE_SERVER_SETUP.
func GenerateServerSetup() ([]byte, error) {
	var outPtr *C.uchar
	var outLen C.size_t

	rc := C.opaque_server_setup(&outPtr, &outLen)
	if rc != 0 {
		return nil, fmt.Errorf("opaque_server_setup failed (rc=%d)", rc)
	}
	return rustBytesToGo(outPtr, outLen), nil
}

// StartRegistration processes a client's RegistrationRequest and returns a RegistrationResponse.
// Registration in opaque-ke v4 is STATELESS on the server — no state to persist between steps.
func StartRegistration(serverSetup, registrationRequest, userIdentifier []byte) (registrationResponse []byte, err error) {
	var outPtr *C.uchar
	var outLen C.size_t

	rc := C.opaque_start_registration(
		(*C.uchar)(unsafe.Pointer(&serverSetup[0])), C.size_t(len(serverSetup)),
		(*C.uchar)(unsafe.Pointer(&registrationRequest[0])), C.size_t(len(registrationRequest)),
		(*C.uchar)(unsafe.Pointer(&userIdentifier[0])), C.size_t(len(userIdentifier)),
		&outPtr, &outLen,
	)
	if rc != 0 {
		return nil, fmt.Errorf("opaque_start_registration failed (rc=%d)", rc)
	}
	return rustBytesToGo(outPtr, outLen), nil
}

// FinishRegistration processes a client's RegistrationUpload and returns the password file
// record that should be stored in the database alongside the user.
func FinishRegistration(registrationUpload []byte) (passwordFileRecord []byte, err error) {
	var outPtr *C.uchar
	var outLen C.size_t

	rc := C.opaque_finish_registration(
		(*C.uchar)(unsafe.Pointer(&registrationUpload[0])), C.size_t(len(registrationUpload)),
		&outPtr, &outLen,
	)
	if rc != 0 {
		return nil, fmt.Errorf("opaque_finish_registration failed (rc=%d)", rc)
	}
	return rustBytesToGo(outPtr, outLen), nil
}

// StartLogin processes a client's CredentialRequest and returns:
//   - credentialResponse: send this back to the client
//   - serverLoginState: persist this in Redis (30s TTL) for FinishLogin
func StartLogin(serverSetup, passwordFileRecord, credentialRequest, userIdentifier []byte) (credentialResponse, serverLoginState []byte, err error) {
	var responsePtr *C.uchar
	var responseLen C.size_t
	var statePtr *C.uchar
	var stateLen C.size_t

	rc := C.opaque_start_login(
		(*C.uchar)(unsafe.Pointer(&serverSetup[0])), C.size_t(len(serverSetup)),
		(*C.uchar)(unsafe.Pointer(&passwordFileRecord[0])), C.size_t(len(passwordFileRecord)),
		(*C.uchar)(unsafe.Pointer(&credentialRequest[0])), C.size_t(len(credentialRequest)),
		(*C.uchar)(unsafe.Pointer(&userIdentifier[0])), C.size_t(len(userIdentifier)),
		&responsePtr, &responseLen,
		&statePtr, &stateLen,
	)
	if rc != 0 {
		return nil, nil, fmt.Errorf("opaque_start_login failed (rc=%d)", rc)
	}
	return rustBytesToGo(responsePtr, responseLen), rustBytesToGo(statePtr, stateLen), nil
}

// FinishLogin verifies a client's CredentialFinalization against the stored ServerLogin state.
// Returns the agreed session key on success (both sides derived the same key).
func FinishLogin(serverLoginState, credentialFinalization []byte) (sessionKey []byte, err error) {
	var outPtr *C.uchar
	var outLen C.size_t

	rc := C.opaque_finish_login(
		(*C.uchar)(unsafe.Pointer(&serverLoginState[0])), C.size_t(len(serverLoginState)),
		(*C.uchar)(unsafe.Pointer(&credentialFinalization[0])), C.size_t(len(credentialFinalization)),
		&outPtr, &outLen,
	)
	if rc != 0 {
		return nil, fmt.Errorf("opaque_finish_login failed (rc=%d)", rc)
	}
	return rustBytesToGo(outPtr, outLen), nil
}
