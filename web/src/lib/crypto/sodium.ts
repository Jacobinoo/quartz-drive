import type SodiumType from 'libsodium-wrappers-sumo'

let sodiumInstance: typeof SodiumType | null = null;

export async function getSodium() {
    if (sodiumInstance) {
        return sodiumInstance;
    }

    const sodium = (await import("libsodium-wrappers-sumo")).default;
    await sodium.ready;

    sodiumInstance = sodium;

    return sodiumInstance;
}