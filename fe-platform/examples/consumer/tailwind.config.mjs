import tokens from "@fe-platform/tokens/preset";

export default {
  presets: [tokens],
  content: [
    "./app/**/*.{ts,tsx}",
    "./node_modules/@fe-platform/ui/src/**/*.{ts,tsx}",
    "./node_modules/@fe-platform/tokens/src/tokens.mjs",
  ],
};
