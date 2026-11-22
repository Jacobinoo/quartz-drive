import { unzlibSync } from "fflate";

export interface TextItemLike {
    str: string;
    transform: number[];
    width: number;
    height: number;
    hasEOL: boolean;
}

/**
 * Sanitize extracted PDF text:
 * - Normalize unicode & ligatures (e.g. fi, fl, ffi -> separate characters)
 * - Strip private use area characters from unmapped embedded subset fonts
 * - Strip non-printable control characters
 * - Strip zero-width spaces and soft hyphens
 */
export function sanitizePdfText(text: string): string {
    return text
        .normalize("NFKC")
        .replace(/[\uE000-\uF8FF\uFFF0-\uFFFF]/g, "")
        .replace(/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F-\x9F]/g, "")
        .replace(/[\u00AD\u200B-\u200D\uFEFF]/g, "")
        .replace(/\n{3,}/g, "\n\n")
        .replace(/[^\S\r\n]+/g, " ")
        .trim();
}

/**
 * Determine if text extracted from a PDF is corrupt font noise or binary garbage.
 */
export function isGibberishText(text: string): boolean {
    if (!text || text.length < 15) return true;

    const lettersAndDigits = text.match(/[\p{L}\p{N}]/gu)?.length || 0;
    const ratio = lettersAndDigits / text.length;

    // Real document text is at least 45% alphanumeric characters
    if (ratio < 0.45) return true;

    const words = text.split(/\s+/).filter(Boolean);
    if (words.length > 5) {
        const avgWordLen = lettersAndDigits / words.length;
        // Text consisting solely of 1-letter noise or 40+ character binary blobs is gibberish
        if (avgWordLen < 1.8 || avgWordLen > 28) return true;
    }

    return false;
}

/**
 * Intelligently assemble PDF.js TextContent items into sentences and paragraphs:
 * - Avoids splitting kerning chunks into fragmented 2-letter tokens (e.g. "wor" + "ds" -> "words")
 * - Inserts spaces between distinct words on the same line
 * - Automatically un-hyphenates line breaks (e.g. "cryptograph-" + "ic" -> "cryptographic")
 */
export function assemblePageText(rawItems: any[]): string {
    let lastY: number | null = null;
    let lastX: number | null = null;
    let lastWidth = 0;
    let pageText = "";

    for (const rawItem of rawItems) {
        if (!("str" in rawItem)) continue;
        const item = rawItem as TextItemLike;

        if (!item.str) {
            if (item.hasEOL && !pageText.endsWith("\n")) {
                pageText += "\n";
                lastX = null;
                lastY = null;
            }
            continue;
        }

        const x = Array.isArray(item.transform) ? item.transform[4] : 0;
        const y = Array.isArray(item.transform) ? item.transform[5] : 0;

        if (lastY !== null && Math.abs(y - lastY) > (item.height || 8) * 0.5) {
            // New line detected
            if (pageText.endsWith("-") && !pageText.endsWith(" -")) {
                pageText = pageText.slice(0, -1); // Un-hyphenate line break
            } else if (!pageText.endsWith(" ") && !pageText.endsWith("\n")) {
                pageText += "\n";
            }
            lastX = null;
        } else if (lastX !== null) {
            // Same line: determine if an actual word boundary space is needed
            const distance = x - (lastX + lastWidth);
            const fontHeight = Array.isArray(item.transform)
                ? Math.abs(item.transform[0]) || item.height || 10
                : 10;
            const spaceThreshold = fontHeight * 0.16;

            const alreadyHasSpace =
                pageText.endsWith(" ") ||
                pageText.endsWith("\n") ||
                item.str.startsWith(" ");

            // Only insert space if there is a real spatial gap between words (not kerning)
            if (!alreadyHasSpace && distance > spaceThreshold) {
                pageText += " ";
            }
        }

        pageText += item.str;
        lastX = x;
        lastY = y;
        lastWidth = item.width || 0;

        if (item.hasEOL) {
            if (pageText.endsWith("-") && !pageText.endsWith(" -")) {
                pageText = pageText.slice(0, -1);
            } else if (!pageText.endsWith("\n")) {
                pageText += "\n";
            }
            lastX = null;
            lastY = null;
        }
    }

    return sanitizePdfText(pageText);
}

/**
 * Main PDF extractor using pdfjs-dist TextContent Engine with intelligent token reconstruction.
 */
export async function extractPdfContent(
    file: File,
    maxPages = 35
): Promise<string> {
    try {
        const arrayBuffer = await file.arrayBuffer();

        // Dynamically load pdfjs-dist in browser
        const pdfjsLib = await import("pdfjs-dist");

        if (typeof window !== "undefined") {
            try {
                // Preload in-memory worker handler as instant zero-failure fallback
                // @ts-ignore
                await import("pdfjs-dist/build/pdf.worker.min.mjs");
            } catch {
                // Handled gracefully by workerSrc
            }
            pdfjsLib.GlobalWorkerOptions.workerSrc = "/pdf.worker.min.mjs";
        }

        const loadingTask = pdfjsLib.getDocument({
            data: arrayBuffer,
            useWorkerFetch: false,
            useSystemFonts: true,
            cMapUrl:
                "https://cdn.jsdelivr.net/npm/pdfjs-dist@" +
                (pdfjsLib.version || "6.5.438") +
                "/cmaps/",
            cMapPacked: true,
            stopAtErrors: false, // Don't reject entire document on recoverable font warnings
        });

        const pdf = await loadingTask.promise;
        const pageCount = Math.min(pdf.numPages, maxPages);
        const textPieces: string[] = [];

        for (let i = 1; i <= pageCount; i++) {
            const page = await pdf.getPage(i);
            const textContent = await page.getTextContent();
            const pageString = assemblePageText(textContent.items);

            if (pageString && !isGibberishText(pageString)) {
                textPieces.push(pageString);
            }
        }

        const fullText = textPieces.join("\n\n").trim();
        if (fullText.length >= 20 && !isGibberishText(fullText)) {
            return fullText;
        }

        // If pdfjs returned empty or corrupt text, attempt clean stream extraction
        return await extractPdfStreamFallback(file);
    } catch (err) {
        console.warn(
            "pdfjs-dist extraction failed, using stream fallback:",
            err
        );
        return extractPdfStreamFallback(file);
    }
}

/**
 * Clean stream decompression fallback using fflate.
 * Decompresses text streams, parses BT...ET text blocks, and discards binary images or corrupt noise.
 */
export async function extractPdfStreamFallback(file: File): Promise<string> {
    try {
        const slice = file.slice(0, 4 * 1024 * 1024); // Inspect first 4MB
        const buffer = await slice.arrayBuffer();
        const bytes = new Uint8Array(buffer);

        // Convert binary buffer to latin1 string for regex matching
        let binaryStr = "";
        const chunkSize = 16384;
        for (let i = 0; i < bytes.length; i += chunkSize) {
            const chunk = bytes.subarray(i, i + chunkSize);
            binaryStr += String.fromCharCode.apply(null, chunk as unknown as number[]);
        }

        // Match PDF streams: << dictionary >> stream ... endstream
        const streamRegex = /<<([\s\S]*?)>>\s*stream[\r\n]+([\s\S]*?)endstream/g;
        let match: RegExpExecArray | null;
        let combinedText = "";

        while ((match = streamRegex.exec(binaryStr)) !== null) {
            const dict = match[1];
            // Skip image objects, font descriptors, and ICC profiles
            if (
                dict.includes("/Image") ||
                dict.includes("/FontDescriptor") ||
                dict.includes("/ICCBased")
            ) {
                continue;
            }

            let streamData = match[2];
            let streamBytes = new Uint8Array(streamData.length);
            for (let i = 0; i < streamData.length; i++) {
                streamBytes[i] = streamData.charCodeAt(i);
            }

            let decompressedStr = "";
            if (dict.includes("/FlateDecode")) {
                try {
                    const decompressed = unzlibSync(streamBytes);
                    decompressedStr = new TextDecoder("utf-8", {
                        fatal: false,
                    }).decode(decompressed);
                } catch {
                    continue; // Skip if decompression fails
                }
            } else {
                decompressedStr = streamData;
            }

            // Extract readable text from BT ... ET blocks
            const parsed = parsePdfStreamText(decompressedStr);
            if (parsed && !isGibberishText(parsed)) {
                combinedText += parsed + "\n";
            }
        }

        const cleaned = sanitizePdfText(combinedText);
        if (cleaned.length >= 20 && !isGibberishText(cleaned)) {
            return cleaned;
        }

        return "";
    } catch {
        return "";
    }
}

/**
 * Extract clean string literals from PDF text operators within BT ... ET blocks.
 */
function parsePdfStreamText(streamStr: string): string {
    let result = "";
    const btRegex = /BT\s+([\s\S]*?)\s+ET/g;
    let blockMatch: RegExpExecArray | null;

    while ((blockMatch = btRegex.exec(streamStr)) !== null) {
        const block = blockMatch[1];
        // Match: (text) Tj, [(text) -10 (text)] TJ, T*
        const opRegex = /\((.*?)\)\s*Tj|\[(.*?)\]\s*TJ|T\*/g;
        let opMatch: RegExpExecArray | null;

        while ((opMatch = opRegex.exec(block)) !== null) {
            if (opMatch[0] === "T*") {
                result += "\n";
            } else if (opMatch[1] !== undefined) {
                result += unescapePdfLiteral(opMatch[1]) + " ";
            } else if (opMatch[2] !== undefined) {
                const arrayContent = opMatch[2];
                const itemRegex = /\((.*?)\)|(-?\d+(?:\.\d+)?)/g;
                let itemMatch: RegExpExecArray | null;

                while ((itemMatch = itemRegex.exec(arrayContent)) !== null) {
                    if (itemMatch[1] !== undefined) {
                        result += unescapePdfLiteral(itemMatch[1]);
                    } else if (itemMatch[2] !== undefined) {
                        const spacing = parseFloat(itemMatch[2]);
                        // Negative number indicates forward movement in PDF TJ (word spacing)
                        if (spacing < -140) {
                            result += " ";
                        }
                    }
                }
                result += " ";
            }
        }
        result += "\n";
    }

    return result.trim();
}

function unescapePdfLiteral(str: string): string {
    return str
        .replace(/\\([()\\])/g, "$1")
        .replace(/\\n/g, "\n")
        .replace(/\\r/g, "\r")
        .replace(/\\t/g, "\t");
}
