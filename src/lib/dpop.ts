function base64UrlEncode(str: string): string {
    return btoa(str)
        .replace(/\+/g, '-')
        .replace(/\//g, '_')
        .replace(/=+$/, '');
}
function base64UrlDecode(str: string): string {
    // Add padding back before decoding
    const pad = '='.repeat((4 - (str.length % 4)) % 4);
    return atob(str.replace(/-/g, '+').replace(/_/g, '/') + pad);
}

async function generateAndStoreDpopKey() {
    const keyPair = await crypto.subtle.generateKey(
        {
            name: "ECDSA",
            namedCurve: "P-256"
        },
        false,
        ["sign", "verify"]
    );

    const publicKeyJwk = await crypto.subtle.exportKey("jwk", keyPair.publicKey);
    const dbRequest = indexedDB.open("dpop-keys", 1);

    dbRequest.onupgradeneeded = event => {
        const db = (event.target as IDBOpenDBRequest).result;
        db.createObjectStore("keys", { keyPath: "id" });
    }

    dbRequest.onsuccess = event => {
        const db = (event.target as IDBOpenDBRequest).result;
        const tx = db.transaction("keys", "readwrite");
        const store = tx.objectStore("keys");
        store.put({
            id: "dpopKey",
            privateKey: keyPair.privateKey,
            publicKey: publicKeyJwk
        });
        tx.oncomplete = () => db.close();
    }
    return publicKeyJwk;
}

async function getDpopPrivateKey(): Promise<CryptoKey> {
    return new Promise((resolve, reject) => {
        const dbRequest = indexedDB.open("dpop-keys", 1);

        dbRequest.onupgradeneeded = (event) => {
            const db = (event.target as IDBOpenDBRequest).result;
            if (!db.objectStoreNames.contains("keys")) {
                db.createObjectStore("keys", { keyPath: "id" });
            }
        };
        dbRequest.onsuccess = (event) => {
            const db = (event.target as IDBOpenDBRequest).result;

            // Safety check: Does the "keys" object store actually exist?
            if (!db.objectStoreNames.contains("keys")) {
                db.close();
                reject("No DPoP object store found (user needs to log in)");
                return;
            }
            try {
                const tx = db.transaction("keys", "readonly");
                const store = tx.objectStore("keys");
                const request = store.get("dpopKey");

                request.onsuccess = () => {
                    if (request.result && request.result.privateKey) {
                        resolve(request.result.privateKey);
                    } else {
                        reject("No DPoP key found in store");
                    }
                    db.close();
                };

                request.onerror = () => {
                    db.close();
                    reject("Error reading from DPoP store");
                };
            } catch (e) {
                db.close();
                reject(e);
            }
        };
        dbRequest.onerror = () => reject("Error opening IndexedDB");
    });
}

async function createDpopProof(
    privateKey: CryptoKey,
    publicKeyJwk: JsonWebKey,
    method: string,
    url: string,
    accessToken?: string,
) {
    const header = {
        alg: "ES256",
        typ: "dpop+jwt",
        jwk: publicKeyJwk
    };
    const payload: {
        htm: string; htu: string; jti: string; iat: number; ath?: string
    } = {
        htm: method,
        htu: url,
        jti: crypto.randomUUID(),
        iat: Math.floor(Date.now() / 1000),
    };

    console.log(payload);

    const encoder = new TextEncoder();

    if(accessToken) {
        const hash = await crypto.subtle.digest('SHA-256', encoder.encode(accessToken));
        const hashBytes = new Uint8Array(hash);
        // @ts-ignore
        payload.ath = hashBytes.toBase64({ alphabet: "base64url", omitPadding: true});
    }

    // @ts-ignore
    const headerStr: string = encoder.encode(JSON.stringify(header)).toBase64({ alphabet: "base64url", omitPadding: true});
    // @ts-ignore
    const payloadStr: string = encoder.encode(JSON.stringify(payload)).toBase64({ alphabet: "base64url", omitPadding: true});
    const input = `${headerStr}.${payloadStr}`;

    const signature = await crypto.subtle.sign(
        {
            name: "ECDSA",
            hash: { name: "SHA-256" }
        },
        privateKey,
        encoder.encode(input)
    );

    return (
        `${input}.${(new Uint8Array(signature) as any).toBase64({alphabet: "base64url", omitPadding: true})}`
    )
}

async function getDpopKeyPair(): Promise<{ privateKey: CryptoKey; publicKey: JsonWebKey }> {
    return new Promise((resolve, reject) => {
        const dbRequest = indexedDB.open("dpop-keys", 1);

        dbRequest.onupgradeneeded = (event) => {
            const db = (event.target as IDBOpenDBRequest).result;
            if (!db.objectStoreNames.contains("keys")) {
                db.createObjectStore("keys", { keyPath: "id" });
            }
        };
        dbRequest.onsuccess = (event) => {
            const db = (event.target as IDBOpenDBRequest).result;

            // Safety check: Does the "keys" object store actually exist?
            if (!db.objectStoreNames.contains("keys")) {
                db.close();
                reject("No DPoP object store found (user needs to log in)");
                return;
            }
            try {
                const tx = db.transaction("keys", "readonly");
                const store = tx.objectStore("keys");
                const request = store.get("dpopKey");

                request.onsuccess = () => {
                    if (request.result && request.result.privateKey && request.result.publicKey) {
                        resolve({
                            privateKey: request.result.privateKey,
                            publicKey: request.result.publicKey
                        });
                    } else {
                        reject("No DPoP keypair found in store");
                    }
                    db.close();
                };

                request.onerror = () => {
                    db.close();
                    reject("Error reading DPoP keypair from store");
                };
            } catch (e) {
                db.close();
                reject(e);
            }
        };
        dbRequest.onerror = () => reject("Error opening IndexedDB");
    });
}

async function deleteDpopDatabase(): Promise<void> {
    return new Promise((resolve, reject) => {
        const req = indexedDB.deleteDatabase("dpop-keys");
        req.onsuccess = () => {
            console.log("DPoP IndexedDB deleted successfully");
            resolve();
        };
        req.onerror = () => {
            console.error("Could not delete DPoP IndexedDB");
            reject(req.error);
        };
        req.onblocked = () => {
            console.warn("DPoP IndexedDB delete blocked by open connection");
            resolve(); // Resolve anyway so signout isn't held up
        };
    });
}


export { generateAndStoreDpopKey, getDpopPrivateKey, createDpopProof, getDpopKeyPair, deleteDpopDatabase };
