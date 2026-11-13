export async function extractLegacyDocContent(file: File): Promise<string> {
    const slice = file.slice(0, 3 * 1024 * 1024); // First 3MB
    const buffer = await slice.arrayBuffer();
    const bytes = new Uint8Array(buffer);

    let text = "";
    // Extract runs of printable ASCII and UTF-16LE characters
    for (let i = 0; i < bytes.length - 1; i++) {
        const code = bytes[i];
        if (
            (code >= 32 && code <= 126) ||
            code === 10 ||
            code === 13 ||
            code === 9
        ) {
            text += String.fromCharCode(code);
        } else if (
            code === 0 &&
            text.length > 0 &&
            text[text.length - 1] !== " "
        ) {
            text += " ";
        }
    }

    // Filter out binary garbage strings by keeping words of 3+ chars
    const words = text
        .split(/\s+/)
        .filter((w) => w.length >= 3 && /^[a-zA-Z0-9_\-.,]+$/.test(w));
    return words.join(" ");
}
