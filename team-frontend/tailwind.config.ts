import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./src/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/components/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/features/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    extend: {
      colors: {
        primary: {
          50: "#fff5f2",
          100: "#ffe8e1",
          200: "#ffd4c7",
          300: "#ffb5a0",
          400: "#ff8566",
          500: "#ee4d2d",
          600: "#d73211",
          700: "#b52309",
          800: "#941e0c",
          900: "#7a1d0f",
          950: "#430b05",
          DEFAULT: "#ee4d2d",
        },
        surface: {
          DEFAULT: "#ffffff",
          muted: "#f9fafb",
          subtle: "#f3f4f6",
          border: "#e5e7eb",
        },
        brand: {
          DEFAULT: "#ee4d2d",
          dark: "#d73211",
          light: "#ffeee8",
          hover: "#f05d40",
        },
        mall: {
          DEFAULT: "#d0011b",
          dark: "#b00016",
        },
        shopee: {
          bg: "#f5f5f5",
          orange: "#ee4d2d",
          red: "#d0011b",
          yellow: "#ffbe00",
          green: "#00bfa5",
          dark: "#222222",
          gray: "#757575",
          border: "#e5e7eb",
        },
      },
      fontFamily: {
        sans: [
          "Inter",
          "-apple-system",
          "BlinkMacSystemFont",
          '"Segoe UI"',
          "Roboto",
          '"Helvetica Neue"',
          "Arial",
          '"Noto Sans"',
          "sans-serif",
          '"Apple Color Emoji"',
          '"Segoe UI Emoji"',
          '"Segoe UI Symbol"',
        ],
      },
      boxShadow: {
        "preline-card":
          "0 1px 3px 0 rgba(0, 0, 0, 0.07), 0 1px 2px -1px rgba(0, 0, 0, 0.05)",
        "preline-hover":
          "0 10px 15px -3px rgba(0, 0, 0, 0.08), 0 4px 6px -4px rgba(0, 0, 0, 0.04)",
        shopee: "0 1px 1px 0 rgba(0, 0, 0, 0.05)",
        "shopee-hover": "0 2px 8px 0 rgba(0, 0, 0, 0.12)",
        "shopee-card":
          "0 1px 2px 0 rgba(60,64,67,.1), 0 1px 3px 1px rgba(60,64,67,.05)",
      },
    },
  },
  plugins: [],
};
export default config;
