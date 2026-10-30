function originOf(url) {
  try {
    const u = new URL(url);
    return u.protocol === "http:" || u.protocol === "https:" ? u.origin : null;
  } catch {
    return null;
  }
}

function buildCsp() {
  const gateway = originOf(
    process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080",
  );
  const dev = process.env.NODE_ENV !== "production";
  const uniq = (xs) => [...new Set(xs.filter(Boolean))];
  const directives = {
    "default-src": ["'self'"],
    "script-src": [
      "'self'",
      "'unsafe-inline'",
      ...(dev ? ["'unsafe-eval'"] : []),
    ],
    "style-src": ["'self'", "'unsafe-inline'"],
    "connect-src": uniq([
      "'self'",
      gateway,
      ...(dev ? ["ws://localhost:*", "ws://127.0.0.1:*"] : []),
    ]),
    "img-src": ["'self'", "data:", "blob:"],
    "frame-ancestors": ["'none'"],
  };
  return Object.entries(directives)
    .map(([k, v]) => `${k} ${v.join(" ")}`)
    .join("; ");
}

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: "standalone",
  serverExternalPackages: ["@connectrpc/connect-node"],
  transpilePackages: [
    "@fe-platform/api-client",
    "@fe-platform/session",
    "@fe-platform/tokens",
    "@fe-platform/ui",
  ],
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          { key: "Content-Security-Policy", value: buildCsp() },
          {
            key: "Referrer-Policy",
            value: "strict-origin-when-cross-origin",
          },
          { key: "X-Robots-Tag", value: "noindex, nofollow" },
        ],
      },
    ];
  },
  webpack: (config) => {
    config.resolve.extensionAlias = {
      ".js": [".ts", ".tsx", ".js", ".jsx"],
    };
    return config;
  },
};
export default nextConfig;
