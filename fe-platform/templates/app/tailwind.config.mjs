import tokens from "@fe-platform/tokens/preset";

export default {
  presets: [tokens],
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "../../packages/ui/src/**/*.{ts,tsx}",
    "../../packages/tokens/src/tokens.mjs",
  ],
};
