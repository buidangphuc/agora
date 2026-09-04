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
