import { fileURLToPath } from "node:url";

import { defineConfig } from "vitest/config";

// Path aliases mirror tsconfig.json ("@/*" -> "./src/*"). `server-only` is a
// marker module whose default entry throws when imported outside an RSC bundle;
// under Vitest we alias it to the package's own empty.js so gateway/action
// modules (which all `import "server-only"`) load cleanly.
export default defineConfig({
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
      "server-only": fileURLToPath(
        new URL("./node_modules/server-only/empty.js", import.meta.url),
      ),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    include: ["src/**/*.{test,spec}.{ts,tsx}"],
  },
});
