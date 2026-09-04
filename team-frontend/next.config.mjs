/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: "standalone",
  experimental: {
    // connect-node is a server-only dependency; keep it out of the RSC bundle.
    serverComponentsExternalPackages: ["@connectrpc/connect-node"],
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
