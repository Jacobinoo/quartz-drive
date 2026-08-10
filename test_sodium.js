const sodium = require('libsodium-wrappers-sumo');

async function test() {
    await sodium.ready;
    const buffer = Buffer.from("123456789"); // 9 bytes
    console.log("Buffer size:", buffer.length);
    const nonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES); // 24 bytes
    const key = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES); // 32 bytes
    const ad = sodium.from_string("81b2a952-475c-42e5-827d-92d54fb4a2cc|0|1"); // 36+1+1+1+1 = 40 bytes
    
    const ciphertext = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(buffer, ad, null, nonce, key);
    console.log("Ciphertext size:", ciphertext.length);
    
    const payload = new Uint8Array(nonce.length + ciphertext.length);
    console.log("Payload size:", payload.length);
}

test();
