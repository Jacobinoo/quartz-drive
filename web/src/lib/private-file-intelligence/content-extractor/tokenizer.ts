import { isStopword } from "./stopwords";

export interface TokenizeResult {
    tokens: string[];
    uniqueTokens: string[];
}

/**
 * Tokenize and normalize raw text into clean search tokens.
 * - Strips diacritics
 * - Filters out stopwords
 * - Strips PDF/layout coordinate numbers
 * - Caps unique tokens at 8,000
 */
export function tokenizeText(rawText: string): TokenizeResult {
    const normalized = rawText
        .toLowerCase()
        .replace(/[łŁ]/g, "l")
        .normalize("NFKD")
        .replace(/[\u0300-\u036f]/g, "");

    const allMatches = normalized.match(/[\p{L}\p{N}_]{2,35}/gu) || [];

    const tokens = allMatches.filter((term) => {
        if (isStopword(term)) return false;

        // Keep words containing letters
        if (/\p{L}/u.test(term)) return true;

        // Keep standard 4-digit years ("2024", "2025") or IDs (5+ digits)
        return /^(19\d\d|20\d\d|\d{5,})$/.test(term);
    });

    const uniqueSet = new Set<string>();
    for (const t of tokens) {
        if (uniqueSet.size >= 8000) break; // Bound memory
        uniqueSet.add(t);
    }

    return {
        tokens,
        uniqueTokens: Array.from(uniqueSet),
    };
}
