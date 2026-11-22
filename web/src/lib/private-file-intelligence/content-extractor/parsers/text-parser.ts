const MAX_INDEXABLE_BYTES = 5 * 1024 * 1024;

export async function extractTextBasedContent(
    file: File,
    extension: string
): Promise<string> {
    const slice =
        file.size > MAX_INDEXABLE_BYTES
            ? file.slice(0, MAX_INDEXABLE_BYTES)
            : file;
    const rawText = await slice.text();

    switch (extension) {
        case "html":
        case "htm":
        case "xml":
        case "svg": {
            return rawText
                .replace(
                    /<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi,
                    " "
                )
                .replace(
                    /<style\b[^<]*(?:(?!<\/style>)<[^<]*)*<\/style>/gi,
                    " "
                )
                .replace(/<[^>]+>/g, " ")
                .replace(/&nbsp;/g, " ")
                .replace(/&amp;/g, "&")
                .replace(/&lt;/g, "<")
                .replace(/&gt;/g, ">")
                .replace(/&quot;/g, '"');
        }

        case "json": {
            try {
                const parsed = JSON.parse(rawText);
                const collected: string[] = [];
                const walk = (val: unknown) => {
                    if (typeof val === "string") collected.push(val);
                    else if (
                        typeof val === "number" ||
                        typeof val === "boolean"
                    )
                        collected.push(String(val));
                    else if (Array.isArray(val)) val.forEach(walk);
                    else if (val && typeof val === "object") {
                        Object.entries(val).forEach(([k, v]) => {
                            collected.push(k);
                            walk(v);
                        });
                    }
                };
                walk(parsed);
                return collected.join(" ");
            } catch {
                return rawText;
            }
        }

        case "csv":
        case "tsv": {
            return rawText.replace(/[,\t;]+/g, " ");
        }

        case "md":
        case "markdown": {
            return rawText
                .replace(/#{1,6}\s+/g, " ")
                .replace(/(\*\*|__)(.*?)\1/g, "$2")
                .replace(/(\*|_)(.*?)\1/g, "$2")
                .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
                .replace(/`{1,3}[^`]*`{1,3}/g, " ");
        }

        default:
            return rawText;
    }
}
