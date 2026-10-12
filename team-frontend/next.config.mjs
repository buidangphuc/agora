const GOOGLE_HOSTS = [
  "https://www.googletagmanager.com",
  "https://*.google-analytics.com",
  "https://*.analytics.google.com",
  "https://region1.google-analytics.com",
];

/** Origin of an http(s) URL, or null when unset or invalid. */
function originOf(url) {
  try {
    const u = new URL(url);
    return u.protocol === "http:" || u.protocol === "https:" ? u.origin : null;
  } catch {
    return null;
  }
}

/** Content-Security-Policy derived from build-time env. */
function buildCsp() {
  const google = process.env.NEXT_PUBLIC_GTM_ID?.trim() ? GOOGLE_HOSTS : [];
  const gateway = originOf(
    process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080",
  );
  // Presigned uploads (browser PUT) and images go to the media/S3 origin.
  const media = originOf(
    process.env.NEXT_PUBLIC_MEDIA_BASE_URL ||
      "http://localhost:9000/listing-images",
  );
  const dev = process.env.NODE_ENV !== "production";
  const uniq = (xs) => [...new Set(xs.filter(Boolean))];
  const directives = {
    "default-src": ["'self'"],
    "script-src": [
      "'self'",
      "'unsafe-inline'",
      ...(dev ? ["'unsafe-eval'"] : []),
      ...google,
    ],
    "style-src": ["'self'", "'unsafe-inline'"],
    "connect-src": uniq([
      "'self'",
      gateway,
      media,
      ...(dev ? ["ws://localhost:*", "ws://127.0.0.1:*"] : []),
      ...google,
    ]),
    "img-src": uniq(["'self'", "data:", "blob:", media, ...google]),
  };
  return Object.entries(directives)
    .map(([k, v]) => `${k} ${v.join(" ")}`)
    .join("; ");
}

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: "standalone",
  // connect-node is a server-only dependency; keep it out of the RSC bundle.
  serverExternalPackages: ["@connectrpc/connect-node"],
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [{ key: "Content-Security-Policy", value: buildCsp() }],
      },
    ];
  },
  webpack: (config) => {
    // Connect-ES / protoc-gen-es emit ESM imports with .js extensions that point
    // at .ts files; let webpack resolve them.
    config.resolve.extensionAlias = {
      ".js": [".ts", ".tsx", ".js", ".jsx"],
    };
    return config;
  },
};
export default nextConfig;
