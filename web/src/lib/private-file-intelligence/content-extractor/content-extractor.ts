import { extractTextBasedContent } from "@/lib/private-file-intelligence/content-extractor/parsers/text-parser";
import {
    extractDocxContent,
    extractXlsxContent,
    extractEpubContent,
} from "@/lib/private-file-intelligence/content-extractor/parsers/document-parser";
import { extractPdfContent } from "@/lib/private-file-intelligence/content-extractor/parsers/pdf-parser";
import { extractLegacyDocContent } from "@/lib/private-file-intelligence/content-extractor/parsers/legacy-doc-parser";
import { tokenizeText } from "./tokenizer";

// Re-export modular components for convenience
export * from "./tokenizer";
export * from "@/lib/private-file-intelligence/content-extractor/parsers/text-parser";
export * from "@/lib/private-file-intelligence/content-extractor/parsers/document-parser";
export * from "@/lib/private-file-intelligence/content-extractor/parsers/pdf-parser";
export * from "@/lib/private-file-intelligence/content-extractor/parsers/legacy-doc-parser";
export * from "@/lib/private-file-intelligence/content-extractor/parsers/exif-parser";

export interface CoreFileMetadata {
    name: string;
    basename: string;
    extension: string;
    sizeBytes: number;
    sizeFormatted: string;
    sizeCategory: "small" | "medium" | "large" | "huge";
    lastModified: number;
    formattedDate: string;
    mimeType: string;
}

export interface ExtractedContentResult {
    rawText: string;
    previewSnippet: string;
    tokens: string[];
    uniqueTokensCount: number;
    totalWordCount: number;
    durationMs: number;
    parserUsed: string;
    isTruncated: boolean;
}

export function formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024)
        return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

export function extractCoreMetadata(file: File): CoreFileMetadata {
    const name = file.name.trim();
    const lastDot = name.lastIndexOf(".");

    const hasExt = lastDot > 0 && lastDot < name.length - 1;
    const basename = hasExt ? name.slice(0, lastDot) : name;
    const extension = hasExt ? name.slice(lastDot + 1).toLowerCase() : "";

    const size = file.size;
    let sizeCategory: CoreFileMetadata["sizeCategory"] = "small";
    if (size > 100 * 1024 * 1024) sizeCategory = "huge";
    else if (size > 10 * 1024 * 1024) sizeCategory = "large";
    else if (size > 1024 * 1024) sizeCategory = "medium";

    const lastMod = file.lastModified || Date.now();
    const d = new Date(lastMod);
    const formattedDate = d.toLocaleDateString("en-US", {
        month: "short",
        day: "numeric",
        year: "numeric",
        hour: "numeric",
        minute: "2-digit",
    });

    return {
        name,
        basename,
        extension,
        sizeBytes: size,
        sizeFormatted: formatBytes(size),
        sizeCategory,
        lastModified: lastMod,
        formattedDate,
        mimeType: file.type || "application/octet-stream",
    };
}

/**
 * Master Dispatcher: Extract content with a strict 3-second watchdog timer.
 * Routes to the appropriate parser, normalizes and tokenizes extracted text.
 */
export async function extractFileContent(
    file: File,
    extension: string
): Promise<ExtractedContentResult> {
    const startTime = performance.now();
    const ext = extension.toLowerCase();

    let parserUsed = "None";
    let rawText = "";

    // 3-Second Hard Timeout Watchdog
    const timeoutPromise = new Promise<{ text: string; timedOut: boolean }>(
        (resolve) => {
            setTimeout(() => {
                resolve({ text: "", timedOut: true });
            }, 3000);
        }
    );

    const extractPromise = (async () => {
        switch (ext) {
            case "txt":
            case "csv":
            case "tsv":
            case "json":
            case "md":
            case "markdown":
            case "html":
            case "htm":
            case "xml":
            case "svg":
            case "css":
            case "js":
            case "jsx":
            case "ts":
            case "tsx":
            case "py":
            case "sh":
            case "yaml":
            case "yml":
            case "sql":
            case "log":
                parserUsed = `Native String / ${ext.toUpperCase()}`;
                return await extractTextBasedContent(file, ext);

            case "docx":
                parserUsed = "fflate OpenXML (word/document.xml)";
                return await extractDocxContent(file);

            case "xlsx":
                parserUsed = "fflate OpenXML (xl/sharedStrings.xml)";
                return await extractXlsxContent(file);

            case "epub":
                parserUsed = "fflate E-Book (XHTML Chapters)";
                return await extractEpubContent(file);

            case "pdf":
                parserUsed = "pdfjs-dist TextContent Engine";
                return await extractPdfContent(file);

            case "doc":
                parserUsed = "OLE2 Stream Scanner (ASCII/UTF-16)";
                return await extractLegacyDocContent(file);

            default:
                // Binary / media without text
                parserUsed = "Metadata Only (Binary Format)";
                return "";
        }
    })();

    const raced = await Promise.race([
        extractPromise.then((text) => ({ text, timedOut: false })),
        timeoutPromise,
    ]);

    const durationMs = Math.round(performance.now() - startTime);

    if (raced.timedOut) {
        parserUsed += " (Timed Out - Gracefully Degraded)";
    }

    rawText = raced.text.trim();
    const { tokens, uniqueTokens } = tokenizeText(rawText);

    // Generate clean preview snippet (first 350 characters)
    const previewSnippet =
        rawText.length > 350 ? rawText.slice(0, 350).trim() + "..." : rawText;

    return {
        rawText,
        previewSnippet,
        tokens: uniqueTokens,
        uniqueTokensCount: uniqueTokens.length,
        totalWordCount: tokens.length,
        durationMs,
        parserUsed,
        isTruncated: rawText.length > 50000,
    };
}
