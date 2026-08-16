import stylistic from "@stylistic/eslint-plugin";
import { defineConfig } from "eslint/config";
import importNewlines from "eslint-plugin-import-newlines";
import jsonc from "eslint-plugin-jsonc";
import simpleImportSort from "eslint-plugin-simple-import-sort";
import tseslint from "typescript-eslint";

const sourceFiles = ["**/*.{js,jsx,mjs,cjs,ts,tsx,mts,cts}"];

export default defineConfig(
    {
        ignores: [
            "**/.turbo/**",
            "**/bin/**",
            "**/coverage/**",
            "**/dist/**",
            "**/node_modules/**",
            "**/target/**",
            ".pnpm-store/**",
        ],
    },
    ...jsonc.configs.base,
    {
        files: sourceFiles,
        languageOptions: {
            ecmaVersion: "latest",
            parserOptions: {
                ecmaFeatures: {
                    jsx: true,
                },
            },
        },
        plugins: {
            "@stylistic": stylistic,
            "import-newlines": importNewlines,
            "simple-import-sort": simpleImportSort,
        },
        rules: {
            "@stylistic/brace-style": ["error", "1tbs", { allowSingleLine: false }],
            "@stylistic/comma-spacing": ["error", { after: true, before: false }],
            "@stylistic/indent": ["error", 4, { SwitchCase: 1 }],
            "@stylistic/jsx-quotes": ["error", "prefer-double"],
            "@stylistic/max-statements-per-line": ["error", { max: 1 }],
            "@stylistic/no-tabs": "error",
            "@stylistic/no-trailing-spaces": "error",
            "@stylistic/quotes": [
                "error",
                "double",
                { allowTemplateLiterals: "never", avoidEscape: false },
            ],
            "@stylistic/semi": ["error", "always"],
            curly: ["error", "all"],
            "import-newlines/enforce": [
                "error",
                {
                    allowBlankLines: false,
                    forceSingleLine: false,
                    items: 100,
                    "max-len": 100,
                    semi: true,
                },
            ],
            "prefer-arrow-callback": [
                "error",
                { allowNamedFunctions: false, allowUnboundThis: false },
            ],
            "simple-import-sort/exports": "error",
            "simple-import-sort/imports": "error",
        },
    },
    {
        files: ["**/*.{ts,tsx,mts,cts}"],
        languageOptions: {
            parser: tseslint.parser,
            parserOptions: {
                project: ["./packages/agenty-cli/tsconfig.eslint.json"],
                tsconfigRootDir: import.meta.dirname,
            },
        },
        plugins: {
            "@typescript-eslint": tseslint.plugin,
        },
        rules: {
            "@typescript-eslint/no-explicit-any": "error",
            "@typescript-eslint/no-unsafe-argument": "error",
            "@typescript-eslint/no-unsafe-assignment": "error",
            "@typescript-eslint/no-unsafe-call": "error",
            "@typescript-eslint/no-unsafe-function-type": "error",
            "@typescript-eslint/no-unsafe-member-access": "error",
            "@typescript-eslint/no-unsafe-return": "error",
            "@typescript-eslint/no-unsafe-unary-minus": "error",
            "@typescript-eslint/use-unknown-in-catch-callback-variable": "error",
        },
    },
    {
        files: ["**/*.{json,jsonc,json5}"],
        rules: {
            "jsonc/indent": ["error", 4],
        },
    },
);
