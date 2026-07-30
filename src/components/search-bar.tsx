"use client";

import { useState, useRef, useEffect } from "react";
import { Input } from "@/components/ui/input";
import { Loader2, File, Folder } from "lucide-react";
import { buildE2EESearchIndex, searchFiles } from "@/lib/searchService";
import { DecryptedSearchItem } from "@/lib/SearchIndexStore";

export function SearchBar() {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<DecryptedSearchItem[]>([]);
  const [isBuilding, setIsBuilding] = useState(false);
  const [isFocused, setIsFocused] = useState(false);

  // Track if we've already tried to build the index this session
  const hasBuiltRef = useRef(false);

  const handleFocus = async () => {
    setIsFocused(true);

    // Only build the index once per session when they click the search bar!
    if (!hasBuiltRef.current) {
      setIsBuilding(true);
      try {
        await buildE2EESearchIndex();
        hasBuiltRef.current = true;
      } catch (e) {
        console.error("Failed to build secure search index", e);
      } finally {
        setIsBuilding(false);
      }
    }
  };

  const handleChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setQuery(val);

    if (val.trim() === "") {
      setResults([]);
      return;
    }

    // Run the lightning-fast filter against IndexedDB!
    const matches = await searchFiles(val);
    setResults(matches);
  };

  return (
    <div className="relative w-full max-w-sm">
      <div className="relative">
        <Input
          placeholder="Search..."
          value={query}
          onFocus={handleFocus}
          onChange={handleChange}
          onBlur={() => {
            // Delay closing slightly so clicks on results register
            setTimeout(() => setIsFocused(false), 200);
          }}
          className="w-full"
        />
        {isBuilding && (
          <div className="absolute right-3 top-1/2 -translate-y-1/2">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          </div>
        )}
      </div>

      {/* The Results Dropdown Popover */}
      {isFocused && (query.length > 0 || isBuilding) && (
        <div className="absolute top-full mt-2 w-full rounded-md border bg-popover text-popover-foreground shadow-md z-50 p-1">

          {isBuilding && (
            <div className="flex items-center gap-2 p-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              <span>Unlocking Secure Search...</span>
            </div>
          )}

          {!isBuilding && results.length === 0 && query.length > 0 && (
            <div className="p-2 text-sm text-muted-foreground text-center">
              No files found.
            </div>
          )}

          {!isBuilding && results.map((item) => (
            <div
              key={item.id}
              className="flex items-center gap-2 p-2 hover:bg-accent hover:text-accent-foreground rounded-sm cursor-pointer text-sm"
              onClick={() => {
                // TODO: Wire this up to navigate to the folder, or select the file!
                console.log("Clicked:", item);
              }}
            >
              {item.type === "FOLDER" ? (
                <Folder className="h-4 w-4 text-blue-500" />
              ) : (
                <File className="h-4 w-4 text-muted-foreground" />
              )}
              <span className="truncate">{item.name}</span>
            </div>
          ))}

        </div>
      )}
    </div>
  );
}
