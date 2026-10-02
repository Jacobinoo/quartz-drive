import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import compat from "eslint-plugin-compat";
import { fixupConfigRules } from "@eslint/compat";
import eslintConfigPrettier from "eslint-config-prettier";

const eslintConfig = defineConfig([
  ...fixupConfigRules([...nextVitals, ...nextTs]),
  compat.configs["flat/recommended"],
  eslintConfigPrettier,
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
  ]),
]);

export default eslintConfig;
