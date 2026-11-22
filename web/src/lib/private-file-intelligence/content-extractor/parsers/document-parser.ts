import { unzipSync, strFromU8 } from "fflate";

/** Extracts Word (.docx) document body & headers text */
export async function extractDocxContent(file: File): Promise<string> {
    const buf = new Uint8Array(await file.arrayBuffer());
    const unzipped = unzipSync(buf, {
        filter: (f) =>
            f.name === "word/document.xml" || f.name === "word/header1.xml",
    });

    const docXml = unzipped["word/document.xml"];
    if (!docXml) return "";

    const xmlStr = strFromU8(docXml);
    // Match text nodes in document body: <w:t ...>Text</w:t>
    const matches = xmlStr.match(/<w:t\b[^>]*>([\s\S]*?)<\/w:t>/g) || [];
    const textPieces = matches.map((m) => m.replace(/<[^>]+>/g, ""));
    return textPieces.join(" ");
}

/** Extracts Excel (.xlsx) workbook shared strings and worksheet cells */
export async function extractXlsxContent(file: File): Promise<string> {
    const buf = new Uint8Array(await file.arrayBuffer());
    const unzipped = unzipSync(buf, {
        filter: (f) =>
            f.name === "xl/sharedStrings.xml" ||
            f.name.startsWith("xl/worksheets/sheet"),
    });

    const texts: string[] = [];

    // 1. Shared Strings table (contains all text entries in the workbook)
    if (unzipped["xl/sharedStrings.xml"]) {
        const sharedXml = strFromU8(unzipped["xl/sharedStrings.xml"]);
        const stringMatches =
            sharedXml.match(/<t\b[^>]*>([\s\S]*?)<\/t>/g) || [];
        texts.push(...stringMatches.map((m) => m.replace(/<[^>]+>/g, "")));
    }

    // 2. Cell values in sheets
    for (const path in unzipped) {
        if (path.startsWith("xl/worksheets/sheet")) {
            const sheetXml = strFromU8(unzipped[path]);
            const cellMatches = sheetXml.match(/<v>([\s\S]*?)<\/v>/g) || [];
            texts.push(...cellMatches.map((m) => m.replace(/<[^>]+>/g, "")));
        }
    }

    return texts.join(" ");
}

/** Extracts E-Book (.epub) XHTML chapters */
export async function extractEpubContent(file: File): Promise<string> {
    const buf = new Uint8Array(await file.arrayBuffer());
    const unzipped = unzipSync(buf, {
        filter: (f) => f.name.endsWith(".xhtml") || f.name.endsWith(".html"),
    });

    const chapters: string[] = [];
    for (const path in unzipped) {
        const htmlStr = strFromU8(unzipped[path]);
        const plain = htmlStr.replace(/<[^>]+>/g, " ");
        chapters.push(plain);
    }
    return chapters.join("\n");
}
