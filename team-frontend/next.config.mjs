/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: "standalone",
  experimental: {
    // connect-node is a server-only dependency; keep it out of the RSC bundle.
    serverComponentsExternalPackages: ["@connectrpc/connect-node"],
  },
  // /dev/* is the development-only component catalogue. Pages that call
  // notFound() under a Suspense boundary (our root loading.tsx) stream a 200
  // status, so production gets a real 404 by rewriting to a route that does not
  // exist. The page itself also calls notFound() as a second guard.
  async rewrites() {
    if (process.env.NODE_ENV !== "production") return [];
    // beforeFiles: must win over the filesystem route app/dev/ui/page.tsx.
    return {
      beforeFiles: [{ source: "/dev/:path*", destination: "/__dev-disabled" }],
    };
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
